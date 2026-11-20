package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type countingEmbedder struct {
	mu    sync.Mutex
	calls int
	model string
}

func (e *countingEmbedder) Model() string {
	if e.model == "" {
		return defaultEmbedModel
	}
	return e.model
}

func (e *countingEmbedder) Embed(_ context.Context, _ string, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.calls += len(texts)
	e.mu.Unlock()
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v := make([]float32, embedDim)
		low := strings.ToLower(text)
		switch {
		case strings.Contains(low, "прощ"):
			v[0] = 1
		case strings.Contains(low, "море"):
			v[1] = 1
		case strings.Contains(low, "печал"), strings.Contains(low, "утеш"):
			v[2] = 1
		case strings.Contains(low, "царь"), strings.Contains(low, "вирсав"):
			v[3] = 1
		default:
			v[4] = 1
		}
		out[i] = v
	}
	return out, nil
}

func (e *countingEmbedder) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func newSemanticServer(t *testing.T, emb Embedder) (*httptest.Server, *Server) {
	t.Helper()
	reset(t)
	api := New(testPool)
	api.SetEmbedder(emb)
	api.indexInline = true
	srv := httptest.NewServer(api.Handler())
	srv.Client().Timeout = 20 * time.Second
	t.Cleanup(srv.Close)
	return srv, api
}

func TestSemanticDisabled(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	code, raw := api(t, srv, user.Token, http.MethodGet, "/api/songs/semantic?q=мир", nil)
	if code != http.StatusServiceUnavailable || !strings.Contains(string(raw), "смысловой поиск недоступен") {
		t.Fatalf("песни %d %s", code, raw)
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, "/api/bible/semantic?q=мир", nil)
	if code != http.StatusServiceUnavailable || !strings.Contains(string(raw), "смысловой поиск недоступен") {
		t.Fatalf("библия %d %s", code, raw)
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, "/api/songs/search?q=мир", nil)
	if code != http.StatusOK {
		t.Fatalf("поиск по словам %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/songs/semantic?q=мир", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("без сессии %d %s", code, raw)
	}
}

func TestSemanticSongs(t *testing.T) {
	emb := &countingEmbedder{}
	srv, svc := newSemanticServer(t, emb)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	hidden := createBook(t, srv, other.Token, other.DimensionID, map[string]any{"fbid": "hidden", "title": "Чужой"})
	rev := pushAt(t, srv, owner, book.ID, 0, []testSong{
		{"forgive", "После падения", "Бог даёт прощение тому, кто встал", false},
		{"sea", "Тихая вода", "лодка вышла в открытое море", false},
	})
	pushAt(t, srv, other, hidden.ID, 0, []testSong{
		{"secret", "Секретная", "этот текст не должен находиться снаружи", false},
	})

	code, raw := api(t, srv, owner.Token, http.MethodGet, "/api/songs/semantic?q="+urlQuery("песня о прощении после падения")+"&limit=1", nil)
	if code != http.StatusOK {
		t.Fatalf("ранг %d %s", code, raw)
	}
	hits := semanticHits(t, raw)
	if len(hits) != 1 || hits[0].FBID != "forgive" || hits[0].Status != "draft" || !strings.Contains(hits[0].Fragment, "прощение") {
		t.Fatalf("ближайшая %+v", hits)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/songs/semantic?q="+urlQuery("прощение"), nil)
	if code != http.StatusOK {
		t.Fatalf("чужой поиск %d %s", code, raw)
	}
	for _, hit := range semanticHits(t, raw) {
		if hit.FBID == "forgive" || hit.SongbookID == book.ID {
			t.Fatalf("черновик виден чужому %+v", hit)
		}
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/semantic?q="+urlQuery("посторонняя тема"), nil)
	if code != http.StatusOK {
		t.Fatalf("свой поиск %d %s", code, raw)
	}
	for _, hit := range semanticHits(t, raw) {
		if hit.FBID == "secret" {
			t.Fatalf("чужая песня %+v", hit)
		}
	}

	before := emb.Calls()
	stats, err := svc.ReindexAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.SongsEmbedded != 0 || emb.Calls() != before {
		t.Fatalf("повтор пересчитал песни: %+v calls %d -> %d", stats, before, emb.Calls())
	}
	if _, err := testPool.Exec(context.Background(), `UPDATE song_embeddings SET model = 'other'`); err != nil {
		t.Fatal(err)
	}
	stats, err = svc.ReindexAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.SongsEmbedded != 3 || emb.Calls() != before+3 {
		t.Fatalf("смена модели %+v calls %d", stats, emb.Calls())
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
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/songs/semantic?q="+urlQuery("прощение")+"&limit=1&songbook_id="+book.ID+"&dimension_id="+owner.DimensionID, nil)
	if code != http.StatusOK {
		t.Fatalf("публичная %d %s", code, raw)
	}
	hits = semanticHits(t, raw)
	if len(hits) != 1 || hits[0].Status != "approved" || hits[0].FBID != "forgive" {
		t.Fatalf("одобренная %+v", hits)
	}

	rev = pushAt(t, srv, owner, book.ID, rev, []testSong{{"forgive", "После падения", "новое прощение и мир", false}})
	var rows int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM song_embeddings WHERE fbid = 'forgive'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("строк эмбеддинга %d", rows)
	}
	pushAt(t, srv, owner, book.ID, rev, []testSong{{"forgive", "После падения", "удалена", true}})
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM song_embeddings WHERE fbid = 'forgive'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("удалённая песня осталась в индексе: %d", rows)
	}
}

type testSong struct {
	fbid    string
	title   string
	text    string
	deleted bool
}

func pushAt(t *testing.T, srv *httptest.Server, user authResp, bookID string, base int64, songs []testSong) int64 {
	t.Helper()
	ops := make([]any, len(songs))
	for i, song := range songs {
		ops[i] = map[string]any{
			"operation_id": uuid.NewString(),
			"song": map[string]any{
				"fbid": song.fbid, "title": song.title, "deleted": song.deleted,
				"parts": []any{map[string]any{"kind": "Куплет", "text": song.text, "align": 0}},
			},
		}
	}
	code, raw := api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+bookID+"/push", map[string]any{
		"base_revision": base,
		"operations":    ops,
	})
	if code != http.StatusOK {
		t.Fatalf("push %d %s", code, raw)
	}
	var body struct {
		Results []opResult `json:"results"`
	}
	mustJSON(t, raw, &body)
	if len(body.Results) != len(songs) {
		t.Fatalf("результаты %+v", body.Results)
	}
	var rev int64
	for _, res := range body.Results {
		if res.Status != "applied" {
			t.Fatalf("не применено %+v", body.Results)
		}
		if res.Revision > rev {
			rev = res.Revision
		}
	}
	return rev
}

func semanticHits(t *testing.T, raw []byte) []semanticSongHit {
	t.Helper()
	var body struct {
		Songs []semanticSongHit `json:"songs"`
	}
	mustJSON(t, raw, &body)
	return body.Songs
}

func urlQuery(q string) string {
	return url.QueryEscape(q)
}

func TestHTTPEmbedderPrefixes(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		vec := make([]float32, embedDim)
		vec[0] = 1
		_ = json.NewEncoder(w).Encode([][]float32{vec})
	}))
	t.Cleanup(srv.Close)
	emb := NewHTTPEmbedder(srv.URL, "intfloat/multilingual-e5-small")
	if _, err := emb.Embed(context.Background(), embedQuery, []string{"кто виноват"}); err != nil {
		t.Fatal(err)
	}
	if got := body["inputs"].([]any)[0]; got != "query: кто виноват" || body["truncate"] != true {
		t.Fatalf("запрос %#v", body)
	}
	if _, err := emb.Embed(context.Background(), embedPassage, []string{"текст песни"}); err != nil {
		t.Fatal(err)
	}
	if got := body["inputs"].([]any)[0]; got != "passage: текст песни" {
		t.Fatalf("отрывок %#v", body)
	}
	plain := NewHTTPEmbedder(srv.URL, "bge-m3")
	if _, err := plain.Embed(context.Background(), embedPassage, []string{"текст"}); err != nil {
		t.Fatal(err)
	}
	if got := body["inputs"].([]any)[0]; got != "текст" {
		t.Fatalf("без префикса %#v", body)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([][]float32{{1, 2}})
	}))
	t.Cleanup(bad.Close)
	if _, err := NewHTTPEmbedder(bad.URL, defaultEmbedModel).Embed(context.Background(), embedQuery, []string{"текст"}); err == nil {
		t.Fatal("короткий вектор принят")
	}
}
