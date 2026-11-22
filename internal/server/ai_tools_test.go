package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"flammebible/internal/ai/provider"
)

// Модель с отметкой «tools» получает список инструментов, вызывает list_plans,
// сервер выполняет его от имени пользователя и отправляет результат с tool_call_id.
func TestAIChatTools(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("тело провайдера: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		msgs, _ := body["messages"].([]any)
		last, _ := msgs[len(msgs)-1].(map[string]any)
		if _, ok := body["tools"]; ok && last["role"] == "user" {
			fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"list_plans","arguments":""}}]}}]}`+"\n\n")
			fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"scope\":\"all\"}"}}]}}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", replyJSON("Нашёл план"))
	})
	seed := seedChat(t, srv, false)
	code, raw := api(t, srv, seed.owner.Token, http.MethodPost, "/api/plans", map[string]any{"title": "Воскресенье утро"})
	if code != http.StatusOK {
		t.Fatalf("план %d %s", code, raw)
	}

	// Без отметки «tools» инструменты не передаются.
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "привет"})
	readSSE(t, res.Body)
	mu.Lock()
	if _, ok := bodies[0]["tools"]; ok {
		t.Fatal("модель без tools получила инструменты")
	}
	bodies = nil
	mu.Unlock()

	if _, err := testPool.Exec(context.Background(), `UPDATE ai_models SET can_tools = true WHERE id = $1`, seed.modelID); err != nil {
		t.Fatal(err)
	}
	res = postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "какие у меня планы?"})
	events := readSSE(t, res.Body)
	if eventText(events, "tool_start", "name") != "list_plans" || eventText(events, "tool_result", "summary") != "найдено: 1" {
		t.Fatalf("события инструмента %+v", events)
	}
	if eventText(events, "delta", "text") != "Нашёл план" {
		t.Fatalf("ответ %+v", events)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("запросов к модели %d", len(bodies))
	}
	tools, _ := bodies[0]["tools"].([]any)
	names := []string{}
	for _, item := range tools {
		fn := item.(map[string]any)["function"].(map[string]any)
		names = append(names, fn["name"].(string))
		if _, ok := fn["parameters"].(map[string]any); !ok {
			t.Fatalf("нет схемы у %v", fn["name"])
		}
	}
	if strings.Join(names, ",") != "search_songs,get_song,list_songbooks,search_bible,list_plans,get_plan" || bodies[0]["tool_choice"] != "auto" {
		t.Fatalf("инструменты %v, tool_choice %v", names, bodies[0]["tool_choice"])
	}
	msgs := bodies[1]["messages"].([]any)
	tool := msgs[len(msgs)-1].(map[string]any)
	asst := msgs[len(msgs)-2].(map[string]any)
	if asst["role"] != "assistant" || asst["tool_calls"] == nil || tool["role"] != "tool" || tool["tool_call_id"] != "call_1" ||
		!strings.Contains(tool["content"].(string), "Воскресенье утро") {
		t.Fatalf("ответ инструмента %v", msgs)
	}
}

// Чужой план модель не прочитает: ответ «не найдено», как в обычном API.
func TestAIToolGetPlanAccess(t *testing.T) {
	srv := openChatServer(t, func(w http.ResponseWriter, r *http.Request) {})
	seed := seedChat(t, srv, false)
	code, raw := api(t, srv, seed.owner.Token, http.MethodPost, "/api/plans", map[string]any{"title": "Личный"})
	if code != http.StatusOK {
		t.Fatalf("план %d %s", code, raw)
	}
	var plan planView
	mustJSON(t, raw, &plan)
	s := &Server{db: testPool}
	ctx := context.Background()
	args, _ := json.Marshal(map[string]string{"plan_id": plan.ID})
	call := func(userID string) aiToolOutcome {
		return s.runAITool(ctx, userID, provider.ToolCall{ID: "c1", Type: "function", Function: provider.FunctionCall{Name: "get_plan", Arguments: string(args)}})
	}
	if out := call(seed.owner.UserID); !out.OK || !strings.Contains(out.Content, "Личный") {
		t.Fatalf("свой план %+v", out)
	}
	if out := call(seed.other.UserID); out.OK || !strings.Contains(out.Content, "не найдено") {
		t.Fatalf("чужой план %+v", out)
	}
}

// search_songs отдаёт song_ref, по которому get_song читает текст; чужой черновик не виден.
func TestAIToolSongs(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	pushSong(t, srv, owner, book.ID, "grace", "Благодать", "дивная благодать спасла меня")
	s := &Server{db: testPool}
	ctx := context.Background()
	run := func(userID, name, args string) aiToolOutcome {
		return s.runAITool(ctx, userID, provider.ToolCall{ID: "c1", Type: "function", Function: provider.FunctionCall{Name: name, Arguments: args}})
	}
	out := run(owner.UserID, "search_songs", `{"query":"благодать"}`)
	ref := book.ID + ":grace"
	if !out.OK || !strings.Contains(out.Content, ref) || !strings.Contains(out.Content, `"semantic":"off"`) {
		t.Fatalf("поиск %+v", out)
	}
	out = run(owner.UserID, "get_song", `{"song_ref":"`+ref+`"}`)
	if !out.OK || !strings.Contains(out.Content, "дивная благодать") {
		t.Fatalf("песня %+v", out)
	}
	if out = run(other.UserID, "get_song", `{"song_ref":"`+ref+`"}`); out.OK {
		t.Fatalf("чужой черновик прочитан %+v", out)
	}
	if out = run(owner.UserID, "search_songs", `{"query":""}`); out.OK || !strings.Contains(out.Content, "неверные аргументы") {
		t.Fatalf("пустой запрос %+v", out)
	}
	if out = run(owner.UserID, "no_such_tool", `{}`); out.OK {
		t.Fatalf("неизвестный инструмент %+v", out)
	}
	if out = run(owner.UserID, "list_songbooks", ``); !out.OK || !strings.Contains(out.Content, "Домашний") {
		t.Fatalf("сборники %+v", out)
	}
}
