package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func enableAI(t *testing.T) {
	t.Helper()
	key := bytes.Repeat([]byte{3}, 32)
	t.Setenv("AI_SECRET_KEY", base64.StdEncoding.EncodeToString(key))
}

func aiResolveExample(ctx context.Context, host string) ([]netip.Addr, error) {
	if host == "example.com" {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	}
	return nil, io.EOF
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newAIServer(t *testing.T, trip roundTripFunc) *httptest.Server {
	t.Helper()
	enableAI(t)
	reset(t)
	app := New(testPool)
	app.aiResolve = aiResolveExample
	if trip != nil {
		app.aiHTTP = &http.Client{Transport: trip}
	}
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func TestAIDisabled(t *testing.T) {
	t.Setenv("AI_SECRET_KEY", "")
	srv := newTestServer(t)
	code, raw := api(t, srv, "", http.MethodGet, "/api/ai/providers", nil)
	if code != http.StatusServiceUnavailable || !bytes.Contains(raw, []byte("ИИ не настроен")) {
		t.Fatalf("без ключа %d %s", code, raw)
	}
	user := registerUser(t, srv, "ai-off@example.com", "secret-pass", nil)
	code, raw = api(t, srv, user.Token, http.MethodPost, "/api/ai/providers", map[string]any{
		"name": "Локальный", "base_url": "https://example.com/v1", "token": "sk-test-token-1234",
	})
	if code != http.StatusServiceUnavailable {
		t.Fatalf("создание без ключа %d %s", code, raw)
	}
}

func TestAIProviders(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	token := "sk-test-token-1234"
	nextToken := "sk-next-token-9876"
	var gotAuth string
	var hits int
	srv := newAIServer(t, func(r *http.Request) (*http.Response, error) {
		hits++
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Host != "example.com" || r.URL.Path != "/v1/models" {
			t.Errorf("запрос %s", r.URL.String())
		}
		body := `{"data":[{"id":"chat-a"},{"id":"vision-b"}]}`
		if strings.HasSuffix(gotAuth, nextToken) {
			body = `{"error":"Bearer ` + nextToken + `"}`
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)

	code, raw := api(t, srv, "", http.MethodGet, "/api/ai/providers", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("без входа %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/providers", map[string]any{
		"name": "Рабочий", "base_url": "http://example.com/v1", "token": token,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("http %d %s", code, raw)
	}
	for _, rawURL := range []string{
		"https://user:pass@example.com/v1",
		"https://127.0.0.1/v1",
		"https://localhost/v1",
		"https://127.1/v1",
		"https://0x7f000001/v1",
		"https://[::ffff:127.0.0.1]/v1",
		"https://10.1.2.3/v1",
	} {
		code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/providers", map[string]any{
			"name": "Плохой", "base_url": rawURL, "token": token,
		})
		if code != http.StatusBadRequest {
			t.Fatalf("%s принят %d %s", rawURL, code, raw)
		}
	}
	if hits != 0 {
		t.Fatal("запрещённый адрес ходил в сеть")
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/providers", map[string]any{
		"name": " Рабочий ", "base_url": "https://example.com/v1/", "token": token,
	})
	if code != http.StatusOK {
		t.Fatalf("создание %d %s", code, raw)
	}
	if bytes.Contains(raw, []byte(token)) {
		t.Fatalf("токен в ответе %s", raw)
	}
	var created aiProviderView
	mustJSON(t, raw, &created)
	if created.Name != "Рабочий" || created.BaseURL != "https://example.com/v1" || created.TokenLast4 != "1234" {
		t.Fatalf("карточка %+v", created)
	}
	var enc []byte
	var nonce []byte
	if err := testPool.QueryRow(context.Background(), `SELECT token_enc, token_nonce FROM ai_providers WHERE id = $1`, created.ID).Scan(&enc, &nonce); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(enc, []byte(token)) || len(nonce) != 12 {
		t.Fatalf("хранение токена enc=%d nonce=%d", len(enc), len(nonce))
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/providers", map[string]any{
		"name": "Рабочий", "base_url": "https://example.com/v1", "token": token,
	})
	if code != http.StatusConflict {
		t.Fatalf("повтор имени %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodPost, "/api/ai/providers", map[string]any{
		"name": "Рабочий", "base_url": "https://example.com/v1", "token": token,
	})
	if code != http.StatusOK {
		t.Fatalf("то же имя у другого %d %s", code, raw)
	}
	var foreign aiProviderView
	mustJSON(t, raw, &foreign)

	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/ai/providers", nil)
	if code != http.StatusOK || bytes.Contains(raw, []byte(created.ID)) || bytes.Contains(raw, []byte(token)) {
		t.Fatalf("чужой список %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodPatch, "/api/ai/providers/"+created.ID, map[string]any{"name": "Чужой"})
	if code != http.StatusNotFound {
		t.Fatalf("чужая правка %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPatch, "/api/ai/providers/"+created.ID, map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("пустая правка %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPatch, "/api/ai/providers/"+created.ID, map[string]any{"base_url": "https://10.0.0.1/v1"})
	if code != http.StatusBadRequest {
		t.Fatalf("правка на частный адрес %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPatch, "/api/ai/providers/"+created.ID, map[string]any{"name": "Основной"})
	if code != http.StatusOK || !bytes.Contains(raw, []byte("Основной")) {
		t.Fatalf("имя %d %s", code, raw)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/providers/"+created.ID+"/test", nil)
	if code != http.StatusOK {
		t.Fatalf("проверка %d %s", code, raw)
	}
	var probe aiTestResult
	mustJSON(t, raw, &probe)
	if !probe.OK || probe.Count != 2 || probe.Error != "" || gotAuth != "Bearer "+token {
		t.Fatalf("результат %+v auth %q", probe, gotAuth)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/ai/providers/"+created.ID+"/remote-models", nil)
	if code != http.StatusOK || !bytes.Contains(raw, []byte("chat-a")) {
		t.Fatalf("список моделей %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/ai/providers/"+created.ID+"/models", []map[string]any{
		{"model": "chat-a", "display_name": "Чат", "can_tools": true, "can_vision": false},
		{"model": "vision-b", "display_name": "", "can_tools": false, "can_vision": true},
		{"model": "ручная", "display_name": "Своя", "can_tools": true, "can_vision": true},
	})
	if code != http.StatusOK {
		t.Fatalf("модели %d %s", code, raw)
	}
	var saved struct {
		Models []aiModelView `json:"models"`
	}
	mustJSON(t, raw, &saved)
	if len(saved.Models) != 3 {
		t.Fatalf("модели %+v", saved.Models)
	}
	var chatID, visionID string
	for _, model := range saved.Models {
		if model.Model == "chat-a" {
			chatID = model.ID
			if model.DisplayName != "Чат" || !model.CanTools || model.CanVision {
				t.Fatalf("чат %+v", model)
			}
		}
		if model.Model == "vision-b" {
			visionID = model.ID
			if model.DisplayName != "vision-b" || model.CanTools || !model.CanVision {
				t.Fatalf("фото %+v", model)
			}
		}
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/ai/providers/"+created.ID+"/models", []map[string]any{
		{"model": "chat-a", "display_name": "Чат 2", "can_tools": true, "can_vision": false},
	})
	if code != http.StatusOK {
		t.Fatalf("замена моделей %d %s", code, raw)
	}
	mustJSON(t, raw, &saved)
	if len(saved.Models) != 1 || saved.Models[0].ID != chatID || saved.Models[0].DisplayName != "Чат 2" {
		t.Fatalf("id модели не сохранился %+v", saved.Models)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/ai/tasks", map[string]any{
		"chat": chatID, "vision": visionID, "parse_song": "", "title": "",
	})
	if code != http.StatusNotFound {
		t.Fatalf("чужая или удалённая модель %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodPut, "/api/ai/tasks", map[string]any{
		"chat": chatID, "vision": "", "parse_song": "", "title": "",
	})
	if code != http.StatusNotFound {
		t.Fatalf("задача на чужую модель %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/ai/tasks", map[string]any{
		"chat": chatID, "vision": "", "parse_song": chatID, "title": "",
	})
	if code != http.StatusOK {
		t.Fatalf("задачи %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/ai/providers", nil)
	if code != http.StatusOK {
		t.Fatalf("список %d %s", code, raw)
	}
	var state aiState
	mustJSON(t, raw, &state)
	if len(state.Providers) != 1 || state.Tasks["chat"] != chatID || state.Tasks["parse_song"] != chatID || state.Tasks["vision"] != "" || state.Tasks["title"] != "" {
		t.Fatalf("состояние %+v", state)
	}
	if bytes.Contains(raw, []byte(token)) || bytes.Contains(raw, []byte("token_enc")) {
		t.Fatalf("секрет в списке %s", raw)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/ai/providers/"+created.ID+"/token", map[string]any{"token": nextToken})
	if code != http.StatusOK || !bytes.Contains(raw, []byte("9876")) || bytes.Contains(raw, []byte(nextToken)) {
		t.Fatalf("смена токена %d %s", code, raw)
	}
	var updated aiProviderView
	mustJSON(t, raw, &updated)
	if updated.CheckedAt != nil || updated.CheckError != "" {
		t.Fatalf("старая проверка осталась %+v", updated)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/providers/"+created.ID+"/test", nil)
	if code != http.StatusOK {
		t.Fatalf("проверка нового токена %d %s", code, raw)
	}
	mustJSON(t, raw, &probe)
	if probe.OK || strings.Contains(probe.Error, nextToken) || !strings.Contains(probe.Error, "Bearer ***") {
		t.Fatalf("ошибка проверки %+v", probe)
	}
	var stored string
	if err := testPool.QueryRow(context.Background(), `SELECT check_error FROM ai_providers WHERE id = $1`, created.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, nextToken) {
		t.Fatalf("токен в check_error %q", stored)
	}

	code, raw = api(t, srv, owner.Token, http.MethodDelete, "/api/ai/providers/"+foreign.ID, nil)
	if code != http.StatusNotFound {
		t.Fatalf("чужое удаление %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodDelete, "/api/ai/providers/"+foreign.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление %d %s", code, raw)
	}
	var left int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM ai_providers WHERE user_id = $1`, other.UserID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("провайдер остался")
	}

	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/users/"+owner.UserID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление пользователя %d %s", code, raw)
	}
	for _, table := range []string{"ai_providers", "ai_models", "ai_task_models"} {
		var n int
		if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s после удаления пользователя: %d", table, n)
		}
	}
}
