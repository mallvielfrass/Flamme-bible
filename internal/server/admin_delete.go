package server

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) adminDeleteSongbook(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var title string
	err = tx.QueryRow(ctx, `SELECT title FROM songbooks WHERE id = $1 FOR UPDATE`, id).Scan(&title)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	var patches int
	if err := tx.QueryRow(ctx, `
		WITH RECURSIVE tree AS (
			SELECT id FROM songbooks WHERE parent_id = $1
			UNION ALL
			SELECT child.id FROM songbooks child JOIN tree ON child.parent_id = tree.id
		)
		SELECT count(*) FROM tree
	`, id).Scan(&patches); err != nil {
		writeMapped(w, err)
		return
	}
	if patches > 0 {
		if _, err := tx.Exec(ctx, `
			WITH RECURSIVE tree AS (
				SELECT id FROM songbooks WHERE id = $1
				UNION ALL
				SELECT child.id FROM songbooks child JOIN tree ON child.parent_id = tree.id
			)
			UPDATE songbooks SET hidden_at = COALESCE(hidden_at, now())
			WHERE id IN (SELECT id FROM tree)
		`, id); err != nil {
			writeMapped(w, err)
			return
		}
		if err := writeAudit(ctx, tx, p.UserID, "hide", "songbook", id, title, ""); err != nil {
			writeMapped(w, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeMapped(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "hidden": true, "patches": patches})
		return
	}
	if err := eraseSongbook(ctx, tx, id); err != nil {
		writeMapped(w, err)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, "delete", "songbook", id, title, ""); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true})
}

func (s *Server) adminUnhideSongbook(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		WITH RECURSIVE tree AS (
			SELECT id FROM songbooks WHERE id = $1
			UNION ALL
			SELECT child.id FROM songbooks child JOIN tree ON child.parent_id = tree.id
		)
		UPDATE songbooks SET hidden_at = NULL WHERE id IN (SELECT id FROM tree)
	`, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeMapped(w, errNotFound)
		return
	}
	var title string
	_ = tx.QueryRow(ctx, `SELECT title FROM songbooks WHERE id = $1`, id).Scan(&title)
	if err := writeAudit(ctx, tx, p.UserID, "unhide", "songbook", id, title, ""); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminDeleteModule(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var title string
	err = tx.QueryRow(ctx, `SELECT title FROM modules WHERE id = $1 FOR UPDATE`, id).Scan(&title)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	tag, err := tx.Exec(ctx, `DELETE FROM modules WHERE id = $1`, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeMapped(w, errNotFound)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, "delete", "module", id, title, ""); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminDeleteCommunity(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var kind, name string
	err = tx.QueryRow(ctx, `SELECT kind, name FROM dimensions WHERE id = $1 FOR UPDATE`, id).Scan(&kind, &name)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if kind != "community" {
		writeMapped(w, errRequest)
		return
	}
	if err := eraseDimension(ctx, tx, id); err != nil {
		writeMapped(w, err)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, "delete", "community", id, name, ""); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
