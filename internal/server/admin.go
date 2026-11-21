package server

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type moduleView struct {
	ID             string     `json:"id"`
	OwnerUserID    *string    `json:"owner_user_id"`
	DimensionID    *string    `json:"dimension_id"`
	ShortName      string     `json:"short_name"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	ModerationNote string     `json:"moderation_note"`
	ReviewedBy     *string    `json:"reviewed_by"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type submission struct {
	Kind           string     `json:"kind"`
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	ShortName      string     `json:"short_name,omitempty"`
	Status         string     `json:"status"`
	ModerationNote string     `json:"moderation_note"`
	DimensionID    *string    `json:"dimension_id"`
	OwnerUserID    *string    `json:"owner_user_id"`
	CommunityName  string     `json:"community_name,omitempty"`
	AuthorEmail    string     `json:"author_email,omitempty"`
	SongsCount     int        `json:"songs_count,omitempty"`
	BooksCount     int        `json:"books_count,omitempty"`
	PatchCount     int        `json:"patch_count,omitempty"`
	Publisher      string     `json:"publisher,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`
	HiddenAt       *time.Time `json:"hidden_at,omitempty"`
	IndexPending   int        `json:"index_pending,omitempty"`
	IndexFailed    int        `json:"index_failed,omitempty"`
}

func markSongbookPending(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `
		UPDATE songbooks
		SET publish_requested = true,
		    status = CASE WHEN status = 'approved' THEN status ELSE 'pending' END,
		    submitted_at = CASE WHEN status = 'approved' THEN submitted_at ELSE now() END
		WHERE id = $1
	`, id)
	return err
}

func (s *Server) createModule(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		ShortName   string `json:"short_name"`
		Title       string `json:"title"`
		DimensionID string `json:"dimension_id"`
	}
	if err := readJSON(r, &body); err != nil || !validModuleName(strings.TrimSpace(body.ShortName)) {
		writeMapped(w, errRequest)
		return
	}
	title := strings.TrimSpace(body.Title)
	if utf8.RuneCountInString(title) > 200 {
		writeMapped(w, errRequest)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var owner any
	var dim any
	if body.DimensionID == "" {
		owner = p.UserID
	} else {
		if _, err := uuid.Parse(body.DimensionID); err != nil {
			writeMapped(w, errNotFound)
			return
		}
		if _, err := membershipTx(ctx, tx, p.UserID, body.DimensionID); err != nil {
			writeMapped(w, err)
			return
		}
		var kind string
		if err := tx.QueryRow(ctx, `SELECT kind FROM dimensions WHERE id = $1`, body.DimensionID).Scan(&kind); err != nil {
			writeMapped(w, err)
			return
		}
		if kind != "community" {
			writeMapped(w, errRequest)
			return
		}
		dim = body.DimensionID
	}
	id := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO modules (id, owner_user_id, dimension_id, short_name, title, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'draft', now())
	`, id, owner, dim, strings.TrimSpace(body.ShortName), title); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := loadModule(ctx, tx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) listModules(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT id::text FROM modules
		WHERE owner_user_id = $1
		   OR dimension_id IN (SELECT dimension_id FROM memberships WHERE user_id = $1 AND NOT excluded)
		ORDER BY updated_at DESC, id
	`, p.UserID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []moduleView{}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			writeMapped(w, err)
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	rows.Close()
	for _, id := range ids {
		view, err := loadModule(r.Context(), s.db, id)
		if err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, view)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getModule(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	view, err := loadModule(r.Context(), s.db, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := moduleAccess(r.Context(), s.db, p.UserID, view); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) submitModule(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
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
	view, err := lockModule(ctx, tx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := moduleAccess(ctx, tx, p.UserID, view); err != nil {
		writeMapped(w, err)
		return
	}
	if view.Status != "draft" && view.Status != "rejected" {
		writeMapped(w, errConflict)
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE modules
		SET status = 'pending', moderation_note = '', reviewed_by = NULL, reviewed_at = NULL,
		    submitted_at = now(), updated_at = now()
		WHERE id = $1
	`, id); err != nil {
		writeMapped(w, err)
		return
	}
	view, err = loadModule(ctx, tx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) submitSongbook(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
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
	var dimID, status string
	err = tx.QueryRow(ctx, `SELECT dimension_id::text, status FROM songbooks WHERE id = $1 FOR UPDATE`, id).Scan(&dimID, &status)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := membershipTx(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	if status != "draft" && status != "rejected" && status != "pending" {
		writeMapped(w, errConflict)
		return
	}
	if err := markSongbookPending(ctx, tx, id); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := scanBook(tx.QueryRow(ctx, `SELECT `+bookCols+` FROM songbooks WHERE id = $1`, id))
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) publicSongbooks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT `+bookCols+` FROM songbooks
		WHERE status = 'approved' AND public AND hidden_at IS NULL
		ORDER BY title, id
	`)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []songbookView{}
	for rows.Next() {
		view, err := scanBook(rows)
		if err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, view)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) publicModules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id::text FROM modules WHERE status = 'approved' ORDER BY short_name, id
	`)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			writeMapped(w, err)
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	rows.Close()
	list := []moduleView{}
	for _, id := range ids {
		view, err := loadModule(r.Context(), s.db, id)
		if err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, view)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminSubmissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireModerator(w, r); !ok {
		return
	}
	s.writeSubmissions(w, r)
}

func (s *Server) adminApproveSongbook(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	s.moderateSongbook(w, r, p, "approved", "")
}

func (s *Server) adminRejectSongbook(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	note, ok := readNote(w, r, true)
	if !ok {
		return
	}
	s.moderateSongbook(w, r, p, "rejected", note)
}

func (s *Server) adminRevokeSongbook(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	note, ok := readNote(w, r, false)
	if !ok {
		return
	}
	s.moderateSongbook(w, r, p, "draft", note)
}

func (s *Server) adminApproveModule(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	s.moderateModule(w, r, p, "approved", "")
}

func (s *Server) adminRejectModule(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	note, ok := readNote(w, r, true)
	if !ok {
		return
	}
	s.moderateModule(w, r, p, "rejected", note)
}

func (s *Server) adminRevokeModule(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	note, ok := readNote(w, r, false)
	if !ok {
		return
	}
	s.moderateModule(w, r, p, "draft", note)
}

func (s *Server) moderateSongbook(w http.ResponseWriter, r *http.Request, p principal, status, note string) {
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
	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM songbooks WHERE id = $1 FOR UPDATE`, id).Scan(&current)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if !transitionOK(current, status) {
		writeMapped(w, errConflict)
		return
	}
	public := status == "approved"
	if _, err := tx.Exec(ctx, `
		UPDATE songbooks
		SET status = $2, public = $3, publish_requested = false, moderation_note = $4, reviewed_by = $5, reviewed_at = now()
		WHERE id = $1
	`, id, status, public, note, p.UserID); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := scanBook(tx.QueryRow(ctx, `SELECT `+bookCols+` FROM songbooks WHERE id = $1`, id))
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, moderationAction(status), "songbook", id, view.Title, note); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) moderateModule(w http.ResponseWriter, r *http.Request, p principal, status, note string) {
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
	view, err := lockModule(ctx, tx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if !transitionOK(view.Status, status) {
		writeMapped(w, errConflict)
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE modules
		SET status = $2, moderation_note = $3, reviewed_by = $4, reviewed_at = now(), updated_at = now()
		WHERE id = $1
	`, id, status, note, p.UserID); err != nil {
		writeMapped(w, err)
		return
	}
	view, err = loadModule(ctx, tx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, moderationAction(status), "module", id, view.Title, note); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	s.noteModule(id, status)
	writeJSON(w, http.StatusOK, view)
}

func readNote(w http.ResponseWriter, r *http.Request, required bool) (string, bool) {
	if !required && r.ContentLength == 0 {
		return "", true
	}
	var body struct {
		Note string `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return "", false
	}
	note := strings.TrimSpace(body.Note)
	if required && note == "" {
		writeMapped(w, errRequest)
		return "", false
	}
	if utf8.RuneCountInString(note) > 2000 {
		writeMapped(w, errRequest)
		return "", false
	}
	return note, true
}

func validModerationStatus(status string) bool {
	switch status {
	case "draft", "pending", "approved", "rejected", "hidden", "all":
		return true
	default:
		return false
	}
}

func moderationAction(status string) string {
	switch status {
	case "approved":
		return "approve"
	case "rejected":
		return "reject"
	default:
		return "revoke"
	}
}

func transitionOK(from, to string) bool {
	switch to {
	case "approved", "rejected":
		return from == "pending" || from == "rejected"
	case "draft":
		return from == "approved"
	default:
		return false
	}
}

func moduleAccess(ctx context.Context, q planQuerier, userID string, view moduleView) error {
	if view.OwnerUserID != nil {
		if *view.OwnerUserID == userID {
			return nil
		}
		return errNotFound
	}
	if view.DimensionID == nil {
		return errNotFound
	}
	_, err := scanMembership(q.QueryRow(ctx, `
		SELECT role, excluded FROM memberships WHERE dimension_id = $1 AND user_id = $2
	`, *view.DimensionID, userID))
	return err
}

func lockModule(ctx context.Context, tx pgx.Tx, id string) (moduleView, error) {
	var ignored string
	err := tx.QueryRow(ctx, `SELECT id::text FROM modules WHERE id = $1 FOR UPDATE`, id).Scan(&ignored)
	if err == pgx.ErrNoRows {
		return moduleView{}, errNotFound
	}
	if err != nil {
		return moduleView{}, err
	}
	return loadModule(ctx, tx, id)
}

func loadModule(ctx context.Context, q planQuerier, id string) (moduleView, error) {
	var view moduleView
	err := q.QueryRow(ctx, `
		SELECT id::text, owner_user_id::text, dimension_id::text, short_name, title, status,
		       moderation_note, reviewed_by::text, reviewed_at, created_at, updated_at
		FROM modules WHERE id = $1
	`, id).Scan(
		&view.ID, &view.OwnerUserID, &view.DimensionID, &view.ShortName, &view.Title, &view.Status,
		&view.ModerationNote, &view.ReviewedBy, &view.ReviewedAt, &view.CreatedAt, &view.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return moduleView{}, errNotFound
	}
	if err != nil {
		return moduleView{}, err
	}
	view.CreatedAt = view.CreatedAt.UTC()
	view.UpdatedAt = view.UpdatedAt.UTC()
	if view.ReviewedAt != nil {
		t := view.ReviewedAt.UTC()
		view.ReviewedAt = &t
	}
	return view, nil
}
