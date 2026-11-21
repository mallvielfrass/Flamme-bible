package server

import (
	"net/http"
	"testing"
)

func TestAuditLog(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	mod := registerUser(t, srv, "mod@example.com", "secret-pass", nil)
	if _, err := testPool.Exec(t.Context(), `UPDATE users SET role = 'moderator' WHERE id = $1`, mod.UserID); err != nil {
		t.Fatal(err)
	}
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Журнал"})
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("submit %d %s", code, raw)
	}
	code, raw = api(t, srv, mod.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/reject", map[string]any{"note": "дубль"})
	if code != http.StatusOK {
		t.Fatalf("reject %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/admin/audit", nil)
	if code != http.StatusForbidden {
		t.Fatalf("пользователь в журнале %d %s", code, raw)
	}
	code, raw = api(t, srv, mod.Token, http.MethodGet, "/api/admin/audit?action=reject&actor=mod@example.com", nil)
	if code != http.StatusOK {
		t.Fatalf("журнал %d %s", code, raw)
	}
	var page struct {
		Entries []auditEntry `json:"entries"`
		Total   int          `json:"total"`
	}
	mustJSON(t, raw, &page)
	if page.Total != 1 || page.Entries[0].ActorEmail != "mod@example.com" || page.Entries[0].Summary == "" {
		t.Fatalf("запись %+v", page)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/users/"+mod.UserID+"/role", map[string]any{"role": "user"})
	if code != http.StatusOK {
		t.Fatalf("роль %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/audit?action=role", nil)
	if code != http.StatusOK {
		t.Fatalf("роли %d %s", code, raw)
	}
	mustJSON(t, raw, &page)
	if page.Total != 1 || page.Entries[0].Note != "user" {
		t.Fatalf("роль в журнале %+v", page.Entries)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/songbooks/"+book.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("карточка %d %s", code, raw)
	}
	var card struct {
		Audit []auditEntry `json:"audit"`
	}
	mustJSON(t, raw, &card)
	if len(card.Audit) != 1 || card.Audit[0].Action != "reject" {
		t.Fatalf("история карточки %+v", card.Audit)
	}
}
