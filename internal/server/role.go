package server

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Пароль администратора не хранится в окружении: его выбирают при регистрации.
// ADMIN_EMAIL только отмечает, какой адрес получает role=admin.
func promoteBootstrapAdmin(ctx context.Context, tx pgx.Tx, userID, email string) error {
	want := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	if want == "" || strings.ToLower(strings.TrimSpace(email)) != want {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE users SET role = 'admin' WHERE id = $1 AND role <> 'admin'`, userID)
	return err
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (principal, bool) {
	return s.requireRole(w, r, "admin")
}

func (s *Server) requireModerator(w http.ResponseWriter, r *http.Request) (principal, bool) {
	return s.requireRole(w, r, "admin", "moderator")
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, allowed ...string) (principal, bool) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return principal{}, false
	}
	var role string
	err = s.db.QueryRow(r.Context(), `SELECT role FROM users WHERE id = $1`, p.UserID).Scan(&role)
	if err != nil {
		writeMapped(w, err)
		return principal{}, false
	}
	for _, want := range allowed {
		if role == want {
			return p, true
		}
	}
	writeMapped(w, errForbidden)
	return principal{}, false
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var email, role string
	err = s.db.QueryRow(r.Context(), `SELECT email, role FROM users WHERE id = $1`, p.UserID).Scan(&email, &role)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id": p.UserID,
		"email":   email,
		"role":    role,
	})
}
