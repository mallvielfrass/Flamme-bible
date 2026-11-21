package server

import (
	"context"
	"fmt"
	"strconv"
)

func (s *Server) createShadowTables(ctx context.Context, dim int) error {
	if dim < 1 || dim > 4096 {
		return fmt.Errorf("размерность %d", dim)
	}
	stmts := []string{
		`DROP TABLE IF EXISTS song_embeddings_next`,
		fmt.Sprintf(`
			CREATE TABLE song_embeddings_next (
				songbook_id uuid NOT NULL,
				fbid text NOT NULL,
				part_index int NOT NULL,
				model text NOT NULL,
				text_hash text NOT NULL,
				embedding vector(%d) NOT NULL,
				PRIMARY KEY (songbook_id, fbid, part_index),
				FOREIGN KEY (songbook_id, fbid) REFERENCES songs (songbook_id, fbid) ON DELETE CASCADE
			)`, dim),
		`CREATE INDEX song_embeddings_next_hnsw ON song_embeddings_next USING hnsw (embedding vector_cosine_ops)`,
		`DROP TABLE IF EXISTS bible_passages_next`,
		fmt.Sprintf(`
			CREATE TABLE bible_passages_next (
				id uuid PRIMARY KEY,
				module_id uuid NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
				book int NOT NULL,
				book_name text NOT NULL DEFAULT '',
				chapter int NOT NULL,
				verse_start int NOT NULL,
				verse_end int NOT NULL,
				reference text NOT NULL,
				body text NOT NULL,
				model text NOT NULL,
				text_hash text NOT NULL,
				embedding vector(%d) NOT NULL,
				UNIQUE (module_id, book, chapter, verse_start)
			)`, dim),
		`CREATE INDEX bible_passages_next_hnsw ON bible_passages_next USING hnsw (embedding vector_cosine_ops)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) startShadowReindex(ctx context.Context, actor string) error {
	if s.embedder == nil {
		return errSemanticOff
	}
	if s.shadowOpen(ctx) {
		return errConflict
	}
	s.refreshEmbedLive(ctx)
	live := s.liveEmbed()
	if live.ModelID == "" || live.Dim <= 0 {
		return errSemanticOff
	}
	if err := s.createShadowTables(ctx, live.Dim); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM embedding_jobs WHERE target = 'next'`); err != nil {
		return err
	}
	songs, err := s.shadowSongJobs(ctx)
	if err != nil {
		return err
	}
	if err := s.enqueueSongParts(ctx, "next", songs); err != nil {
		return err
	}
	if err := s.enqueueShadowPassages(ctx); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.putSetting(ctx, tx, "embed_shadow", "1"); err != nil {
		return err
	}
	if err := s.putSetting(ctx, tx, "embed_shadow_model", live.ModelID); err != nil {
		return err
	}
	if err := s.putSetting(ctx, tx, "embed_shadow_actor", actor); err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, actor, "embed_reindex", "embed", "queue", live.ModelID, fmt.Sprintf("dim %d", live.Dim)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.afterEmbedEnqueue(ctx)
}

func (s *Server) shadowSongJobs(ctx context.Context) ([]songPartJob, error) {
	docs, err := s.loadSongDocs(ctx, nil)
	if err != nil {
		return nil, err
	}
	var jobs []songPartJob
	for _, doc := range docs {
		if doc.deleted {
			continue
		}
		for _, draft := range songPartDrafts(doc.title, doc.parts) {
			jobs = append(jobs, songPartJob{
				SongbookID: doc.songbookID,
				FBID:       doc.fbid,
				Index:      draft.index,
				Text:       draft.text,
				Hash:       draft.hash,
			})
		}
	}
	return jobs, nil
}

func (s *Server) enqueueShadowPassages(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT id::text FROM modules WHERE status = 'approved' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for _, id := range ids {
		verses, err := s.loadVerses(ctx, id)
		if err != nil {
			return err
		}
		var jobs []biblePartJob
		for _, draft := range chunkVerses(verses) {
			jobs = append(jobs, biblePartJob{ModuleID: id, Draft: draft, Hash: hashText(draft.Body)})
		}
		if err := s.enqueueBibleParts(ctx, "next", id, jobs); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) maybeSwitchShadow(ctx context.Context) error {
	if !s.shadowOpen(ctx) {
		return nil
	}
	var left int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM embedding_jobs
		WHERE target = 'next' AND status <> 'done'
	`).Scan(&left); err != nil {
		return err
	}
	if left > 0 {
		return nil
	}
	return s.switchShadow(ctx)
}

func (s *Server) switchShadow(ctx context.Context) error {
	model, err := s.setting(ctx, "embed_shadow_model")
	if err != nil {
		return err
	}
	actor, _ := s.setting(ctx, "embed_shadow_actor")
	live := s.liveEmbed()
	dim := live.Dim
	if dim <= 0 {
		raw, _ := s.setting(ctx, "embed_set_dim")
		dim, _ = strconv.Atoi(raw)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	steps := []string{
		`ALTER INDEX song_embeddings_hnsw RENAME TO song_embeddings_prev_hnsw`,
		`ALTER TABLE song_embeddings RENAME TO song_embeddings_prev`,
		`ALTER TABLE song_embeddings_next RENAME TO song_embeddings`,
		`ALTER INDEX song_embeddings_next_hnsw RENAME TO song_embeddings_hnsw`,
		`DROP TABLE song_embeddings_prev`,
		`ALTER INDEX bible_passages_hnsw RENAME TO bible_passages_prev_hnsw`,
		`ALTER TABLE bible_passages RENAME TO bible_passages_prev`,
		`ALTER TABLE bible_passages_next RENAME TO bible_passages`,
		`ALTER INDEX bible_passages_next_hnsw RENAME TO bible_passages_hnsw`,
		`DROP TABLE bible_passages_prev`,
		`DELETE FROM embedding_jobs WHERE target = 'next'`,
	}
	for _, stmt := range steps {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	if err := s.putSetting(ctx, tx, "embed_set_model", model); err != nil {
		return err
	}
	if err := s.putSetting(ctx, tx, "embed_set_dim", strconv.Itoa(dim)); err != nil {
		return err
	}
	if err := s.putSetting(ctx, tx, "embed_shadow", "0"); err != nil {
		return err
	}
	if actor != "" {
		if err := writeAudit(ctx, tx, actor, "embed_switch", "embed", "queue", model, fmt.Sprintf("dim %d", dim)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
