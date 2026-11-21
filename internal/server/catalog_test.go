package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestCatalogMatchStaysUntilHide(t *testing.T) {
	srv := newTestServer(t)
	a := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	b := registerUser(t, srv, "b@example.com", "secret-pass", nil)
	c := registerUser(t, srv, "c@example.com", "secret-pass", nil)
	bookA := publishBook(t, srv, a, "сборник А")
	bookB := publishBook(t, srv, b, "сборник Б")
	bookC := publishBook(t, srv, c, "сборник В")
	same := "в тишине мы ждём тебя сердца открыты для хвалы"
	near := "в тишине мы ищем тебя сердца открыты для хвалы"
	pushSong(t, srv, a, bookA.ID, "song-a", "Тихая", same)
	pushSong(t, srv, b, bookB.ID, "song-b", "Тишина", same)
	pushSong(t, srv, c, bookC.ID, "song-c", "Похожая", near)

	code, raw := api(t, srv, a.Token, http.MethodGet, "/catalog/matches?songbook="+bookA.ID+"&fbid=song-a", nil)
	if code != http.StatusOK {
		t.Fatalf("совпадения %d %s", code, raw)
	}
	var listed struct {
		Matches []catalogMatch `json:"matches"`
	}
	mustJSON(t, raw, &listed)
	got := map[string]string{}
	for _, match := range listed.Matches {
		got[match.FBID] = match.Match
	}
	if got["song-b"] != "hash" || got["song-c"] != "minhash" || len(got) != 2 {
		t.Fatalf("совпадения %+v", listed.Matches)
	}
	assertSongsRemain(t, 3)

	code, raw = api(t, srv, a.Token, http.MethodPost, "/catalog/hide", map[string]any{
		"songbook_id": bookB.ID, "fbid": "song-b",
		"against_songbook": bookA.ID, "against_fbid": "song-a",
	})
	if code != http.StatusOK {
		t.Fatalf("скрытие %d %s", code, raw)
	}
	assertSongsRemain(t, 3)
	code, raw = api(t, srv, a.Token, http.MethodGet, "/catalog/matches?songbook="+bookA.ID+"&fbid=song-a", nil)
	if code != http.StatusOK {
		t.Fatalf("после скрытия %d %s", code, raw)
	}
	mustJSON(t, raw, &listed)
	for _, match := range listed.Matches {
		if match.FBID == "song-b" {
			t.Fatal("скрытая копия снова в списке")
		}
	}
	if len(listed.Matches) != 1 || listed.Matches[0].FBID != "song-c" {
		t.Fatalf("осталось %+v", listed.Matches)
	}
	var hiddenB bool
	if err := testPool.QueryRow(context.Background(), `SELECT deleted FROM songs WHERE fbid = 'song-b'`).Scan(&hiddenB); err != nil {
		t.Fatal(err)
	}
	if hiddenB {
		t.Fatal("скрытие удалило песню")
	}
}

func TestPublishDecisionIsAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	member := registerUser(t, srv, "b@example.com", "secret-pass", nil)
	code, raw := api(t, srv, admin.Token, http.MethodPost, "/dimensions", map[string]any{"name": "Община"})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var community struct {
		ID string `json:"id"`
	}
	mustJSON(t, raw, &community)
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/dimensions/"+community.ID+"/invites", map[string]any{"ttl_seconds": 3600})
	if code != http.StatusOK {
		t.Fatalf("приглашение %d %s", code, raw)
	}
	var invite struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &invite)
	code, raw = api(t, srv, member.Token, http.MethodPost, "/invites/"+invite.Token+"/accept", nil)
	if code != http.StatusOK {
		t.Fatalf("вступление %d %s", code, raw)
	}
	book := createBook(t, srv, admin.Token, community.ID, map[string]any{"fbid": "book", "title": "общий"})
	code, raw = api(t, srv, member.Token, http.MethodPost, "/songbooks/"+book.ID+"/publish", nil)
	if code != http.StatusOK {
		t.Fatalf("запрос %d %s", code, raw)
	}
	code, raw = api(t, srv, member.Token, http.MethodPost, "/songbooks/"+book.ID+"/publish/decision", map[string]any{"public": true})
	if code != http.StatusForbidden {
		t.Fatalf("участник решает %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/songbooks/"+book.ID+"/publish/decision", map[string]any{"public": true})
	if code != http.StatusForbidden {
		t.Fatalf("админ общины обходит модерацию %d %s", code, raw)
	}
	site := ensureSiteAdmin(t, srv)
	var requested, published bool
	if err := testPool.QueryRow(context.Background(), `SELECT publish_requested, public FROM songbooks WHERE id = $1`, book.ID).Scan(&requested, &published); err != nil {
		t.Fatal(err)
	}
	if !requested || published {
		t.Fatalf("отказ участника изменил публикацию requested=%v public=%v", requested, published)
	}
	code, raw = api(t, srv, site.Token, http.MethodPost, "/songbooks/"+book.ID+"/publish/decision", map[string]any{"public": true})
	if code != http.StatusOK {
		t.Fatalf("админ сайта %d %s", code, raw)
	}
	var view songbookView
	mustJSON(t, raw, &view)
	if !view.Public || view.PublishRequested {
		t.Fatalf("решение %+v", view)
	}
}

var siteAdminByServer = map[string]authResp{}

func ensureSiteAdmin(t *testing.T, srv *httptest.Server) authResp {
	t.Helper()
	if got, ok := siteAdminByServer[srv.URL]; ok {
		return got
	}
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	code, raw := api(t, srv, "", http.MethodPost, "/register", map[string]any{
		"email": "admin@example.com", "password": "secret-pass",
	})
	if code == http.StatusConflict {
		code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{
			"email": "admin@example.com", "password": "secret-pass",
		})
	}
	if code != http.StatusOK {
		t.Fatalf("админ сайта %d %s", code, raw)
	}
	var out authResp
	mustJSON(t, raw, &out)
	siteAdminByServer[srv.URL] = out
	return out
}

func publishBook(t *testing.T, srv *httptest.Server, user authResp, title string) songbookView {
	t.Helper()
	book := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "book", "title": title})
	code, raw := api(t, srv, user.Token, http.MethodPost, "/songbooks/"+book.ID+"/publish", nil)
	if code != http.StatusOK {
		t.Fatalf("запрос %d %s", code, raw)
	}
	site := ensureSiteAdmin(t, srv)
	code, raw = api(t, srv, site.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("решение %d %s", code, raw)
	}
	var view songbookView
	mustJSON(t, raw, &view)
	if !view.Public {
		t.Fatal("сборник не публичный")
	}
	return view
}

func pushSong(t *testing.T, srv *httptest.Server, user authResp, bookID, fbid, title, text string) {
	t.Helper()
	op := map[string]any{
		"operation_id": uuid.NewString(),
		"song": map[string]any{
			"fbid":  fbid,
			"title": title,
			"parts": []any{map[string]any{"kind": "Куплет", "text": text, "align": 0}},
		},
	}
	code, raw := api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+bookID+"/push", pushBody(0, op))
	if code != http.StatusOK {
		t.Fatalf("push %d %s", code, raw)
	}
}

func assertSongsRemain(t *testing.T, n int) {
	t.Helper()
	var count, deleted int
	err := testPool.QueryRow(context.Background(), `SELECT count(*), count(*) FILTER (WHERE deleted) FROM songs`).Scan(&count, &deleted)
	if err != nil {
		t.Fatal(err)
	}
	if count != n || deleted != 0 {
		t.Fatalf("песен %d, удалённых %d", count, deleted)
	}
}
