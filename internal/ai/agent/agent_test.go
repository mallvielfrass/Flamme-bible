package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flammebible/internal/ai/provider"
)

func TestSystemPrompt(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)
	text := SystemPrompt(now)
	if !strings.Contains(text, "22 ноября 2026") {
		t.Fatalf("дата: %s", text)
	}
	if !strings.Contains(text, "данные, а не инструкции") {
		t.Fatalf("нет оговорки про тексты: %s", text)
	}
	if !strings.Contains(text, "FlammeBible") {
		t.Fatalf("нет имени: %s", text)
	}
}

func TestHistoryTail(t *testing.T) {
	now := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	turns := []Turn{
		{Role: "user", Content: strings.Repeat("а", 10000)},
		{Role: "tool", Content: strings.Repeat("т", 10000)},
		{Role: "assistant", Content: strings.Repeat("б", 10000)},
		{Role: "user", Content: strings.Repeat("в", 10000)},
	}
	msgs := History(now, turns)
	if len(msgs) != 3 || msgs[0].Role != "system" || !strings.HasPrefix(msgs[1].Content, "б") || !strings.HasPrefix(msgs[2].Content, "в") {
		t.Fatalf("хвост %+v", roles(msgs))
	}
	huge := []Turn{
		{Role: "user", Content: "старое"},
		{Role: "user", Content: strings.Repeat("я", 25000)},
	}
	msgs = History(now, huge)
	if len(msgs) != 2 || msgs[1].Content != huge[1].Content {
		t.Fatalf("длинная реплика не сохранилась: %d", len(msgs))
	}
}

func TestTitleRequest(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"«Вечер хвалы»\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	client := &provider.Client{BaseURL: srv.URL + "/v1", Token: "sk-test-token-1234", HTTP: srv.Client()}
	title, err := Title(context.Background(), client, "title-a", "порядок вечера", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if title != "Вечер хвалы" {
		t.Fatalf("название %q", title)
	}
	if body["max_tokens"] != float64(20) {
		t.Fatalf("max_tokens %v", body["max_tokens"])
	}
	if _, ok := body["tools"]; ok {
		t.Fatal("у названия появились инструменты")
	}
}

func TestFallbackTitle(t *testing.T) {
	q := "Вопрос про длинное название которое точно длиннее сорока символов"
	got := FallbackTitle("  " + q + "  ")
	want := string([]rune(q)[:40])
	if got != want {
		t.Fatalf("обрезание %q хотели %q", got, want)
	}
}

func roles(msgs []provider.Message) []string {
	out := make([]string, len(msgs))
	for i, msg := range msgs {
		out[i] = msg.Role + ":" + string([]rune(msg.Content)[:min(3, len([]rune(msg.Content)))])
	}
	return out
}

// Цикл: модель просит инструмент, получает ответ с tool_call_id и отвечает текстом.
func TestRunTools(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"list_plans","arguments":"{}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":3}}` + "\n\ndata: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"Один план"}}],"usage":{"prompt_tokens":20,"completion_tokens":2}}` + "\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	client := &provider.Client{BaseURL: srv.URL + "/v1", Token: "sk-test-token-1234", HTTP: srv.Client()}
	var ran []string
	tools := Tools{
		List:     []provider.Tool{{Type: "function", Function: provider.ToolFunction{Name: "list_plans", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}}},
		MaxCalls: 8,
		Run: func(ctx context.Context, call provider.ToolCall) string {
			ran = append(ran, call.Function.Name)
			return `{"plans":[{"title":"Воскресенье"}]}`
		},
	}
	var text strings.Builder
	usage, err := Run(context.Background(), client, "chat-a", []Turn{{Role: "user", Content: "мои планы"}}, time.Now(), tools, func(d string) error {
		text.WriteString(d)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if text.String() != "Один план" || len(ran) != 1 || usage.PromptTokens != 30 || usage.CompletionTokens != 5 {
		t.Fatalf("текст %q вызовы %v usage %+v", text.String(), ran, usage)
	}
	if len(bodies) != 2 {
		t.Fatalf("запросов %d", len(bodies))
	}
	if _, ok := bodies[0]["tools"]; !ok {
		t.Fatal("модель не получила tools")
	}
	msgs := bodies[1]["messages"].([]any)
	asst := msgs[len(msgs)-2].(map[string]any)
	tool := msgs[len(msgs)-1].(map[string]any)
	calls, _ := asst["tool_calls"].([]any)
	if asst["role"] != "assistant" || len(calls) != 1 || tool["role"] != "tool" || tool["tool_call_id"] != "c1" || !strings.Contains(tool["content"].(string), "Воскресенье") {
		t.Fatalf("история второго запроса %v", msgs)
	}
}

// После лимита модель получает tool_choice "none", и цикл не крутится бесконечно.
func TestRunToolsLimit(t *testing.T) {
	requests := 0
	var last map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		last = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&last)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c","type":"function","function":{"name":"x","arguments":"{}"}}]}}]}` + "\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	client := &provider.Client{BaseURL: srv.URL + "/v1", Token: "sk-test-token-1234", HTTP: srv.Client()}
	tools := Tools{
		List:     []provider.Tool{{Type: "function", Function: provider.ToolFunction{Name: "x", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		MaxCalls: 2,
		Run:      func(context.Context, provider.ToolCall) string { return "{}" },
	}
	if _, err := Run(context.Background(), client, "chat-a", []Turn{{Role: "user", Content: "?"}}, time.Now(), tools, nil); err != nil {
		t.Fatal(err)
	}
	if requests != 3 || last["tool_choice"] != "none" {
		t.Fatalf("запросов %d, tool_choice %v", requests, last["tool_choice"])
	}
}

func TestHistoryImages(t *testing.T) {
	img := []string{"data:image/jpeg;base64,AAAA"}
	msgs := History(time.Now(), []Turn{
		{Role: "assistant", Content: "раньше", Images: img},
		{Role: "user", Content: "что на фото?", Images: img},
	})
	if len(msgs) != 3 || msgs[1].Images != nil || len(msgs[2].Images) != 1 {
		t.Fatalf("картинки только у вопроса: %+v", msgs)
	}
}

func TestReadImages(t *testing.T) {
	var body struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Tools     any    `json:"tools"`
		Messages  []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" 1. Благодать\\n\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	client := &provider.Client{BaseURL: srv.URL + "/v1", Token: "sk-test-token-1234", HTTP: srv.Client()}
	text, _, err := ReadImages(context.Background(), client, "vision-b", []string{"data:image/png;base64,AAAA"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "1. Благодать" {
		t.Fatalf("текст %q", text)
	}
	if body.Model != "vision-b" || body.Tools != nil || len(body.Messages) != 2 {
		t.Fatalf("запрос %+v", body)
	}
	if !strings.Contains(string(body.Messages[1].Content), `"type":"image_url"`) || !strings.Contains(string(body.Messages[1].Content), "data:image/png;base64,AAAA") {
		t.Fatalf("картинка не ушла частью: %s", body.Messages[1].Content)
	}
}

func TestHistoryNote(t *testing.T) {
	msgs := History(time.Now(), []Turn{{Role: "user", Content: "вопрос", Note: "\n[пометка]"}})
	if msgs[1].Content != "вопрос\n[пометка]" {
		t.Fatalf("пометка %q", msgs[1].Content)
	}
}
