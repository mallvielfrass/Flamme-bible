package server

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type dimensionView struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Email       string `json:"email"`
	Confession  string `json:"confession"`
	Description string `json:"description"`
}

func createDimension(ctx context.Context, tx pgx.Tx, kind, name, address, email, confession, description, userID, role string) (string, error) {
	id := uuid.NewString()
	_, err := tx.Exec(ctx, `
		INSERT INTO dimensions (id, kind, name, address, email, confession, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, kind, name, address, email, confession, description)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO memberships (dimension_id, user_id, role, excluded) VALUES ($1, $2, $3, false)
	`, id, userID, role); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sync_counters (dimension_id, seq) VALUES ($1, 0)`, id); err != nil {
		return "", err
	}
	return id, nil
}

func scanMembership(row pgx.Row) (string, error) {
	var role string
	var excluded bool
	err := row.Scan(&role, &excluded)
	if err == pgx.ErrNoRows {
		return "", errNotFound
	}
	if err != nil {
		return "", err
	}
	if excluded {
		return "", errForbidden
	}
	return role, nil
}

func membershipTx(ctx context.Context, tx pgx.Tx, userID, dimID string) (string, error) {
	return scanMembership(tx.QueryRow(ctx, `
		SELECT role, excluded FROM memberships WHERE dimension_id = $1 AND user_id = $2
	`, dimID, userID))
}

func (s *Server) membership(ctx context.Context, userID, dimID string) (string, error) {
	return scanMembership(s.db.QueryRow(ctx, `
		SELECT role, excluded FROM memberships WHERE dimension_id = $1 AND user_id = $2
	`, dimID, userID))
}

func (s *Server) createCommunity(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		Name        string `json:"name"`
		Address     string `json:"address"`
		Email       string `json:"email"`
		Confession  string `json:"confession"`
		Description string `json:"description"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	id, err := createDimension(ctx, tx, "community", body.Name, body.Address, body.Email, body.Confession, body.Description, p.UserID, "admin")
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dimensionView{
		ID: id, Kind: "community", Name: body.Name, Address: body.Address,
		Email: body.Email, Confession: body.Confession, Description: body.Description,
	})
}

func (s *Server) getDimension(w http.ResponseWriter, r *http.Request) {
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
	if _, err := s.membership(r.Context(), p.UserID, id); err != nil {
		writeMapped(w, err)
		return
	}
	var view dimensionView
	err = s.db.QueryRow(r.Context(), `
		SELECT id::text, kind, name, address, email, confession, description FROM dimensions WHERE id = $1
	`, id).Scan(&view.ID, &view.Kind, &view.Name, &view.Address, &view.Email, &view.Confession, &view.Description)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	if _, err := uuid.Parse(dimID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		TTL int64 `json:"ttl_seconds"`
	}
	if err := readJSON(r, &body); err != nil || body.TTL <= 0 || body.TTL > 366*24*3600 {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	role, err := membershipTx(ctx, tx, p.UserID, dimID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if role != "admin" {
		writeMapped(w, errForbidden)
		return
	}
	var kind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM dimensions WHERE id = $1`, dimID).Scan(&kind); err != nil {
		writeMapped(w, err)
		return
	}
	if kind != "community" {
		writeJSON(w, http.StatusBadRequest, errJSON("община"))
		return
	}
	token, err := newSecret()
	if err != nil {
		writeMapped(w, err)
		return
	}
	var expires time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO invites (digest, dimension_id, expires_at)
		VALUES ($1, $2, now() + make_interval(secs => $3))
		RETURNING expires_at
	`, tokenDigest(token), dimID, body.TTL).Scan(&expires)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": expires})
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	token := r.PathValue("token")
	if token == "" {
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
	var dimID string
	var used *time.Time
	var expired bool
	err = tx.QueryRow(ctx, `
		SELECT dimension_id::text, used_at, expires_at <= now()
		FROM invites WHERE digest = $1 FOR UPDATE
	`, tokenDigest(token)).Scan(&dimID, &used, &expired)
	if err == pgx.ErrNoRows || (err == nil && (used != nil || expired)) {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	var excluded bool
	err = tx.QueryRow(ctx, `SELECT excluded FROM memberships WHERE dimension_id = $1 AND user_id = $2`, dimID, p.UserID).Scan(&excluded)
	if err == nil && !excluded {
		writeMapped(w, errMember)
		return
	}
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
			INSERT INTO memberships (dimension_id, user_id, role, excluded) VALUES ($1, $2, 'member', false)
		`, dimID, p.UserID)
	} else if err == nil {
		_, err = tx.Exec(ctx, `
			UPDATE memberships SET excluded = false, role = 'member' WHERE dimension_id = $1 AND user_id = $2
		`, dimID, p.UserID)
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE invites SET used_at = now() WHERE digest = $1`, tokenDigest(token)); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"dimension_id": dimID})
}

func (s *Server) excludeMember(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	target := r.PathValue("userID")
	if _, err := uuid.Parse(dimID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if _, err := uuid.Parse(target); err != nil {
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
	role, err := membershipTx(ctx, tx, p.UserID, dimID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if role != "admin" {
		writeMapped(w, errForbidden)
		return
	}
	var ignored string
	err = tx.QueryRow(ctx, `
		SELECT role FROM memberships WHERE dimension_id = $1 AND user_id = $2 FOR UPDATE
	`, dimID, target).Scan(&ignored)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE memberships SET excluded = true WHERE dimension_id = $1 AND user_id = $2`, dimID, target); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
