package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

var (
	tinyJPEG = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00фото"))
	tinyPNG  = "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"))
)

func TestCheckChatImages(t *testing.T) {
	big := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(append([]byte("\xFF\xD8\xFF\xE0"), make([]byte, maxChatImageBytes)...))
	cases := []struct {
		name   string
		images []string
		ok     bool
	}{
		{"jpeg и png", []string{tinyJPEG, tinyPNG}, true},
		{"четыре", []string{tinyJPEG, tinyJPEG, tinyJPEG, tinyJPEG}, true},
		{"пять", []string{tinyJPEG, tinyJPEG, tinyJPEG, tinyJPEG, tinyJPEG}, false},
		{"больше 2 МиБ", []string{big}, false},
		{"тип не совпадает", []string{"data:image/png;base64," + strings.TrimPrefix(tinyJPEG, "data:image/jpeg;base64,")}, false},
		{"gif", []string{"data:image/gif;base64,R0lGODlh"}, false},
		{"ссылка", []string{"https://example.com/a.jpg"}, false},
		{"не base64", []string{"data:image/jpeg;base64,@@@"}, false},
		{"пустая", []string{"data:image/jpeg;base64,"}, false},
	}
	for _, c := range cases {
		if err := checkChatImages(c.images); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestIsAIMessagePath(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/ai/chats/11111111-1111-4111-8111-111111111111/messages": true,
		"/api/ai/chats/x/stop":       false,
		"/api/ai/chats//messages":    false,
		"/api/ai/chats/a/b/messages": false,
		"/api/ai/providers":          false,
	} {
		if isAIMessagePath(path) != want {
			t.Errorf("%s: хотели %v", path, want)
		}
	}
}

// providerRequest — то, что модель получила в последнем сообщении пользователя.
type providerRequest struct {
	Model    string
	LastUser json.RawMessage
}

func captureProvider(t *testing.T, reply func(model string) string) (http.HandlerFunc, func() []providerRequest) {
	var mu sync.Mutex
	var seen []providerRequest
	h := func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("тело провайдера: %v", err)
		}
		if body.MaxTokens == 20 {
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", replyJSON("Название"))
			return
		}
		var last json.RawMessage
		for _, m := range body.Messages {
			if m.Role == "user" {
				last = m.Content
			}
		}
		mu.Lock()
		seen = append(seen, providerRequest{Model: body.Model, LastUser: last})
		mu.Unlock()
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", replyJSON(reply(body.Model)))
	}
	return h, func() []providerRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]providerRequest(nil), seen...)
	}
}

// putModels заменяет модели единственного провайдера владельца и возвращает id по имени.
func putModels(t *testing.T, srv *httptest.Server, token string, models []map[string]any) map[string]string {
	t.Helper()
	code, raw := api(t, srv, token, http.MethodGet, "/api/ai/providers", nil)
	if code != http.StatusOK {
		t.Fatalf("провайдеры %d %s", code, raw)
	}
	var state aiState
	mustJSON(t, raw, &state)
	code, raw = api(t, srv, token, http.MethodPut, "/api/ai/providers/"+state.Providers[0].ID+"/models", models)
	if code != http.StatusOK {
		t.Fatalf("модели %d %s", code, raw)
	}
	var saved struct {
		Models []aiModelView `json:"models"`
	}
	mustJSON(t, raw, &saved)
	ids := map[string]string{}
	for _, m := range saved.Models {
		ids[m.Model] = m.ID
	}
	return ids
}

func chatMessages(t *testing.T, srv *httptest.Server, token, chatID string) []aiMsgView {
	t.Helper()
	code, raw := api(t, srv, token, http.MethodGet, "/api/ai/chats/"+chatID, nil)
	if code != http.StatusOK {
		t.Fatalf("чат %d %s", code, raw)
	}
	var opened struct {
		Messages []aiMsgView `json:"messages"`
	}
	mustJSON(t, raw, &opened)
	return opened.Messages
}

// Модель с «фото» получает картинки частями image_url. В базе остаётся только их число,
// а следующий вопрос несёт пометку текстом, без картинок.
func TestAIChatImagesVisionModel(t *testing.T) {
	h, seen := captureProvider(t, func(string) string { return "Нашёл «Благодать»" })
	srv := openChatServer(t, h)
	seed := seedChat(t, srv, false)
	putModels(t, srv, seed.owner.Token, []map[string]any{
		{"model": "chat-a", "display_name": "Чат", "can_tools": false, "can_vision": true},
	})
	path := "/api/ai/chats/" + seed.chatID + "/messages"
	res := postStream(t, srv, seed.owner.Token, path, map[string]any{"text": "Что за песни?", "images": []string{tinyJPEG, tinyPNG}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("поток %d", res.StatusCode)
	}
	readSSE(t, res.Body)
	reqs := seen()
	if len(reqs) != 1 {
		t.Fatalf("запросов %d", len(reqs))
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(reqs[0].LastUser, &parts); err != nil {
		t.Fatalf("content не списком: %s", reqs[0].LastUser)
	}
	if len(parts) != 3 || parts[0].Text != "Что за песни?" || parts[1].ImageURL.URL != tinyJPEG || parts[2].ImageURL.URL != tinyPNG {
		t.Fatalf("части %s", reqs[0].LastUser)
	}
	msgs := chatMessages(t, srv, seed.owner.Token, seed.chatID)
	if len(msgs) != 2 || msgs[0].Images == nil || msgs[0].Images.Count != 2 || msgs[0].Images.Text != "" {
		t.Fatalf("сохранённое %+v", msgs)
	}

	res = postStream(t, srv, seed.owner.Token, path, map[string]any{"text": "Добавь их"})
	readSSE(t, res.Body)
	reqs = seen()
	if len(reqs) != 2 {
		t.Fatalf("запросов %d", len(reqs))
	}
	var text string
	if err := json.Unmarshal(reqs[1].LastUser, &text); err != nil || text != "Добавь их" {
		t.Fatalf("второй вопрос %s", reqs[1].LastUser)
	}
}

// Модель без «фото» и без модели задачи «Фото»: понятный отказ до записи в чат.
func TestAIChatImagesNoVision(t *testing.T) {
	h, seen := captureProvider(t, func(string) string { return "ок" })
	srv := openChatServer(t, h)
	seed := seedChat(t, srv, false)
	path := "/api/ai/chats/" + seed.chatID + "/messages"
	res := postStream(t, srv, seed.owner.Token, path, map[string]any{"text": "Что тут?", "images": []string{tinyJPEG}})
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body["error"], "не читает изображения") {
		t.Fatalf("ответ %d %v", res.StatusCode, body)
	}
	res = postStream(t, srv, seed.owner.Token, path, map[string]any{"text": "Много", "images": []string{tinyJPEG, tinyJPEG, tinyJPEG, tinyJPEG, tinyJPEG}})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("пять картинок %d", res.StatusCode)
	}
	if len(seen()) != 0 || len(chatMessages(t, srv, seed.owner.Token, seed.chatID)) != 0 {
		t.Fatal("отказ записал сообщения или сходил к модели")
	}
}

// Модель чата без «фото», но выбрана модель для задачи «Фото»: она читает текст,
// модель чата получает его строкой, текст остаётся в истории.
func TestAIChatImagesReaderModel(t *testing.T) {
	h, seen := captureProvider(t, func(model string) string {
		if model == "vision-b" {
			return "1. Благодать удивительная"
		}
		return "Нашёл"
	})
	srv := openChatServer(t, h)
	seed := seedChat(t, srv, false)
	ids := putModels(t, srv, seed.owner.Token, []map[string]any{
		{"model": "chat-a", "display_name": "Чат", "can_tools": false, "can_vision": false},
		{"model": "vision-b", "display_name": "Фото", "can_tools": false, "can_vision": true},
	})
	code, raw := api(t, srv, seed.owner.Token, http.MethodPut, "/api/ai/tasks", map[string]any{
		"chat": ids["chat-a"], "vision": ids["vision-b"], "parse_song": "", "title": "",
	})
	if code != http.StatusOK {
		t.Fatalf("задачи %d %s", code, raw)
	}
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"images": []string{tinyJPEG}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("поток %d", res.StatusCode)
	}
	events := readSSE(t, res.Body)
	if eventText(events, "tool_start", "name") != imageReadCallID || eventText(events, "delta", "text") != "Нашёл" {
		t.Fatalf("события %+v", events)
	}
	reqs := seen()
	if len(reqs) != 2 || reqs[0].Model != "vision-b" || reqs[1].Model != "chat-a" {
		t.Fatalf("запросы %+v", reqs)
	}
	if !strings.Contains(string(reqs[0].LastUser), `"image_url"`) {
		t.Fatalf("модель для фото без картинки: %s", reqs[0].LastUser)
	}
	var text string
	if err := json.Unmarshal(reqs[1].LastUser, &text); err != nil || !strings.HasPrefix(text, defaultImageAsk) || !strings.Contains(text, "Благодать удивительная") {
		t.Fatalf("модель чата получила %s", reqs[1].LastUser)
	}
	msgs := chatMessages(t, srv, seed.owner.Token, seed.chatID)
	if len(msgs) != 2 || msgs[0].Content != defaultImageAsk || msgs[0].Images == nil || msgs[0].Images.Count != 1 || msgs[0].Images.Text != "1. Благодать удивительная" {
		t.Fatalf("сохранённое %+v", msgs)
	}
}

// Вопрос с картинкой больше общего предела JSON в 1 МиБ доходит до обработчика.
func TestAIMessageBodyLimit(t *testing.T) {
	h, _ := captureProvider(t, func(string) string { return "ок" })
	srv := openChatServer(t, h)
	seed := seedChat(t, srv, false)
	photo := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(append([]byte("\xFF\xD8\xFF\xE0"), make([]byte, 3<<19)...))
	res := postStream(t, srv, seed.owner.Token, "/api/ai/chats/"+seed.chatID+"/messages", map[string]any{"text": "Что тут?", "images": []string{photo}})
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body["error"], "не читает изображения") {
		t.Fatalf("тело 1,5 МиБ не дошло до проверки модели: %d %v", res.StatusCode, body)
	}
}
