package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type moduleVerse struct {
	Book     int
	BookName string
	Chapter  int
	Verse    int
	Body     string
}

type passageDraft struct {
	Book       int
	BookName   string
	Chapter    int
	VerseStart int
	VerseEnd   int
	Reference  string
	Body       string
}

type storedPassage struct {
	id    string
	hash  string
	model string
}

type bibleHit struct {
	ModuleID        string `json:"module_id"`
	ModuleShortName string `json:"module_short_name"`
	ModuleTitle     string `json:"module_title"`
	Book            int    `json:"book"`
	BookName        string `json:"book_name"`
	Chapter         int    `json:"chapter"`
	VerseStart      int    `json:"verse_start"`
	VerseEnd        int    `json:"verse_end"`
	Reference       string `json:"reference"`
	Text            string `json:"text"`
}

// chunkVerses режет главу на окна по 3–5 стихов. Глава короче трёх стихов
// остаётся одним отрывком: соседних стихов, которыми её можно дополнить, нет.
func chunkVerses(verses []moduleVerse) []passageDraft {
	var out []passageDraft
	var group []moduleVerse
	flush := func() {
		if len(group) == 0 {
			return
		}
		out = append(out, chunkChapter(group)...)
		group = nil
	}
	for _, verse := range verses {
		if len(group) > 0 && (group[0].Book != verse.Book || group[0].Chapter != verse.Chapter) {
			flush()
		}
		group = append(group, verse)
	}
	flush()
	return out
}

func chunkChapter(verses []moduleVerse) []passageDraft {
	var out []passageDraft
	i := 0
	for i < len(verses) {
		rest := len(verses) - i
		take := 4
		if rest < 3 {
			take = rest
		} else if rest <= 5 {
			take = rest
		} else if rest-4 < 3 {
			// Шесть стихов: 4+2 оставило бы хвост короче трёх, поэтому 3+3.
			take = rest - 3
		}
		out = append(out, makePassage(verses[i:i+take]))
		i += take
	}
	return out
}

func makePassage(verses []moduleVerse) passageDraft {
	first := verses[0]
	last := verses[len(verses)-1]
	var body strings.Builder
	for i, verse := range verses {
		if i > 0 {
			body.WriteByte('\n')
		}
		fmt.Fprintf(&body, "%d %s", verse.Verse, strings.TrimSpace(verse.Body))
	}
	name := first.BookName
	if name == "" {
		name = "Книга"
	}
	ref := fmt.Sprintf("%s %d:%d", name, first.Chapter, first.Verse)
	if last.Verse != first.Verse {
		ref = fmt.Sprintf("%s %d:%d-%d", name, first.Chapter, first.Verse, last.Verse)
	}
	return passageDraft{
		Book:       first.Book,
		BookName:   first.BookName,
		Chapter:    first.Chapter,
		VerseStart: first.Verse,
		VerseEnd:   last.Verse,
		Reference:  ref,
		Body:       body.String(),
	}
}

func (s *Server) ImportBibleModule(ctx context.Context, moduleID, dir string) (int, error) {
	if !validUUID(moduleID) {
		return 0, errNotFound
	}
	verses, err := readBibleQuote(dir)
	if err != nil {
		return 0, err
	}
	if err := s.replaceVerses(ctx, moduleID, verses); err != nil {
		return 0, err
	}
	return len(verses), nil
}

func (s *Server) replaceVerses(ctx context.Context, moduleID string, verses []moduleVerse) error {
	if len(verses) == 0 {
		return fmt.Errorf("в модуле нет стихов")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM modules WHERE id = $1 FOR UPDATE`, moduleID).Scan(&status)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM module_verses WHERE module_id = $1`, moduleID); err != nil {
		return err
	}
	for _, verse := range verses {
		if _, err := tx.Exec(ctx, `
			INSERT INTO module_verses (module_id, book, book_name, chapter, verse, body)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, moduleID, verse.Book, verse.BookName, verse.Chapter, verse.Verse, verse.Body); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if status == "approved" && s.embedder != nil {
		_, _, err = s.indexModule(ctx, moduleID)
		return err
	}
	return s.clearPassages(ctx, moduleID)
}

func (s *Server) clearPassages(ctx context.Context, moduleID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM bible_passages WHERE module_id = $1`, moduleID)
	return err
}

func (s *Server) indexModule(ctx context.Context, moduleID string) (int, int, error) {
	if s.embedder == nil {
		return 0, 0, nil
	}
	verses, err := s.loadVerses(ctx, moduleID)
	if err != nil {
		return 0, 0, err
	}
	desired := chunkVerses(verses)
	stored, err := s.loadStoredPassages(ctx, moduleID)
	if err != nil {
		return 0, 0, err
	}
	type pendingPassage struct {
		draft passageDraft
		hash  string
		id    string
	}
	var pending []pendingPassage
	keep := make([]string, 0, len(desired))
	embedded := 0
	skipped := 0
	for _, draft := range desired {
		hash := hashText(draft.Body)
		key := fmt.Sprintf("%d:%d:%d", draft.Book, draft.Chapter, draft.VerseStart)
		if ex, ok := stored[key]; ok && ex.model == s.embedder.Model() && ex.hash == hash {
			skipped++
			keep = append(keep, ex.id)
			continue
		}
		id := uuid.NewString()
		if ex, ok := stored[key]; ok {
			id = ex.id
		}
		pending = append(pending, pendingPassage{draft: draft, hash: hash, id: id})
		keep = append(keep, id)
	}
	for start := 0; start < len(pending); start += embedBatch {
		end := start + embedBatch
		if end > len(pending) {
			end = len(pending)
		}
		chunk := pending[start:end]
		texts := make([]string, len(chunk))
		for i := range chunk {
			texts[i] = chunk[i].draft.Body
		}
		vecs, err := s.embedder.Embed(ctx, embedPassage, texts)
		if err != nil {
			return embedded, skipped, err
		}
		if len(vecs) != len(chunk) {
			return embedded, skipped, fmt.Errorf("эмбеддер вернул %d векторов на %d текстов", len(vecs), len(chunk))
		}
		for i := range chunk {
			if err := validateVector(vecs[i]); err != nil {
				return embedded, skipped, err
			}
			if err := s.upsertPassage(ctx, moduleID, chunk[i].id, chunk[i].draft, chunk[i].hash, vecs[i]); err != nil {
				return embedded, skipped, err
			}
			embedded++
		}
	}
	if err := s.deleteStalePassages(ctx, moduleID, keep); err != nil {
		return embedded, skipped, err
	}
	return embedded, skipped, nil
}

func (s *Server) loadVerses(ctx context.Context, moduleID string) ([]moduleVerse, error) {
	rows, err := s.db.Query(ctx, `
		SELECT book, book_name, chapter, verse, body
		FROM module_verses
		WHERE module_id = $1
		ORDER BY book, chapter, verse
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var verses []moduleVerse
	for rows.Next() {
		var verse moduleVerse
		if err := rows.Scan(&verse.Book, &verse.BookName, &verse.Chapter, &verse.Verse, &verse.Body); err != nil {
			return nil, err
		}
		verses = append(verses, verse)
	}
	return verses, rows.Err()
}

func (s *Server) loadStoredPassages(ctx context.Context, moduleID string) (map[string]storedPassage, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, book, chapter, verse_start, model, text_hash
		FROM bible_passages WHERE module_id = $1
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]storedPassage{}
	for rows.Next() {
		var item storedPassage
		var book, chapter, start int
		if err := rows.Scan(&item.id, &book, &chapter, &start, &item.model, &item.hash); err != nil {
			return nil, err
		}
		out[fmt.Sprintf("%d:%d:%d", book, chapter, start)] = item
	}
	return out, rows.Err()
}

func (s *Server) upsertPassage(ctx context.Context, moduleID, id string, draft passageDraft, hash string, vec []float32) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO bible_passages (
			id, module_id, book, book_name, chapter, verse_start, verse_end,
			reference, body, model, text_hash, embedding
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::vector)
		ON CONFLICT (module_id, book, chapter, verse_start) DO UPDATE SET
			verse_end = EXCLUDED.verse_end,
			book_name = EXCLUDED.book_name,
			reference = EXCLUDED.reference,
			body = EXCLUDED.body,
			model = EXCLUDED.model,
			text_hash = EXCLUDED.text_hash,
			embedding = EXCLUDED.embedding
	`, id, moduleID, draft.Book, draft.BookName, draft.Chapter, draft.VerseStart, draft.VerseEnd,
		draft.Reference, draft.Body, s.embedder.Model(), hash, vectorLiteral(vec))
	return err
}

func (s *Server) deleteStalePassages(ctx context.Context, moduleID string, keep []string) error {
	if keep == nil {
		keep = []string{}
	}
	_, err := s.db.Exec(ctx, `
		DELETE FROM bible_passages
		WHERE module_id = $1 AND NOT (id::text = ANY($2::text[]))
	`, moduleID, keep)
	return err
}

func (s *Server) searchBibleSemantic(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	if s.embedder == nil {
		writeMapped(w, errSemanticOff)
		return
	}
	q, limit, err := parseSemanticQuery(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	moduleID, shortName, err := bibleModuleFilter(r.URL.Query().Get("module"))
	if err != nil {
		writeMapped(w, err)
		return
	}
	vec, ok := s.queryVector(r.Context(), q)
	if !ok {
		writeMapped(w, errSemanticOff)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT p.module_id::text, m.short_name, m.title, p.book, p.book_name, p.chapter,
		       p.verse_start, p.verse_end, p.reference, p.body
		FROM bible_passages p
		JOIN modules m ON m.id = p.module_id
		WHERE m.status = 'approved'
		  AND ($2::uuid IS NULL OR p.module_id = $2::uuid)
		  AND ($3::text IS NULL OR m.short_name = $3)
		ORDER BY p.embedding <=> $1::vector
		LIMIT $4
	`, vectorLiteral(vec), moduleID, shortName, limit)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	hits := []bibleHit{}
	for rows.Next() {
		var hit bibleHit
		if err := rows.Scan(&hit.ModuleID, &hit.ModuleShortName, &hit.ModuleTitle, &hit.Book, &hit.BookName, &hit.Chapter, &hit.VerseStart, &hit.VerseEnd, &hit.Reference, &hit.Text); err != nil {
			writeMapped(w, err)
			return
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"passages": hits})
}

func bibleModuleFilter(raw string) (id any, short any, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, nil
	}
	if validUUID(raw) {
		return raw, nil, nil
	}
	if validModuleName(raw) {
		return nil, raw, nil
	}
	return nil, nil, errRequest
}
