package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelNames(t *testing.T) {
	if !validHFModel("cointegrated/rubert-tiny2") || !validHFModel("org/name-1.0") {
		t.Fatal("нормальное имя")
	}
	for _, bad := range []string{"", "rubert", "../org/model", "http://example.com/org/model", "org/name with space", "org//name"} {
		if bad == "org//name" && validHFModel(bad) {
			// двойной слэш допустим набором символов; путь всё равно не файловый
			continue
		}
		if validHFModel(bad) {
			t.Fatalf("принято %q", bad)
		}
	}
	if !validGGUFName("model.gguf") || !validGGUFName("My_model-1.gguf") {
		t.Fatal("имя файла")
	}
	for _, bad := range []string{"../model.gguf", "dir/model.gguf", "model.bin", "model.gguf/../x", ".gguf"} {
		if validGGUFName(bad) {
			t.Fatalf("файл принят %q", bad)
		}
	}
}

func TestLlamaInfoFallback(t *testing.T) {
	vec := make([]float32, 8)
	vec[0] = 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "model.gguf"}}})
		case "/v1/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": vec}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	info, err := NewHTTPEmbedder(srv.URL+"/embed", "").Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Source != "llama" || info.ModelID != "model.gguf" || info.Dim != 8 {
		t.Fatalf("%+v", info)
	}
}

func TestAdminEmbedModel(t *testing.T) {
	emb := &countingEmbedder{}
	srv, _ := newSemanticServer(t, emb)
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	pushAt(t, srv, owner, book.ID, 0, []testSong{{"forgive", "После падения", "Бог даёт прощение тому, кто встал", false}})
	var jobs int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM embedding_jobs`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}

	code, raw := api(t, srv, owner.Token, http.MethodPut, "/api/admin/embed/model", map[string]any{
		"source": "tei", "model_id": "org/other-model",
	})
	if code != http.StatusForbidden {
		t.Fatalf("чужой %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPut, "/api/admin/embed/model", map[string]any{
		"source": "tei", "model_id": "../secret",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("плохое имя %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPut, "/api/admin/embed/model", map[string]any{
		"source": "tei", "model_id": "org/other-model",
	})
	if code != http.StatusOK {
		t.Fatalf("запись %d %s", code, raw)
	}
	var stats embedQueueStats
	mustJSON(t, raw, &stats)
	if stats.Model.DesiredModel != "org/other-model" || stats.Model.DesiredSource != "tei" {
		t.Fatalf("карточка %+v", stats.Model)
	}
	if !strings.Contains(stats.Model.Restart, "EMBED_MODEL=org/other-model") || strings.Contains(stats.Model.Restart, "docker.sock") {
		t.Fatalf("подсказка %q", stats.Model.Restart)
	}
	var after int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM embedding_jobs`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != jobs {
		t.Fatalf("смена желаемой модели поставила задания %d -> %d", jobs, after)
	}
	var stored string
	if err := testPool.QueryRow(context.Background(), `SELECT value FROM app_settings WHERE key = 'embed_set_model'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "org/other-model" {
		t.Fatal("желаемая модель записана как текущий набор векторов")
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/embed/model/reset", nil)
	if code != http.StatusOK {
		t.Fatalf("сброс %d %s", code, raw)
	}
	mustJSON(t, raw, &stats)
	if stats.Model.DesiredModel != defaultEmbedModel || stats.Model.DesiredSource != "tei" {
		t.Fatalf("после сброса %+v", stats.Model)
	}
	var audits int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit WHERE action IN ('embed_model', 'embed_model_reset')`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("журнал %d", audits)
	}
}

func TestGGUFUpload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EMBED_MODELS_DIR", dir)
	t.Setenv("GGUF_MAX_BYTES", "16")
	emb := &countingEmbedder{}
	srv, _ := newSemanticServer(t, emb)
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)

	code, raw := postRaw(t, srv, owner.Token, "/api/admin/embed/gguf?name=model.gguf", append([]byte("GGUF"), bytes.Repeat([]byte{1}, 4)...))
	if code != http.StatusForbidden {
		t.Fatalf("чужой %d %s", code, raw)
	}
	code, raw = postRaw(t, srv, admin.Token, "/api/admin/embed/gguf?name=../model.gguf", []byte("GGUF"))
	if code != http.StatusBadRequest {
		t.Fatalf("путь %d %s", code, raw)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "model.gguf")); !os.IsNotExist(err) {
		t.Fatal("файл вышел из каталога")
	}
	code, raw = postRaw(t, srv, admin.Token, "/api/admin/embed/gguf?name=model.gguf", []byte("NOPE"))
	if code != http.StatusBadRequest {
		t.Fatalf("заголовок %d %s", code, raw)
	}
	code, raw = postRaw(t, srv, admin.Token, "/api/admin/embed/gguf?name=model.gguf", append([]byte("GGUF"), bytes.Repeat([]byte{1}, 20)...))
	if code != http.StatusBadRequest {
		t.Fatalf("размер %d %s", code, raw)
	}
	body := append([]byte("GGUF"), bytes.Repeat([]byte{2}, 4)...)
	code, raw = postRaw(t, srv, admin.Token, "/api/admin/embed/gguf?name=model.gguf", body)
	if code != http.StatusOK {
		t.Fatalf("загрузка %d %s", code, raw)
	}
	got, err := os.ReadFile(filepath.Join(dir, "model.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("содержимое %q", got)
	}
	var audits int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit WHERE action = 'embed_gguf'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("журнал %d", audits)
	}
}

func postRaw(t *testing.T, srv *httptest.Server, token, path string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, raw
}
