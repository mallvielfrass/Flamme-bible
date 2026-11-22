package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Чтение Библии для страницы «Библия»: переводы, книги, глава и поиск по словам.
// Перевод задаётся коротким именем, как в пунктах плана (module_short_name).
// Видны только одобренные модули; при одинаковых именах берётся самый ранний
// модуль со стихами — то же правило, что у GET /api/bible/verse.

type bibleTranslation struct {
	ShortName string `json:"short_name"`
	Title     string `json:"title"`
}

type bibleBook struct {
	Book     int    `json:"book"`
	BookName string `json:"book_name"`
	Chapters int    `json:"chapters"`
}

type bibleVerse struct {
	Verse int    `json:"verse"`
	Text  string `json:"text"`
}

type bibleChapter struct {
	ModuleShortName string       `json:"module_short_name"`
	ModuleTitle     string       `json:"module_title"`
	Book            int          `json:"book"`
	BookName        string       `json:"book_name"`
	Chapter         int          `json:"chapter"`
	Verses          []bibleVerse `json:"verses"`
}

type bibleVerseHit struct {
	ModuleShortName string `json:"module_short_name"`
	Book            int    `json:"book"`
	BookName        string `json:"book_name"`
	Chapter         int    `json:"chapter"`
	Verse           int    `json:"verse"`
	Text            string `json:"text"`
}

const bibleSearchLimit = 50

// approvedModule находит модуль перевода по короткому имени.
func (s *Server) approvedModule(ctx context.Context, shortName string) (id, title string, err error) {
	if !validModuleName(shortName) {
		return "", "", errRequest
	}
	err = s.db.QueryRow(ctx, `
		SELECT m.id::text, m.title
		FROM modules m
		WHERE m.status = 'approved' AND m.short_name = $1
		  AND EXISTS (SELECT 1 FROM module_verses v WHERE v.module_id = m.id)
		ORDER BY m.created_at, m.id
		LIMIT 1
	`, shortName).Scan(&id, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", errNotFound
	}
	return id, title, err
}

func (s *Server) bibleTranslations(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT DISTINCT ON (m.short_name) m.short_name, m.title
		FROM modules m
		WHERE m.status = 'approved'
		  AND EXISTS (SELECT 1 FROM module_verses v WHERE v.module_id = m.id)
		ORDER BY m.short_name, m.created_at, m.id
	`)
	if err != nil {
		writeMapped(w, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (bibleTranslation, error) {
		var t bibleTranslation
		return t, row.Scan(&t.ShortName, &t.Title)
	})
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"translations": list})
}

func (s *Server) bibleBooks(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	moduleID, _, err := s.approvedModule(r.Context(), strings.TrimSpace(r.URL.Query().Get("module")))
	if err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT book, MIN(book_name), MAX(chapter)
		FROM module_verses
		WHERE module_id = $1
		GROUP BY book
		ORDER BY book
	`, moduleID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (bibleBook, error) {
		var b bibleBook
		return b, row.Scan(&b.Book, &b.BookName, &b.Chapters)
	})
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"books": list})
}

func (s *Server) bibleChapterHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	q := r.URL.Query()
	book, errBook := strconv.Atoi(q.Get("book"))
	chapter, errChapter := strconv.Atoi(q.Get("chapter"))
	if errBook != nil || errChapter != nil || book < 1 || chapter < 1 {
		writeMapped(w, errRequest)
		return
	}
	out := bibleChapter{ModuleShortName: strings.TrimSpace(q.Get("module")), Book: book, Chapter: chapter}
	moduleID, title, err := s.approvedModule(r.Context(), out.ModuleShortName)
	if err != nil {
		writeMapped(w, err)
		return
	}
	out.ModuleTitle = title
	rows, err := s.db.Query(r.Context(), `
		SELECT verse, book_name, body
		FROM module_verses
		WHERE module_id = $1 AND book = $2 AND chapter = $3
		ORDER BY verse
	`, moduleID, book, chapter)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	out.Verses = []bibleVerse{}
	for rows.Next() {
		var v bibleVerse
		if err := rows.Scan(&v.Verse, &out.BookName, &v.Text); err != nil {
			writeMapped(w, err)
			return
		}
		out.Verses = append(out.Verses, v)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	if len(out.Verses) == 0 {
		writeMapped(w, errNotFound)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// searchBibleWords ищет стихи одного перевода, в которых есть фраза целиком
// (без учёта регистра), по порядку книг. Страница по 50 стихов.
func (s *Server) searchBibleWords(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	if utf8.RuneCountInString(q) < 2 || utf8.RuneCountInString(q) > 200 {
		writeMapped(w, errRequest)
		return
	}
	offset := 0
	if raw := query.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 10000 {
			writeMapped(w, errRequest)
			return
		}
		offset = n
	}
	short := strings.TrimSpace(query.Get("module"))
	moduleID, _, err := s.approvedModule(r.Context(), short)
	if err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT book, book_name, chapter, verse, body
		FROM module_verses
		WHERE module_id = $1 AND body ILIKE $2 ESCAPE '\'
		ORDER BY book, chapter, verse
		LIMIT $3 OFFSET $4
	`, moduleID, likeContains(q), bibleSearchLimit, offset)
	if err != nil {
		writeMapped(w, err)
		return
	}
	hits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (bibleVerseHit, error) {
		hit := bibleVerseHit{ModuleShortName: short}
		return hit, row.Scan(&hit.Book, &hit.BookName, &hit.Chapter, &hit.Verse, &hit.Text)
	})
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verses": hits, "more": len(hits) == bibleSearchLimit})
}
