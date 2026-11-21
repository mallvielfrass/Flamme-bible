package server

import (
	"context"
	"net/http"
	"testing"
)

func TestHideOrDeleteSongbook(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	root := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "root", "title": "Основной"})
	patch := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "patch", "title": "Патч", "parent_id": root.ID})
	pushSong(t, srv, owner, root.ID, "song-a", "Тихая", "в тишине мы ждём тебя")
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+root.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("submit %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+root.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("approve %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/songbooks/"+root.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("скрытие %d %s", code, raw)
	}
	var hidden struct {
		Hidden  bool `json:"hidden"`
		Patches int  `json:"patches"`
	}
	mustJSON(t, raw, &hidden)
	if !hidden.Hidden || hidden.Patches != 1 {
		t.Fatalf("ответ %+v", hidden)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/songbooks", nil)
	if code != http.StatusOK || string(raw) != "[]\n" {
		t.Fatalf("каталог %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=тишине", nil)
	if code != http.StatusOK {
		t.Fatalf("поиск %d %s", code, raw)
	}
	if string(raw) != "{\"songs\":[]}\n" && !containsNone(raw) {
		t.Fatalf("скрытый нашёлся %s", raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/dimensions/"+owner.DimensionID+"/songbooks", nil)
	if code != http.StatusOK || string(raw) != "[]\n" {
		t.Fatalf("синхронизация %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/submissions?status=hidden", nil)
	if code != http.StatusOK {
		t.Fatalf("скрытые %d %s", code, raw)
	}
	var queue struct {
		Total int `json:"total"`
	}
	mustJSON(t, raw, &queue)
	if queue.Total != 2 {
		t.Fatalf("в скрытых %d", queue.Total)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+root.ID+"/unhide", nil)
	if code != http.StatusOK {
		t.Fatalf("вернуть %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/catalog/songbooks", nil)
	if code != http.StatusOK || string(raw) == "[]\n" {
		t.Fatalf("после возврата %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/songbooks/"+patch.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление патча %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/songbooks/"+root.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление сборника %d %s", code, raw)
	}
	var gone struct {
		Deleted bool `json:"deleted"`
	}
	mustJSON(t, raw, &gone)
	if !gone.Deleted {
		t.Fatalf("не удалён %s", raw)
	}
	var left int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM songbooks WHERE id = $1`, root.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("сборник остался")
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules", map[string]any{"short_name": "RST", "title": "Текст"})
	if code != http.StatusOK {
		t.Fatalf("модуль %d %s", code, raw)
	}
	var module moduleView
	mustJSON(t, raw, &module)
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO module_verses (module_id, book, book_name, chapter, verse, body)
		VALUES ($1, 1, 'Бытие', 1, 1, 'В начале')
	`, module.ID); err != nil {
		t.Fatal(err)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/modules/"+module.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление модуля %d %s", code, raw)
	}
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM module_verses WHERE module_id = $1`, module.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("стихи остались")
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/communities", map[string]any{"name": "Дом"})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var community dimensionView
	mustJSON(t, raw, &community)
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/communities/"+community.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление общины %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/communities/"+community.ID, nil)
	if code != http.StatusNotFound {
		t.Fatalf("община жива %d %s", code, raw)
	}
}

func containsNone(raw []byte) bool {
	return string(raw) == "{\"songs\":[]}\n"
}
