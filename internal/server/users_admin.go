package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type adminUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Blocked     bool       `json:"blocked"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	Communities int        `json:"communities"`
	Devices     int        `json:"devices"`
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	role := r.URL.Query().Get("role")
	blocked := r.URL.Query().Get("blocked")
	if role != "" && role != "user" && role != "moderator" && role != "admin" {
		writeMapped(w, errRequest)
		return
	}
	if blocked != "" && blocked != "0" && blocked != "1" {
		writeMapped(w, errRequest)
		return
	}
	limit, offset, err := pageWindow(r, 50, 200)
	if err != nil {
		writeMapped(w, err)
		return
	}
	ctx := r.Context()
	where, args := userFilter(q, role, blocked)
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM users u WHERE `+where, args...).Scan(&total); err != nil {
		writeMapped(w, err)
		return
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(ctx, `
		SELECT u.id::text, u.email, u.role, u.blocked_at IS NOT NULL,
		       u.created_at, u.last_seen_at,
		       (SELECT count(*) FROM memberships m
		         JOIN dimensions d ON d.id = m.dimension_id
		         WHERE m.user_id = u.id AND d.kind = 'community' AND NOT m.excluded),
		       (SELECT count(*) FROM devices d WHERE d.user_id = u.id)
		FROM users u
		WHERE `+where+`
		ORDER BY u.email, u.id
		LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []adminUser{}
	for rows.Next() {
		var item adminUser
		if err := rows.Scan(&item.ID, &item.Email, &item.Role, &item.Blocked, &item.CreatedAt, &item.LastSeenAt, &item.Communities, &item.Devices); err != nil {
			writeMapped(w, err)
			return
		}
		item.CreatedAt = item.CreatedAt.UTC()
		if item.LastSeenAt != nil {
			seen := item.LastSeenAt.UTC()
			item.LastSeenAt = &seen
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	counts, err := s.userCounts(ctx, q)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": list, "total": total, "counts": counts})
}

func userFilter(q, role, blocked string) (string, []any) {
	parts := []string{"TRUE"}
	args := []any{}
	if q != "" {
		args = append(args, likeContains(q))
		parts = append(parts, "u.email ILIKE $"+strconv.Itoa(len(args)))
	}
	if role != "" {
		args = append(args, role)
		parts = append(parts, "u.role = $"+strconv.Itoa(len(args)))
	}
	if blocked == "1" {
		parts = append(parts, "u.blocked_at IS NOT NULL")
	}
	if blocked == "0" {
		parts = append(parts, "u.blocked_at IS NULL")
	}
	return strings.Join(parts, " AND "), args
}

func (s *Server) userCounts(ctx context.Context, q string) (map[string]int, error) {
	where, args := userFilter(q, "", "")
	var all, admin, mod, blocked int
	err := s.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE role = 'admin'),
		       count(*) FILTER (WHERE role = 'moderator'),
		       count(*) FILTER (WHERE blocked_at IS NOT NULL)
		FROM users u WHERE `+where, args...).Scan(&all, &admin, &mod, &blocked)
	if err != nil {
		return nil, err
	}
	return map[string]int{"all": all, "admin": admin, "moderator": mod, "blocked": blocked}, nil
}

func (s *Server) adminSetRole(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	if body.Role != "user" && body.Role != "moderator" && body.Role != "admin" {
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
	var current string
	err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&current)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if id == p.UserID && body.Role != "admin" {
		writeMapped(w, errConflict)
		return
	}
	if current == "admin" && body.Role != "admin" {
		if err := keepAdmin(ctx, tx); err != nil {
			writeMapped(w, err)
			return
		}
	}
	var email string
	if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, id).Scan(&email); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, id, body.Role); err != nil {
		writeMapped(w, err)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, "role", "user", id, email, body.Role); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "role": body.Role})
}

func keepAdmin(ctx context.Context, tx pgx.Tx) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return errConflict
	}
	return nil
}

func (s *Server) adminBlockUser(w http.ResponseWriter, r *http.Request) {
	s.setBlocked(w, r, true)
}

func (s *Server) adminUnblockUser(w http.ResponseWriter, r *http.Request) {
	s.setBlocked(w, r, false)
}

func (s *Server) setBlocked(w http.ResponseWriter, r *http.Request, block bool) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if id == p.UserID {
		writeMapped(w, errConflict)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var role, email string
	err = tx.QueryRow(ctx, `SELECT role, email FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&role, &email)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if block && role == "admin" {
		writeMapped(w, errConflict)
		return
	}
	query := `UPDATE users SET blocked_at = NULL WHERE id = $1`
	if block {
		query = `UPDATE users SET blocked_at = now() WHERE id = $1`
	}
	if _, err := tx.Exec(ctx, query, id); err != nil {
		writeMapped(w, err)
		return
	}
	if block {
		if _, err := tx.Exec(ctx, `UPDATE session_families SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
			writeMapped(w, err)
			return
		}
	}
	action := "unblock"
	if block {
		action = "block"
	}
	if err := writeAudit(ctx, tx, p.UserID, action, "user", id, email, ""); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "blocked": block})
}

func (s *Server) adminRevokeSessions(w http.ResponseWriter, r *http.Request) {
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
	var email string
	err = tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, id).Scan(&email)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE session_families SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, "sessions_revoke", "user", id, email, ""); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": tag.RowsAffected()})
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if id == p.UserID {
		writeMapped(w, errConflict)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var role, email string
	err = tx.QueryRow(ctx, `SELECT role, email FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&role, &email)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if role == "admin" {
		if err := keepAdmin(ctx, tx); err != nil {
			writeMapped(w, err)
			return
		}
	}
	if err := writeAudit(ctx, tx, p.UserID, "delete", "user", id, email, ""); err != nil {
		writeMapped(w, err)
		return
	}
	if err := deleteUserGraph(ctx, tx, id); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func deleteUserGraph(ctx context.Context, tx pgx.Tx, userID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM ai_task_models WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ai_providers WHERE user_id = $1`, userID); err != nil {
		return err
	}
	var personal string
	err := tx.QueryRow(ctx, `
		SELECT d.id::text FROM dimensions d
		JOIN memberships m ON m.dimension_id = d.id
		WHERE m.user_id = $1 AND d.kind = 'personal'
		LIMIT 1
	`, userID).Scan(&personal)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if personal != "" {
		if err := eraseDimension(ctx, tx, personal); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM modules WHERE owner_user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM plan_items WHERE plan_id IN (SELECT id FROM plans WHERE owner_user_id = $1)`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM plans WHERE owner_user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM song_hides WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM memberships WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM operation_results WHERE device_id IN (SELECT id FROM devices WHERE user_id = $1)`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM session_tokens WHERE family_id IN (SELECT id FROM session_families WHERE user_id = $1)`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM session_families WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM devices WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

func eraseDimension(ctx context.Context, tx pgx.Tx, dimID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE songbooks SET parent_id = NULL
		WHERE parent_id IN (SELECT id FROM songbooks WHERE dimension_id = $1)
	`, dimID); err != nil {
		return err
	}
	for {
		var id string
		err := tx.QueryRow(ctx, `
			SELECT id::text FROM songbooks
			WHERE dimension_id = $1
			  AND NOT EXISTS (SELECT 1 FROM songbooks child WHERE child.parent_id = songbooks.id)
			LIMIT 1
		`, dimID).Scan(&id)
		if err == pgx.ErrNoRows {
			break
		}
		if err != nil {
			return err
		}
		if err := eraseSongbook(ctx, tx, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM modules WHERE dimension_id = $1`, dimID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM plan_items WHERE plan_id IN (SELECT id FROM plans WHERE dimension_id = $1)`, dimID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM plans WHERE dimension_id = $1`, dimID); err != nil {
		return err
	}
	for _, table := range []string{"invites", "conflicts", "change_log", "operation_results", "sync_counters", "memberships"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE dimension_id = $1`, dimID); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `DELETE FROM dimensions WHERE id = $1`, dimID)
	return err
}

func eraseSongbook(ctx context.Context, tx pgx.Tx, id string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM song_hides WHERE songbook_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM conflicts WHERE songbook_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM change_log WHERE songbook_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM song_order WHERE songbook_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM songs WHERE songbook_id = $1`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM songbooks WHERE id = $1`, id)
	return err
}
