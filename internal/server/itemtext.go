package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Полный текст пункта плана для просмотра: песня из сборника или один стих перевода.
// Доступ к песне — тот же, что у поиска (songAccessWhere); переводы — только одобренные.

type songText struct {
	Title         string `json:"title"`
	SongbookTitle string `json:"songbook_title"`
	SectionTitle  string `json:"section_title"`
	Text          string `json:"text"`
}

type verseText struct {
	ModuleShortName string `json:"module_short_name"`
	ModuleTitle     string `json:"module_title"`
	BookName        string `json:"book_name"`
	Chapter         int    `json:"chapter"`
	Verse           int    `json:"verse"`
	Text            string `json:"text"`
}

func (s *Server) songTextHandler(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	book := strings.TrimSpace(r.URL.Query().Get("songbook_id"))
	fbid := strings.TrimSpace(r.URL.Query().Get("fbid"))
	if !validUUID(book) || fbid == "" || len(fbid) > 200 {
		writeMapped(w, errRequest)
		return
	}
	var out songText
	var raw []byte
	err = s.db.QueryRow(r.Context(), `
		SELECT s.title, b.title, `+sectionTitleSQL+`, s.parts
		FROM songs s
		JOIN songbooks b ON b.id = s.songbook_id
		WHERE `+songAccessWhere("$1", "$2", "$3")+`
		  AND s.fbid = $4
	`, book, nil, p.UserID, fbid).Scan(&out.Title, &out.SongbookTitle, &out.SectionTitle, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	out.Text = partsText(raw)
	writeJSON(w, http.StatusOK, out)
}

func partsText(raw []byte) string {
	var parts []partDTO
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n\n")
}

func (s *Server) verseTextHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	q := r.URL.Query()
	module := strings.TrimSpace(q.Get("module"))
	book, errBook := strconv.Atoi(q.Get("book"))
	chapter, errChapter := strconv.Atoi(q.Get("chapter"))
	verse, errVerse := strconv.Atoi(q.Get("verse"))
	if !validModuleName(module) || errBook != nil || errChapter != nil || errVerse != nil || book < 1 || chapter < 1 || verse < 1 {
		writeMapped(w, errRequest)
		return
	}
	out := verseText{Chapter: chapter, Verse: verse}
	err := s.db.QueryRow(r.Context(), `
		SELECT m.short_name, m.title, v.book_name, v.body
		FROM module_verses v
		JOIN modules m ON m.id = v.module_id
		WHERE m.status = 'approved' AND m.short_name = $1
		  AND v.book = $2 AND v.chapter = $3 AND v.verse = $4
		ORDER BY m.created_at
		LIMIT 1
	`, module, book, chapter, verse).Scan(&out.ModuleShortName, &out.ModuleTitle, &out.BookName, &out.Text)
	if errors.Is(err, pgx.ErrNoRows) {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
