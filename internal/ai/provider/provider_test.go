package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestListModels(t *testing.T) {
	token := "sk-test-token-1234"
	var auth, target string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		target = r.URL.String()
		auth = r.Header.Get("Authorization")
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("нет таймаута")
		} else {
			left := time.Until(deadline)
			if left < 8*time.Second || left > 10*time.Second {
				t.Errorf("таймаут %s", left)
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"a"},{"id":" "},{"id":"a"},{"id":"b"}]}`)),
			Request:    r,
		}, nil
	})}
	ids, err := (&Client{BaseURL: "https://example.com/v1/", Token: token, HTTP: httpClient}).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if target != "https://example.com/v1/models" {
		t.Fatalf("адрес %s", target)
	}
	if auth != "Bearer "+token {
		t.Fatalf("заголовок %q", auth)
	}
	if strings.Join(ids, ",") != "a,b" {
		t.Fatalf("модели %v", ids)
	}
	if _, err := (&Client{BaseURL: "https://example.com/v1", Token: token}).ListModels(context.Background()); err == nil {
		t.Fatal("пустой клиент принят")
	}
}

func TestListModelsErrorScrub(t *testing.T) {
	token := "sk-test-token-1234"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "нет доступа Bearer "+token+" конец", http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL + "/v1", Token: token, HTTP: srv.Client()}).ListModels(context.Background())
	if err == nil {
		t.Fatal("ждали ошибку")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("токен в ошибке: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "Bearer ***") {
		t.Fatalf("ошибка %s", err.Error())
	}
}

func TestListModelsHTMLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<!doctype html><html><title>Example Domain</title></html>")
	}))
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL + "/v1", Token: "sk-test-token-1234", HTTP: srv.Client()}).ListModels(context.Background())
	if err == nil || strings.Contains(err.Error(), "<html") || strings.Contains(err.Error(), "doctype") {
		t.Fatalf("html в ошибке: %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("нет кода: %v", err)
	}
}

func TestChatStream(t *testing.T) {
	token := "sk-test-token-1234"
	var sawStream bool
	var sawTools bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("путь %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("заголовок %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["stream"] != true {
			t.Fatal("поток не включён")
		}
		if _, ok := body["tools"]; ok {
			sawTools = true
		}
		sawStream = true
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"При\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"вет\"}}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2}}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		left := time.Until(deadline)
		if !ok || left < 100*time.Second || left > 121*time.Second {
			t.Errorf("общий таймаут %s", left)
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	var got strings.Builder
	reply, err := (&Client{BaseURL: srv.URL + "/v1", Token: token, HTTP: httpClient}).Chat(context.Background(), ChatRequest{
		Model: "chat-a",
		Messages: []Message{
			{Role: "user", Content: "привет"},
		},
	}, func(delta string) error {
		got.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	usage := reply.Usage
	if !sawStream || sawTools || got.String() != "Привет" || !usage.Reported || usage.PromptTokens != 4 || usage.CompletionTokens != 2 {
		t.Fatalf("поток %q usage %+v tools %v", got.String(), usage, sawTools)
	}
}

func TestChatErrorScrub(t *testing.T) {
	token := "sk-test-token-1234"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html>нет Bearer "+token+"</html>", http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL + "/v1", Token: token, HTTP: srv.Client()}).Chat(context.Background(), ChatRequest{
		Model:    "chat-a",
		Messages: []Message{{Role: "user", Content: "x"}},
	}, nil)
	if err == nil || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "<html") {
		t.Fatalf("ошибка %v", err)
	}
}

func TestChatFirstByte(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	start := time.Now()
	_, err := (&Client{
		BaseURL:   srv.URL + "/v1",
		Token:     "sk-test-token-1234",
		HTTP:      srv.Client(),
		firstByte: 40 * time.Millisecond,
		answer:    time.Second,
	}).Chat(context.Background(), ChatRequest{
		Model:    "chat-a",
		Messages: []Message{{Role: "user", Content: "x"}},
	}, nil)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("ждали тишину, %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("таймаут первого байта слишком долгий: %s", time.Since(start))
	}
}

func TestChatAnswerTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"а\"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	var got strings.Builder
	_, err := (&Client{
		BaseURL:   srv.URL + "/v1",
		Token:     "sk-test-token-1234",
		HTTP:      srv.Client(),
		firstByte: time.Second,
		answer:    50 * time.Millisecond,
	}).Chat(context.Background(), ChatRequest{
		Model:    "chat-a",
		Messages: []Message{{Role: "user", Content: "x"}},
	}, func(delta string) error {
		got.WriteString(delta)
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ждали предел ответа, %v текст %q", err, got.String())
	}
	if got.String() != "а" {
		t.Fatalf("кусок потерян %q", got.String())
	}
}

// Инструменты должны уйти в запрос в формате OpenAI, а вызов из потока —
// собраться из кусков по index (так шлют OpenAI, DeepSeek, Qwen и агрегаторы).
func TestChatTools(t *testing.T) {
	var body struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name       string         `json:"name"`
				Parameters map[string]any `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
		ToolChoice string `json:"tool_choice"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search_songs","arguments":""}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"благодать\"}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"list_plans","arguments":"{}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	reply, err := (&Client{BaseURL: srv.URL + "/v1", Token: "sk-test-token-1234", HTTP: srv.Client()}).Chat(context.Background(), ChatRequest{
		Model:    "chat-a",
		Messages: []Message{{Role: "user", Content: "найди песню"}},
		Tools: []Tool{{Type: "function", Function: ToolFunction{
			Name: "search_songs", Description: "поиск",
			Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) != 1 || body.Tools[0].Type != "function" || body.Tools[0].Function.Name != "search_songs" ||
		body.Tools[0].Function.Parameters["type"] != "object" || body.ToolChoice != "auto" {
		t.Fatalf("инструменты в запросе %+v", body)
	}
	if len(reply.ToolCalls) != 2 {
		t.Fatalf("вызовы %+v", reply.ToolCalls)
	}
	first, second := reply.ToolCalls[0], reply.ToolCalls[1]
	if first.ID != "call_1" || first.Function.Name != "search_songs" || first.Function.Arguments != `{"query":"благодать"}` {
		t.Fatalf("первый вызов %+v", first)
	}
	if second.ID != "call_2" || second.Function.Name != "list_plans" || second.Function.Arguments != "{}" {
		t.Fatalf("второй вызов %+v", second)
	}
}
