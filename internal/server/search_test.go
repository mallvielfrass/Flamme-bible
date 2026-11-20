package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestSongSearch(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	pushTwoSongs(t, srv, owner, book.ID)
	hidden := createBook(t, srv, other.Token, other.DimensionID, map[string]any{"fbid": "hidden", "title": "Чужой"})
	pushSong(t, srv, other, hidden.ID, "secret", "Секретная", "этот текст не должен находиться снаружи")

	code, raw := api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=Альфа", nil)
	if code != http.StatusOK {
		t.Fatalf("название %d %s", code, raw)
	}
	hits := songHits(t, raw)
	if len(hits) != 1 || hits[0].FBID != "alpha" || hits[0].Status != "draft" {
		t.Fatalf("хиты %+v", hits)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=уникальнаястрока", nil)
	if code != http.StatusOK {
		t.Fatalf("текст %d %s", code, raw)
	}
	hits = songHits(t, raw)
	if len(hits) != 1 || hits[0].FBID != "beta" {
		t.Fatalf("по тексту %+v", hits)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/songs/search?q=Альфа", nil)
	if code != http.StatusOK {
		t.Fatalf("чужой поиск %d %s", code, raw)
	}
	if hits = songHits(t, raw); len(hits) != 0 {
		t.Fatalf("черновик виден чужому %+v", hits)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q="+url.QueryEscape("%"), nil)
	if code != http.StatusOK {
		t.Fatalf("процент %d %s", code, raw)
	}
	if hits = songHits(t, raw); len(hits) != 0 {
		t.Fatalf("процент совпал со всем %+v", hits)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=а&limit=1&offset=1", nil)
	if code != http.StatusOK {
		t.Fatalf("страница %d %s", code, raw)
	}
	hits = songHits(t, raw)
	if len(hits) != 1 || hits[0].FBID != "beta" {
		t.Fatalf("смещение %+v", hits)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=Альфа&songbook_id="+book.ID+"&dimension_id="+owner.DimensionID, nil)
	if code != http.StatusOK || len(songHits(t, raw)) != 1 {
		t.Fatalf("фильтр %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/songs/search?q=Альфа", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("без сессии %d %s", code, raw)
	}

	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("отправка %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("одобрение %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/songs/search?q=Альфа", nil)
	if code != http.StatusOK {
		t.Fatalf("публичный поиск %d %s", code, raw)
	}
	hits = songHits(t, raw)
	if len(hits) != 1 || hits[0].Status != "approved" {
		t.Fatalf("одобренная песня %+v", hits)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=Секретная", nil)
	if code != http.StatusOK || len(songHits(t, raw)) != 0 {
		t.Fatalf("чужой черновик %d %s", code, raw)
	}
}

func songHits(t *testing.T, raw []byte) []songHit {
	t.Helper()
	var body struct {
		Songs []songHit `json:"songs"`
	}
	mustJSON(t, raw, &body)
	return body.Songs
}

func pushTwoSongs(t *testing.T, srv *httptest.Server, user authResp, bookID string) {
	t.Helper()
	ops := []any{
		map[string]any{
			"operation_id": uuid.NewString(),
			"song": map[string]any{
				"fbid": "alpha", "title": "Альфа",
				"parts": []any{map[string]any{"kind": "Куплет", "text": "обычный куплет", "align": 0}},
			},
		},
		map[string]any{
			"operation_id": uuid.NewString(),
			"song": map[string]any{
				"fbid": "beta", "title": "Бета",
				"parts": []any{map[string]any{"kind": "Куплет", "text": "здесь уникальнаястрока лирики", "align": 0}},
			},
		},
	}
	code, raw := api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+bookID+"/push", map[string]any{
		"base_revision": 0,
		"operations":    ops,
	})
	if code != http.StatusOK {
		t.Fatalf("push %d %s", code, raw)
	}
}
