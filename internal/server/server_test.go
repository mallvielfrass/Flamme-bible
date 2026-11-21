package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL не задан")
		os.Exit(1)
	}
	pool, cleanup, err := OpenTestPool(context.Background(), url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func reset(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		DROP TABLE IF EXISTS song_embeddings_next, bible_passages_next, song_embeddings_prev, bible_passages_prev;
		TRUNCATE admin_audit, embedding_jobs, bible_passages, module_verses, song_embeddings,
		         plan_items, plans, modules, client_releases, app_settings,
		         song_hides, conflicts, operation_results, change_log, song_order,
		         songbook_section_songs, songbook_sections, songs, songbooks,
		         invites, session_tokens, session_families, devices, memberships, sync_counters,
		         dimensions, users
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	reset(t)
	srv := httptest.NewServer(New(testPool).Handler())
	srv.Client().Timeout = 20 * time.Second
	t.Cleanup(srv.Close)
	return srv
}

func do(srv *httptest.Server, token, method, path string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, nil, err
	}
	return res.StatusCode, raw, nil
}

func api(t *testing.T, srv *httptest.Server, token, method, path string, body any) (int, []byte) {
	t.Helper()
	code, raw, err := do(srv, token, method, path, body)
	if err != nil {
		t.Fatal(err)
	}
	return code, raw
}

func mustJSON(t *testing.T, raw []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
}

type authResp struct {
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	DimensionID string `json:"dimension_id"`
	Token       string `json:"token"`
}

func registerUser(t *testing.T, srv *httptest.Server, email, password string, extra map[string]any) authResp {
	t.Helper()
	body := map[string]any{"email": email, "password": password}
	for k, v := range extra {
		body[k] = v
	}
	code, raw := api(t, srv, "", http.MethodPost, "/register", body)
	if code != http.StatusOK {
		t.Fatalf("register %d %s", code, raw)
	}
	var out authResp
	mustJSON(t, raw, &out)
	return out
}

func headerBody() map[string]any {
	return map[string]any{
		"format_version": "0x102",
		"flags":          0,
		"charset":        "0x4b0",
		"header_short":   "SB",
		"header_title":   "Songbook",
	}
}

func createBook(t *testing.T, srv *httptest.Server, token, dim string, body map[string]any) songbookView {
	t.Helper()
	if body["header"] == nil {
		body["header"] = headerBody()
	}
	code, raw := api(t, srv, token, http.MethodPost, "/dimensions/"+dim+"/songbooks", body)
	if code != http.StatusOK {
		t.Fatalf("сборник %d %s", code, raw)
	}
	var view songbookView
	mustJSON(t, raw, &view)
	return view
}
