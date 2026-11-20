package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	songEmbedRunes = 4000
	fragmentRunes  = 280
)

type semanticJob struct {
	songs    []songKey
	moduleID string
	clear    bool
}

type songKey struct {
	songbookID string
	fbid       string
}

type songDoc struct {
	songbookID  string
	fbid        string
	title       string
	alt         string
	parts       []partDTO
	deleted     bool
	text        string
	hash        string
	storedModel string
	storedHash  string
}

type semanticSongHit struct {
	songHit
	Fragment string `json:"fragment"`
}

type ReindexStats struct {
	SongsEmbedded    int
	SongsSkipped     int
	Modules          int
	PassagesEmbedded int
	PassagesSkipped  int
}

func (s *Server) SetEmbedder(e Embedder) {
	s.embedder = e
}

func (s *Server) EmbedModel() string {
	if s.embedder == nil {
		return ""
	}
	return s.embedder.Model()
}

func (s *Server) StartIndexer(ctx context.Context) {
	if s.embedder == nil || s.indexCh != nil {
		return
	}
	s.indexCh = make(chan semanticJob, 64)
	go s.indexLoop(ctx)
}

func (s *Server) noteAppliedSongs(bookID string, prepared []preparedOp, results []opResult) {
	if s.embedder == nil {
		return
	}
	keys := make([]songKey, 0, len(results))
	for i, res := range results {
		if res.Status != "applied" {
			continue
		}
		keys = append(keys, songKey{songbookID: bookID, fbid: prepared[i].op.Song.FBID})
	}
	if len(keys) == 0 {
		return
	}
	s.scheduleSongs(keys)
}

func (s *Server) noteModule(id, status string) {
	if s.embedder == nil {
		return
	}
	job := semanticJob{moduleID: id, clear: status != "approved"}
	if s.indexInline {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s.runJob(ctx, job)
		return
	}
	s.enqueue(job)
}

func (s *Server) scheduleSongs(keys []songKey) {
	if s.indexInline {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, _, err := s.indexSongs(ctx, keys); err != nil {
			log.Printf("индекс песен: %v", err)
		}
		return
	}
	s.enqueue(semanticJob{songs: keys})
}

func (s *Server) enqueue(job semanticJob) {
	if s.indexCh == nil {
		log.Printf("индекс: фоновая очередь не запущена")
		return
	}
	select {
	case s.indexCh <- job:
	default:
		go func() { s.indexCh <- job }()
	}
}

func (s *Server) indexLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.indexCh:
			s.runJob(ctx, job)
		}
	}
}

func (s *Server) runJob(ctx context.Context, job semanticJob) {
	var err error
	switch {
	case job.clear:
		err = s.clearPassages(ctx, job.moduleID)
	case job.moduleID != "":
		_, _, err = s.indexModule(ctx, job.moduleID)
	default:
		_, _, err = s.indexSongs(ctx, job.songs)
	}
	if err != nil && ctx.Err() == nil {
		log.Printf("индекс: %v", err)
	}
}

func (s *Server) ReindexAll(ctx context.Context) (ReindexStats, error) {
	if s.embedder == nil {
		return ReindexStats{}, errSemanticOff
	}
	var stats ReindexStats
	embedded, skipped, err := s.indexSongs(ctx, nil)
	if err != nil {
		return stats, err
	}
	stats.SongsEmbedded = embedded
	stats.SongsSkipped = skipped
	if err := s.deleteOrphanEmbeddings(ctx); err != nil {
		return stats, err
	}
	rows, err := s.db.Query(ctx, `SELECT id::text FROM modules WHERE status = 'approved' ORDER BY id`)
	if err != nil {
		return stats, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return stats, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return stats, err
	}
	rows.Close()
	for _, id := range ids {
		embedded, skipped, err := s.indexModule(ctx, id)
		if err != nil {
			return stats, err
		}
		stats.Modules++
		stats.PassagesEmbedded += embedded
		stats.PassagesSkipped += skipped
	}
	if _, err := s.db.Exec(ctx, `
		DELETE FROM bible_passages p
		WHERE NOT EXISTS (
			SELECT 1 FROM modules m WHERE m.id = p.module_id AND m.status = 'approved'
		)
	`); err != nil {
		return stats, err
	}
	return stats, nil
}

const songDocSelect = `
	SELECT s.songbook_id::text, s.fbid, s.title, s.alt_title, s.parts, s.deleted,
	       COALESCE(e.model, ''), COALESCE(e.text_hash, '')
	FROM songs s
	LEFT JOIN song_embeddings e ON e.songbook_id = s.songbook_id AND e.fbid = s.fbid
`

func (s *Server) indexSongs(ctx context.Context, keys []songKey) (int, int, error) {
	if s.embedder == nil {
		return 0, 0, nil
	}
	docs, err := s.loadSongDocs(ctx, keys)
	if err != nil {
		return 0, 0, err
	}
	embedded := 0
	skipped := 0
	var pending []songDoc
	for _, doc := range docs {
		if doc.deleted || doc.text == "" {
			if err := s.deleteSongEmbedding(ctx, doc.songbookID, doc.fbid); err != nil {
				return embedded, skipped, err
			}
			continue
		}
		if doc.storedModel == s.embedder.Model() && doc.storedHash == doc.hash {
			skipped++
			continue
		}
		pending = append(pending, doc)
	}
	for start := 0; start < len(pending); start += embedBatch {
		end := start + embedBatch
		if end > len(pending) {
			end = len(pending)
		}
		chunk := pending[start:end]
		texts := make([]string, len(chunk))
		for i := range chunk {
			texts[i] = chunk[i].text
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
			if err := s.upsertSongEmbedding(ctx, chunk[i], vecs[i]); err != nil {
				return embedded, skipped, err
			}
			embedded++
		}
	}
	return embedded, skipped, nil
}

func (s *Server) loadSongDocs(ctx context.Context, keys []songKey) ([]songDoc, error) {
	if keys == nil {
		return s.querySongDocs(ctx, songDocSelect+` WHERE NOT s.deleted`)
	}
	var docs []songDoc
	for _, key := range keys {
		part, err := s.querySongDocs(ctx, songDocSelect+` WHERE s.songbook_id = $1 AND s.fbid = $2`, key.songbookID, key.fbid)
		if err != nil {
			return nil, err
		}
		docs = append(docs, part...)
	}
	return docs, nil
}

func (s *Server) querySongDocs(ctx context.Context, query string, args ...any) ([]songDoc, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []songDoc
	for rows.Next() {
		var doc songDoc
		var parts []byte
		if err := rows.Scan(&doc.songbookID, &doc.fbid, &doc.title, &doc.alt, &parts, &doc.deleted, &doc.storedModel, &doc.storedHash); err != nil {
			return nil, err
		}
		if len(parts) > 0 && string(parts) != "null" {
			if err := json.Unmarshal(parts, &doc.parts); err != nil {
				return nil, err
			}
		}
		doc.text = songEmbedText(doc.title, doc.alt, doc.parts)
		doc.hash = hashText(doc.text)
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (s *Server) upsertSongEmbedding(ctx context.Context, doc songDoc, vec []float32) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO song_embeddings (songbook_id, fbid, model, text_hash, embedding)
		VALUES ($1, $2, $3, $4, $5::vector)
		ON CONFLICT (songbook_id, fbid) DO UPDATE SET
			model = EXCLUDED.model,
			text_hash = EXCLUDED.text_hash,
			embedding = EXCLUDED.embedding
	`, doc.songbookID, doc.fbid, s.embedder.Model(), doc.hash, vectorLiteral(vec))
	return err
}

func (s *Server) deleteSongEmbedding(ctx context.Context, songbookID, fbid string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM song_embeddings WHERE songbook_id = $1 AND fbid = $2`, songbookID, fbid)
	return err
}

func (s *Server) deleteOrphanEmbeddings(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM song_embeddings e
		WHERE NOT EXISTS (
			SELECT 1 FROM songs s
			WHERE s.songbook_id = e.songbook_id AND s.fbid = e.fbid AND NOT s.deleted
		)
	`)
	return err
}

func songEmbedText(title, alt string, parts []partDTO) string {
	var b strings.Builder
	write := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	write(title)
	write(alt)
	for _, part := range parts {
		write(part.Text)
	}
	return clipRunes(b.String(), songEmbedRunes)
}

func songFragment(title string, parts []partDTO) string {
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text); text != "" {
			return clipRunes(text, fragmentRunes)
		}
	}
	return clipRunes(strings.TrimSpace(title), fragmentRunes)
}

func clipRunes(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[:n]) + "…"
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (s *Server) searchSongsSemantic(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
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
	book, dim, err := songSearchScope(r)
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
		SELECT s.songbook_id::text, s.fbid, s.title, b.title, b.dimension_id::text, b.status, s.parts
		FROM song_embeddings e
		JOIN songs s ON s.songbook_id = e.songbook_id AND s.fbid = e.fbid
		JOIN songbooks b ON b.id = s.songbook_id
		WHERE `+songAccessWhere("$2", "$3", "$4")+`
		ORDER BY e.embedding <=> $1::vector
		LIMIT $5
	`, vectorLiteral(vec), book, dim, p.UserID, limit)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	hits := []semanticSongHit{}
	for rows.Next() {
		var hit semanticSongHit
		var parts []byte
		if err := rows.Scan(&hit.SongbookID, &hit.FBID, &hit.Title, &hit.SongbookTitle, &hit.DimensionID, &hit.Status, &parts); err != nil {
			writeMapped(w, err)
			return
		}
		var parsed []partDTO
		if len(parts) > 0 && string(parts) != "null" {
			if err := json.Unmarshal(parts, &parsed); err != nil {
				writeMapped(w, err)
				return
			}
		}
		hit.Fragment = songFragment(hit.Title, parsed)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"songs": hits})
}

func (s *Server) queryVector(ctx context.Context, q string) ([]float32, bool) {
	vecs, err := s.embedder.Embed(ctx, embedQuery, []string{q})
	if err != nil || len(vecs) != 1 {
		if err != nil {
			log.Printf("смысловой поиск: %v", err)
		}
		return nil, false
	}
	if err := validateVector(vecs[0]); err != nil {
		log.Printf("смысловой поиск: %v", err)
		return nil, false
	}
	return vecs[0], true
}

func parseSemanticQuery(r *http.Request) (string, int, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || utf8.RuneCountInString(q) > 200 {
		return "", 0, errRequest
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return "", 0, errRequest
		}
		limit = n
	}
	return q, limit, nil
}
