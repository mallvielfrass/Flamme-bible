package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func communityStaff(ctx context.Context, q planQuerier, userID, dimID string) error {
	var kind string
	err := q.QueryRow(ctx, `SELECT kind FROM dimensions WHERE id = $1`, dimID).Scan(&kind)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if kind != "community" {
		return errRequest
	}
	var site string
	if err := q.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&site); err != nil {
		return err
	}
	if site == "admin" {
		return nil
	}
	role, err := scanMembership(q.QueryRow(ctx, `
		SELECT role, excluded FROM memberships WHERE dimension_id = $1 AND user_id = $2
	`, dimID, userID))
	if err != nil {
		return err
	}
	if role != "admin" {
		return errForbidden
	}
	return nil
}

type communityRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Confession string `json:"confession"`
	Address    string `json:"address"`
	Members    int    `json:"members"`
	Admins     int    `json:"admins"`
	Songbooks  int    `json:"songbooks"`
	Plans      int    `json:"plans"`
}

func (s *Server) adminCommunities(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, offset, err := pageWindow(r, 50, 200)
	if err != nil {
		writeMapped(w, err)
		return
	}
	ctx := r.Context()
	like := ""
	if q != "" {
		like = likeContains(q)
	}
	var total int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM dimensions d
		WHERE d.kind = 'community' AND ($1 = '' OR d.name ILIKE $1 OR d.address ILIKE $1)
	`, like).Scan(&total); err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(ctx, `
		SELECT d.id::text, d.name, d.confession, d.address,
		       (SELECT count(*) FROM memberships m WHERE m.dimension_id = d.id AND NOT m.excluded),
		       (SELECT count(*) FROM memberships m WHERE m.dimension_id = d.id AND m.role = 'admin' AND NOT m.excluded),
		       (SELECT count(*) FROM songbooks b WHERE b.dimension_id = d.id),
		       (SELECT count(*) FROM plans p WHERE p.dimension_id = d.id AND NOT p.deleted)
		FROM dimensions d
		WHERE d.kind = 'community' AND ($1 = '' OR d.name ILIKE $1 OR d.address ILIKE $1)
		ORDER BY d.name, d.id
		LIMIT $2 OFFSET $3
	`, like, limit, offset)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []communityRow{}
	for rows.Next() {
		var item communityRow
		if err := rows.Scan(&item.ID, &item.Name, &item.Confession, &item.Address, &item.Members, &item.Admins, &item.Songbooks, &item.Plans); err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"communities": list, "total": total})
}

type memberRow struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Excluded bool   `json:"excluded"`
}

func (s *Server) communityMembers(w http.ResponseWriter, r *http.Request) {
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
	if err := communityStaff(r.Context(), s.db, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT u.id::text, u.email, m.role, m.excluded
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.dimension_id = $1
		ORDER BY m.excluded, u.email
	`, dimID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []memberRow{}
	for rows.Next() {
		var item memberRow
		if err := rows.Scan(&item.UserID, &item.Email, &item.Role, &item.Excluded); err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": list})
}

func (s *Server) apiExcludeMember(w http.ResponseWriter, r *http.Request) {
	s.setExcluded(w, r, true)
}

func (s *Server) apiIncludeMember(w http.ResponseWriter, r *http.Request) {
	s.setExcluded(w, r, false)
}

func (s *Server) setExcluded(w http.ResponseWriter, r *http.Request, excluded bool) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	target := r.PathValue("userID")
	if _, err := uuid.Parse(dimID); err != nil || uuid.Validate(target) != nil {
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
	if err := communityStaff(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	var role string
	var was bool
	err = tx.QueryRow(ctx, `SELECT role, excluded FROM memberships WHERE dimension_id = $1 AND user_id = $2 FOR UPDATE`, dimID, target).Scan(&role, &was)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if excluded && role == "admin" && !was {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE dimension_id = $1 AND role = 'admin' AND NOT excluded`, dimID).Scan(&admins); err != nil {
			writeMapped(w, err)
			return
		}
		if admins <= 1 {
			writeMapped(w, errConflict)
			return
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE memberships SET excluded = $3 WHERE dimension_id = $1 AND user_id = $2`, dimID, target, excluded); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "excluded": excluded})
}

func (s *Server) setMemberRole(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	target := r.PathValue("userID")
	if _, err := uuid.Parse(dimID); err != nil || uuid.Validate(target) != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &body); err != nil || (body.Role != "admin" && body.Role != "member") {
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
	if err := communityStaff(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	var current string
	err = tx.QueryRow(ctx, `SELECT role FROM memberships WHERE dimension_id = $1 AND user_id = $2 FOR UPDATE`, dimID, target).Scan(&current)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if current == "admin" && body.Role != "admin" {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE dimension_id = $1 AND role = 'admin' AND NOT excluded`, dimID).Scan(&admins); err != nil {
			writeMapped(w, err)
			return
		}
		if admins <= 1 {
			writeMapped(w, errConflict)
			return
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE memberships SET role = $3 WHERE dimension_id = $1 AND user_id = $2`, dimID, target, body.Role); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"role": body.Role})
}

type inviteRow struct {
	Digest    string    `json:"digest"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
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
	if err := communityStaff(r.Context(), s.db, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT encode(digest, 'hex'), expires_at
		FROM invites
		WHERE dimension_id = $1 AND used_at IS NULL AND expires_at > now()
		ORDER BY expires_at
	`, dimID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []inviteRow{}
	for rows.Next() {
		var item inviteRow
		if err := rows.Scan(&item.Digest, &item.ExpiresAt); err != nil {
			writeMapped(w, err)
			return
		}
		item.ExpiresAt = item.ExpiresAt.UTC()
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": list})
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	digest := strings.ToLower(r.PathValue("digest"))
	if _, err := uuid.Parse(dimID); err != nil || len(digest) != 64 {
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
	if err := communityStaff(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM invites
		WHERE dimension_id = $1 AND encode(digest, 'hex') = $2 AND used_at IS NULL
	`, dimID, digest)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeMapped(w, errNotFound)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
