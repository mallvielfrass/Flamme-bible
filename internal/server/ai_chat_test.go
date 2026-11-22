package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func forwardAI(fake *httptest.Server) roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		u := *r.URL
		u.Scheme = "http"
		u.Host = strings.TrimPrefix(fake.URL, "http://")
		req, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), r.Body)
		if err != nil {
			return nil, err
		}
		req.Header = r.Header.Clone()
		return http.DefaultTransport.RoundTrip(req)
	}
}

func openChatServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	fake := httptest.NewServer(h)
	t.Cleanup(fake.Close)
	return newAIServer(t, forwardAI(fake))
}

type chatSeed struct {
	owner   authResp
	other   authResp
	admin   authResp
	modelID string
	titleID string
	chatID  string
}

func seedChat(t *testing.T, srv *httptest.Server, withTitle bool) chatSeed {
	t.Helper()
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/ai/providers", map[string]any{
		"name": "Рабочий", "base_url": "https://example.com/v1", "token": "sk-test-token-1234",
	})
	if code != http.StatusOK {
		t.Fatalf("провайдер %d %s", code, raw)
	}
	var created aiProviderView
	mustJSON(t, raw, &created)
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/ai/providers/"+created.ID+"/models", []map[string]any{
		{"model": "chat-a", "display_name": "Чат", "can_tools": false, "can_vision": false},
		{"model": "title-a", "display_name": "Название", "can_tools": false, "can_vision": false},
	})
	if code != http.StatusOK {
		t.Fatalf("модели %d %s", code, raw)
	}
	var saved struct {
		Models []aiModelView `json:"models"`
	}
	mustJSON(t, raw, &saved)
	var modelID, titleID string
	for _, model := range saved.Models {
		if model.Model == "chat-a" {
			modelID = model.ID
		}
		if model.Model == "title-a" {
			titleID = model.ID
		}
	}
	if modelID == "" || titleID == "" {
		t.Fatalf("нет моделей %+v", saved.Models)
	}
	tasks := map[string]any{"chat": modelID, "vision": "", "parse_song": "", "title": ""}
	if withTitle {
		tasks["title"] = titleID
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/ai/tasks", tasks)
	if code != http.StatusOK {
		t.Fatalf("задачи %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/chats", map[string]any{"model_id": modelID})
	if code != http.StatusOK {
		t.Fatalf("чат %d %s", code, raw)
	}
	var chat aiChatView
	mustJSON(t, raw, &chat)
	return chatSeed{owner: owner, other: other, admin: admin, modelID: modelID, titleID: titleID, chatID: chat.ID}
}

func postStream(t *testing.T, srv *httptest.Server, token, path string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

type sseEvent struct {
	Event string
	Data  map[string]any
}

func readSSE(t *testing.T, body io.Reader) []sseEvent {
	t.Helper()
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var event string
	var data []string
	var out []sseEvent
	flush := func() {
		if len(data) == 0 {
			event = ""
			return
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &payload); err != nil {
			t.Fatalf("событие %s: %v", strings.Join(data, "\n"), err)
		}
		out = append(out, sseEvent{Event: event, Data: payload})
		event = ""
		data = nil
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	flush()
	return out
}

func eventText(events []sseEvent, name, key string) string {
	for _, ev := range events {
		if ev.Event == name {
			if v, ok := ev.Data[key].(string); ok {
				return v
			}
		}
	}
	return ""
}

func replyJSON(text string) string {
	raw, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]string{"content": text}}},
		"usage":   map[string]int{"prompt_tokens": 5, "completion_tokens": 3},
	})
	return string(raw)
}

func decodeReq(t *testing.T, r *http.Request) (maxTokens int) {
	t.Helper()
	var req struct {
		MaxTokens int `json:"max_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("тело провайдера: %v", err)
	}
	return req.MaxTokens
}

func TestAIChatStream(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if decodeReq(t, r) == 20 {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Утренний план\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", replyJSON("Ответ про уникальноеслово"))
	})
	seed := seedChat(t, srv, true)
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{
		"text": "Подбери песни к утру",
	})
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" || res.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("поток %d %s %s", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("X-Accel-Buffering"))
	}
	events := readSSE(t, res.Body)
	if eventText(events, "message_start", "model") != "chat-a" {
		t.Fatalf("старт %+v", events)
	}
	if eventText(events, "delta", "text") != "Ответ про уникальноеслово" {
		t.Fatalf("кусок %+v", events)
	}
	if eventText(events, "title", "title") != "Утренний план" {
		t.Fatalf("название %+v", events)
	}
	if eventText(events, "done", "message_id") == "" || events[len(events)-1].Data["stopped"] != false {
		t.Fatalf("конец %+v", events)
	}
	code, raw := api(t, srv, seed.owner.Token, http.MethodGet, "/api/ai/chats/"+seed.chatID, nil)
	if code != http.StatusOK {
		t.Fatalf("чат %d %s", code, raw)
	}
	var opened struct {
		Chat     aiChatView  `json:"chat"`
		Messages []aiMsgView `json:"messages"`
	}
	mustJSON(t, raw, &opened)
	if opened.Chat.Title != "Утренний план" || len(opened.Messages) != 2 || opened.Messages[1].Content != "Ответ про уникальноеслово" || opened.Messages[1].Status != "done" {
		t.Fatalf("сохранённое %+v %+v", opened.Chat, opened.Messages)
	}
	if opened.Messages[1].PromptTokens == nil || *opened.Messages[1].PromptTokens != 5 || *opened.Messages[1].CompletionTokens != 3 {
		t.Fatalf("токены %+v", opened.Messages[1])
	}
	code, raw = api(t, srv, seed.other.Token, http.MethodGet, "/api/ai/chats/"+seed.chatID, nil)
	if code != http.StatusNotFound {
		t.Fatalf("чужой чат %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.other.Token, http.MethodPost, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "чужое"})
	if code != http.StatusNotFound {
		t.Fatalf("чужое сообщение %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.other.Token, http.MethodDelete, "/api/ai/chats/"+seed.chatID, nil)
	if code != http.StatusNotFound {
		t.Fatalf("чужое удаление %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.owner.Token, http.MethodGet, "/api/ai/chats?q=уникальноеслово", nil)
	if code != http.StatusOK || !bytes.Contains(raw, []byte(seed.chatID)) {
		t.Fatalf("поиск %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.other.Token, http.MethodGet, "/api/ai/chats?q=уникальноеслово", nil)
	if code != http.StatusOK || bytes.Contains(raw, []byte(seed.chatID)) {
		t.Fatalf("чужой поиск %d %s", code, raw)
	}
	res = postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{
		"text": "Новый вопрос", "from_seq": opened.Messages[0].Seq,
	})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("правка %d %s", res.StatusCode, body)
	}
	_ = readSSE(t, res.Body)
	code, raw = api(t, srv, seed.owner.Token, http.MethodGet, "/api/ai/chats/"+seed.chatID, nil)
	if code != http.StatusOK {
		t.Fatalf("после правки %d %s", code, raw)
	}
	mustJSON(t, raw, &opened)
	if len(opened.Messages) != 2 || opened.Messages[0].Content != "Новый вопрос" || opened.Messages[0].Role != "user" {
		t.Fatalf("после правки %+v", opened.Messages)
	}
	code, raw = api(t, srv, seed.admin.Token, http.MethodDelete, "/api/admin/users/"+seed.owner.UserID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление пользователя %d %s", code, raw)
	}
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM ai_chats WHERE user_id = $1`, seed.owner.UserID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("чаты остались %d", n)
	}
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM ai_messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("сообщения остались %d", n)
	}
}

func TestAIChatStop(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	entered := make(chan struct{})
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if decodeReq(t, r) == 20 {
			http.Error(w, "нет названия", http.StatusInternalServerError)
			return
		}
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", replyJSON("частично"))
		if flusher != nil {
			flusher.Flush()
		}
		close(entered)
		<-r.Context().Done()
	})
	seed := seedChat(t, srv, false)
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "Длинный вопрос"})
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 128)
	deadline := time.Now().Add(5 * time.Second)
	for !bytes.Contains(buf, []byte("частично")) {
		if time.Now().After(deadline) {
			t.Fatalf("нет куска %s", buf)
		}
		n, err := res.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			t.Fatalf("чтение %v %s", err, buf)
		}
	}
	code, raw := api(t, srv, seed.owner.Token, http.MethodPost, "/api/ai/chats/"+seed.chatID+"/stop", nil)
	if code != http.StatusOK {
		t.Fatalf("стоп %d %s", code, raw)
	}
	rest, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(append(buf, rest...), []byte(`"stopped":true`)) {
		t.Fatalf("конец %s", append(buf, rest...))
	}
	code, raw = api(t, srv, seed.owner.Token, http.MethodGet, "/api/ai/chats/"+seed.chatID, nil)
	if code != http.StatusOK {
		t.Fatalf("чат %d %s", code, raw)
	}
	var opened struct {
		Messages []aiMsgView `json:"messages"`
	}
	mustJSON(t, raw, &opened)
	if len(opened.Messages) != 2 || opened.Messages[1].Status != "stopped" || !strings.Contains(opened.Messages[1].Content, "частично") {
		t.Fatalf("частичный %+v", opened.Messages)
	}
}

func TestAIChatBusy(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if decodeReq(t, r) == 20 {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Имя\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", replyJSON("ждём"))
		if flusher != nil {
			flusher.Flush()
		}
		entered <- struct{}{}
		select {
		case <-release:
			fmt.Fprintf(w, "data: [DONE]\n\n")
		case <-r.Context().Done():
		}
	})
	seed := seedChat(t, srv, false)
	var done []chan struct{}
	defer func() {
		close(release)
		for _, ch := range done {
			<-ch
		}
	}()
	read := func(res *http.Response) {
		ch := make(chan struct{})
		done = append(done, ch)
		go func() {
			io.Copy(io.Discard, res.Body)
			close(ch)
		}()
	}
	resA := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "первый"})
	read(resA)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("первый не дошёл до модели")
	}
	code, raw := api(t, srv, seed.owner.Token, http.MethodPost, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "второй"})
	if code != http.StatusConflict || !bytes.Contains(raw, []byte("Ответ ещё пишется")) {
		t.Fatalf("тот же чат %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.owner.Token, http.MethodPost, "/api/ai/chats", map[string]any{"model_id": seed.modelID})
	if code != http.StatusOK {
		t.Fatalf("второй чат %d %s", code, raw)
	}
	var second aiChatView
	mustJSON(t, raw, &second)
	resB := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+second.ID+"/messages", map[string]any{"text": "параллельный"})
	read(resB)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("второй не дошёл до модели")
	}
	code, raw = api(t, srv, seed.owner.Token, http.MethodPost, "/api/ai/chats", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("третий чат %d %s", code, raw)
	}
	var third aiChatView
	mustJSON(t, raw, &third)
	code, raw = api(t, srv, seed.owner.Token, http.MethodPost, "/api/ai/chats/"+third.ID+"/messages", map[string]any{"text": "лишний"})
	if code != http.StatusConflict || !bytes.Contains(raw, []byte("Ответ ещё пишется")) {
		t.Fatalf("два ответа %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.admin.Token, http.MethodGet, "/api/admin/ai", nil)
	if code != http.StatusOK || !bytes.Contains(raw, []byte(`"active_answers":2`)) {
		t.Fatalf("активные %d %s", code, raw)
	}
}

func TestAIChatLimits(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if decodeReq(t, r) == 20 {
			http.Error(w, "нет", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", replyJSON("готово"))
	})
	seed := seedChat(t, srv, false)
	code, raw := api(t, srv, seed.admin.Token, http.MethodPut, "/api/admin/ai", map[string]any{
		"enabled": true, "msg_per_10min": 1, "msg_per_day": 0, "max_tool_calls": 8,
	})
	if code != http.StatusOK {
		t.Fatalf("лимит %d %s", code, raw)
	}
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "раз"})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("первое %d %s", res.StatusCode, body)
	}
	_ = readSSE(t, res.Body)
	code, raw = api(t, srv, seed.owner.Token, http.MethodPost, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "два"})
	if code != http.StatusTooManyRequests || !bytes.Contains(raw, []byte("Слишком много сообщений")) {
		t.Fatalf("второе %d %s", code, raw)
	}
}

func TestUserMessageWindow(t *testing.T) {
	lim := newUserLimiter()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	if !lim.allow("u", 0, 0, now) || !lim.allow("u", 0, 0, now) {
		t.Fatal("ноль должен пропускать")
	}
	if !lim.allow("u", 1, 0, now) || lim.allow("u", 1, 0, now) {
		t.Fatal("предел 10 минут")
	}
	if !lim.allow("u", 1, 0, now.Add(11*time.Minute)) {
		t.Fatal("окно 10 минут не сдвинулось")
	}
	day := newUserLimiter()
	if !day.allow("u", 0, 2, now) || !day.allow("u", 0, 2, now.Add(time.Hour)) || day.allow("u", 0, 2, now.Add(2*time.Hour)) {
		t.Fatal("предел суток")
	}
	if !day.allow("u", 0, 2, now.Add(25*time.Hour)) {
		t.Fatal("сутки не сдвинулись")
	}
}

func TestAIChatTitleFallback(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if decodeReq(t, r) == 20 {
			http.Error(w, "сломанное название", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", replyJSON("Ответ"))
	})
	seed := seedChat(t, srv, true)
	question := "Вопрос про длинное название которое точно длиннее сорока символов"
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": question})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("ответ %d %s", res.StatusCode, body)
	}
	events := readSSE(t, res.Body)
	want := string([]rune(question)[:40])
	if eventText(events, "title", "title") != want {
		t.Fatalf("название %q хотели %q", eventText(events, "title", "title"), want)
	}
	var title string
	if err := testPool.QueryRow(context.Background(), `SELECT title FROM ai_chats WHERE id = $1`, seed.chatID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != want {
		t.Fatalf("в базе %q", title)
	}
}

func TestAIChatNoModel(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("модель не выбрана, а провайдер вызван")
		http.Error(w, "нет", http.StatusInternalServerError)
	})
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/ai/chats", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("чат %d %s", code, raw)
	}
	var chat aiChatView
	mustJSON(t, raw, &chat)
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/ai/chats/"+chat.ID+"/messages", map[string]any{"text": "привет"})
	if code != http.StatusBadRequest || !bytes.Contains(raw, []byte("Не выбрана модель")) {
		t.Fatalf("без модели %d %s", code, raw)
	}
}

func TestAIChatDisabled(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "нет", http.StatusInternalServerError)
	})
	seed := seedChat(t, srv, false)
	code, raw := api(t, srv, seed.admin.Token, http.MethodPut, "/api/admin/ai", map[string]any{
		"enabled": false, "msg_per_10min": 0, "msg_per_day": 0, "max_tool_calls": 8,
	})
	if code != http.StatusOK {
		t.Fatalf("выключение %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.owner.Token, http.MethodGet, "/api/ai/chats", nil)
	if code != http.StatusServiceUnavailable || !bytes.Contains(raw, []byte("Чат ИИ выключен")) {
		t.Fatalf("чаты %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.owner.Token, http.MethodGet, "/api/ai/providers", nil)
	if code != http.StatusOK {
		t.Fatalf("провайдеры при выключенном чате %d %s", code, raw)
	}
}

func TestAIAdminSettings(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {})
	seed := seedChat(t, srv, false)
	code, raw := api(t, srv, seed.owner.Token, http.MethodGet, "/api/admin/ai", nil)
	if code != http.StatusForbidden {
		t.Fatalf("не админ %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.admin.Token, http.MethodGet, "/api/admin/ai", nil)
	if code != http.StatusOK {
		t.Fatalf("настройки %d %s", code, raw)
	}
	var view aiAdminView
	mustJSON(t, raw, &view)
	if !view.Settings.Enabled || view.Settings.MsgPer10Min != 0 || view.Settings.MsgPerDay != 0 || view.Settings.MaxToolCalls != 8 {
		t.Fatalf("значения по умолчанию %+v", view.Settings)
	}
	var stored string
	if err := testPool.QueryRow(context.Background(), `SELECT value FROM app_settings WHERE key = 'ai_max_tool_calls'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "8" {
		t.Fatalf("сид %q", stored)
	}
	code, raw = api(t, srv, seed.admin.Token, http.MethodPut, "/api/admin/ai", map[string]any{
		"enabled": true, "msg_per_10min": 0, "msg_per_day": 0, "max_tool_calls": 21,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("предел инструментов %d %s", code, raw)
	}
	code, raw = api(t, srv, seed.admin.Token, http.MethodPut, "/api/admin/ai", map[string]any{
		"enabled": false, "msg_per_10min": 4, "msg_per_day": 9, "max_tool_calls": 12,
	})
	if code != http.StatusOK {
		t.Fatalf("запись %d %s", code, raw)
	}
	mustJSON(t, raw, &view)
	if view.Settings.Enabled || view.Settings.MsgPer10Min != 4 || view.Settings.MsgPerDay != 9 || view.Settings.MaxToolCalls != 12 {
		t.Fatalf("после записи %+v", view.Settings)
	}
	var action string
	if err := testPool.QueryRow(context.Background(), `SELECT action FROM admin_audit WHERE action = 'ai_settings'`).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != "ai_settings" {
		t.Fatalf("журнал %s", action)
	}
}

func TestAIChatProviderError(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	const token = "sk-test-token-1234"
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if decodeReq(t, r) == 20 {
			http.Error(w, "нет", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", replyJSON("кусок"))
		fmt.Fprintf(w, "data: {\"error\":{\"message\":\"слом Bearer %s\"}}\n\n", token)
	})
	seed := seedChat(t, srv, false)
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "вопрос"})
	events := readSSE(t, res.Body)
	errText := eventText(events, "error", "error")
	if res.StatusCode != http.StatusOK || errText == "" || strings.Contains(errText, token) {
		t.Fatalf("ошибка %d %q", res.StatusCode, errText)
	}
	var status, content string
	if err := testPool.QueryRow(context.Background(), `SELECT status, content FROM ai_messages WHERE role = 'assistant'`).Scan(&status, &content); err != nil {
		t.Fatal(err)
	}
	if status != "error" || content != "кусок" {
		t.Fatalf("сохранено %s %q", status, content)
	}
}
