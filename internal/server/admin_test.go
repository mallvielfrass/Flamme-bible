package server

import (
	"net/http"
	"testing"
)

func TestModerationWorkflow(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "К публикации"})
	if book.Status != "draft" || book.Public {
		t.Fatalf("новый сборник %+v", book)
	}
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/modules", map[string]any{
		"short_name": "RST", "title": "Синодальный",
	})
	if code != http.StatusOK {
		t.Fatalf("модуль %d %s", code, raw)
	}
	var module moduleView
	mustJSON(t, raw, &module)
	if module.Status != "draft" || module.OwnerUserID == nil {
		t.Fatalf("модуль %+v", module)
	}

	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/songbooks", nil)
	if code != http.StatusOK || string(raw) != "[]\n" {
		t.Fatalf("каталог сборников %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/modules", nil)
	if code != http.StatusOK || string(raw) != "[]\n" {
		t.Fatalf("каталог модулей %d %s", code, raw)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("submit сборника %d %s", code, raw)
	}
	mustJSON(t, raw, &book)
	if book.Status != "pending" || !book.PublishRequested {
		t.Fatalf("ожидание %+v", book)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules/"+module.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("submit модуля %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/approve", nil)
	if code != http.StatusForbidden {
		t.Fatalf("владелец одобряет %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/submissions", nil)
	if code != http.StatusOK {
		t.Fatalf("очередь %d %s", code, raw)
	}
	var queue struct {
		Submissions []submission `json:"submissions"`
	}
	mustJSON(t, raw, &queue)
	got := map[string]string{}
	for _, item := range queue.Submissions {
		got[item.Kind] = item.ID
	}
	if got["songbook"] != book.ID || got["module"] != module.ID || len(queue.Submissions) != 2 {
		t.Fatalf("очередь %+v", queue.Submissions)
	}

	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("approve %d %s", code, raw)
	}
	mustJSON(t, raw, &book)
	if book.Status != "approved" || !book.Public || book.ReviewedBy == nil || *book.ReviewedBy != admin.UserID {
		t.Fatalf("одобрен %+v", book)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/songbooks", nil)
	if code != http.StatusOK {
		t.Fatalf("публичный каталог %d %s", code, raw)
	}
	var books []songbookView
	mustJSON(t, raw, &books)
	if len(books) != 1 || books[0].ID != book.ID {
		t.Fatalf("в каталоге %+v", books)
	}

	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/modules/"+module.ID+"/reject", map[string]any{"note": "нет описания"})
	if code != http.StatusOK {
		t.Fatalf("reject %d %s", code, raw)
	}
	mustJSON(t, raw, &module)
	if module.Status != "rejected" || module.ModerationNote != "нет описания" {
		t.Fatalf("отказ %+v", module)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/modules", nil)
	if code != http.StatusOK || string(raw) != "[]\n" {
		t.Fatalf("отклонённый в каталоге %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/modules/"+module.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("владелец видит отказ %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules/"+module.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("повтор %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/modules/"+module.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("approve модуля %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/modules", nil)
	if code != http.StatusOK {
		t.Fatalf("каталог модулей %d %s", code, raw)
	}
	var modules []moduleView
	mustJSON(t, raw, &modules)
	if len(modules) != 1 || modules[0].ShortName != "RST" {
		t.Fatalf("модули %+v", modules)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/modules/"+module.ID+"/revoke", map[string]any{"note": "снято"})
	if code != http.StatusOK {
		t.Fatalf("revoke %d %s", code, raw)
	}
	mustJSON(t, raw, &module)
	if module.Status != "draft" || module.ModerationNote != "снято" {
		t.Fatalf("отзыв %+v", module)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/modules", nil)
	if code != http.StatusOK || string(raw) != "[]\n" {
		t.Fatalf("после отзыва %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/revoke", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("revoke сборника %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/songbooks", nil)
	if code != http.StatusOK || string(raw) != "[]\n" {
		t.Fatalf("сборник остался публичным %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/dimensions/"+owner.DimensionID+"/songbooks", nil)
	if code != http.StatusOK {
		t.Fatalf("свои сборники %d %s", code, raw)
	}
	mustJSON(t, raw, &books)
	if len(books) != 1 || books[0].Status != "draft" {
		t.Fatalf("владелец не видит черновик %+v", books)
	}
}
