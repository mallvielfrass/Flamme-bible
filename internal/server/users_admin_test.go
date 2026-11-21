package server

import (
	"context"
	"net/http"
	"testing"
)

func TestAdminUsers(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	book := createBook(t, srv, other.Token, other.DimensionID, map[string]any{"fbid": "book", "title": "Личный"})

	code, raw := api(t, srv, other.Token, http.MethodGet, "/api/admin/users", nil)
	if code != http.StatusForbidden {
		t.Fatalf("чужой список %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+admin.UserID+"/role", map[string]any{"role": "user"})
	if code != http.StatusConflict {
		t.Fatalf("самопонижение %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+other.UserID+"/role", map[string]any{"role": "moderator"})
	if code != http.StatusOK {
		t.Fatalf("модератор %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+other.UserID+"/role", map[string]any{"role": "admin"})
	if code != http.StatusOK {
		t.Fatalf("второй админ %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+other.UserID+"/role", map[string]any{"role": "user"})
	if code != http.StatusOK {
		t.Fatalf("понижение второго %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+admin.UserID+"/block", nil)
	if code != http.StatusConflict {
		t.Fatalf("блок себя %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+other.UserID+"/block", nil)
	if code != http.StatusOK {
		t.Fatalf("блок %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/me", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("сессия после блока %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{"email": "other@example.com", "password": "secret-pass"})
	if code != http.StatusUnauthorized {
		t.Fatalf("вход заблокированного %d %s", code, raw)
	}
	var left int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM songbooks WHERE id = $1`, book.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatal("блок стёр сборник")
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+other.UserID+"/unblock", nil)
	if code != http.StatusOK {
		t.Fatalf("разблок %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{"email": "other@example.com", "password": "secret-pass"})
	if code != http.StatusOK {
		t.Fatalf("вход после разблока %d %s", code, raw)
	}
	var logged authResp
	mustJSON(t, raw, &logged)
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+other.UserID+"/sessions/revoke", nil)
	if code != http.StatusOK {
		t.Fatalf("сессии %d %s", code, raw)
	}
	code, raw = api(t, srv, logged.Token, http.MethodGet, "/api/me", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("токен после отзыва %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/users?q=other&role=user", nil)
	if code != http.StatusOK {
		t.Fatalf("список %d %s", code, raw)
	}
	var page struct {
		Users []adminUser `json:"users"`
		Total int         `json:"total"`
	}
	mustJSON(t, raw, &page)
	if page.Total != 1 || page.Users[0].Email != "other@example.com" || page.Users[0].CreatedAt.IsZero() {
		t.Fatalf("страница %+v", page)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/users/"+admin.UserID, nil)
	if code != http.StatusConflict {
		t.Fatalf("удаление себя %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/users/"+other.UserID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление %d %s", code, raw)
	}
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE email = 'other@example.com'`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("пользователь остался")
	}
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM songbooks WHERE id = $1`, book.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("личный сборник остался")
	}
}
