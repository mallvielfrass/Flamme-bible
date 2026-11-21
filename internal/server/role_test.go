package server

import (
	"context"
	"net/http"
	"testing"
)

func TestRolesAndBootstrapAdmin(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "user@example.com", "secret-pass", map[string]any{"role": "admin"})
	code, raw := api(t, srv, user.Token, http.MethodGet, "/api/me", nil)
	if code != http.StatusOK {
		t.Fatalf("me %d %s", code, raw)
	}
	var me struct {
		Role string `json:"role"`
	}
	mustJSON(t, raw, &me)
	if me.Role != "user" {
		t.Fatalf("роль из тела запроса принята: %s", me.Role)
	}
	code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{
		"email": "user@example.com", "password": "secret-pass",
	})
	if code != http.StatusOK {
		t.Fatalf("login %d %s", code, raw)
	}
	var logged struct {
		Role  string `json:"role"`
		Token string `json:"token"`
	}
	mustJSON(t, raw, &logged)
	if logged.Role != "user" || logged.Token == "" {
		t.Fatalf("вход %+v", logged)
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, "/api/admin/releases", nil)
	if code != http.StatusForbidden {
		t.Fatalf("пользователь в админке %d %s", code, raw)
	}

	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/me", nil)
	if code != http.StatusOK {
		t.Fatalf("admin me %d %s", code, raw)
	}
	mustJSON(t, raw, &me)
	if me.Role != "admin" {
		t.Fatalf("bootstrap %s", me.Role)
	}

	t.Setenv("ADMIN_EMAIL", "user@example.com")
	code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{
		"email": "user@example.com", "password": "secret-pass",
	})
	if code != http.StatusOK {
		t.Fatalf("повторный вход %d %s", code, raw)
	}
	mustJSON(t, raw, &logged)
	if logged.Role != "admin" {
		t.Fatalf("вход не поднял роль %s", logged.Role)
	}
}

func TestModeratorQueueNotReleases(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	mod := registerUser(t, srv, "mod@example.com", "secret-pass", nil)
	if _, err := testPool.Exec(context.Background(), `UPDATE users SET role = 'moderator' WHERE email = 'mod@example.com'`); err != nil {
		t.Fatal(err)
	}
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "На проверку"})
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("submit %d %s", code, raw)
	}
	code, raw = api(t, srv, mod.Token, http.MethodGet, "/api/admin/submissions", nil)
	if code != http.StatusOK {
		t.Fatalf("модератор очередь %d %s", code, raw)
	}
	code, raw = api(t, srv, mod.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("модератор одобряет %d %s", code, raw)
	}
	code, raw = api(t, srv, mod.Token, http.MethodGet, "/api/admin/releases", nil)
	if code != http.StatusForbidden {
		t.Fatalf("модератор в релизах %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/admin/summary", nil)
	if code != http.StatusForbidden {
		t.Fatalf("пользователь в сводке %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/summary", nil)
	if code != http.StatusOK {
		t.Fatalf("админ сводка %d %s", code, raw)
	}
}

func TestCommunitiesRoutes(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	guest := registerUser(t, srv, "guest@example.com", "secret-pass", nil)
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/communities", map[string]any{
		"name": "Община", "address": "ул. Примерная",
	})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var created dimensionView
	mustJSON(t, raw, &created)
	if created.Kind != "community" || created.Name != "Община" {
		t.Fatalf("создание %+v", created)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/communities", nil)
	if code != http.StatusOK {
		t.Fatalf("список %d %s", code, raw)
	}
	var communities []dimensionView
	mustJSON(t, raw, &communities)
	if len(communities) != 1 || communities[0].ID != created.ID {
		t.Fatalf("общины %+v", communities)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/dimensions", nil)
	if code != http.StatusOK {
		t.Fatalf("пространства %d %s", code, raw)
	}
	var dims []dimensionView
	mustJSON(t, raw, &dims)
	var personal, community int
	for _, dim := range dims {
		if dim.Kind == "personal" {
			personal++
		}
		if dim.ID == created.ID {
			community++
		}
	}
	if personal != 1 || community != 1 {
		t.Fatalf("пространства %+v", dims)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/communities/"+created.ID+"/invites", map[string]any{"ttl_seconds": 3600})
	if code != http.StatusOK {
		t.Fatalf("приглашение %d %s", code, raw)
	}
	var invite struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &invite)
	code, raw = api(t, srv, guest.Token, http.MethodPost, "/api/invites/"+invite.Token+"/accept", nil)
	if code != http.StatusOK {
		t.Fatalf("вступление %d %s", code, raw)
	}
	code, raw = api(t, srv, guest.Token, http.MethodGet, "/api/communities", nil)
	if code != http.StatusOK {
		t.Fatalf("список гостя %d %s", code, raw)
	}
	mustJSON(t, raw, &communities)
	if len(communities) != 1 || communities[0].ID != created.ID {
		t.Fatalf("гость не в общине %+v", communities)
	}
}
