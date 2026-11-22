package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"flammebible/internal/ai/agent"
	"flammebible/internal/ai/provider"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type aiChatView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ModelID   string    `json:"model_id"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type aiMsgView struct {
	ID               string     `json:"id"`
	Seq              int        `json:"seq"`
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	Model            string     `json:"model"`
	Status           string     `json:"status"`
	PromptTokens     *int       `json:"prompt_tokens,omitempty"`
	CompletionTokens *int       `json:"completion_tokens,omitempty"`
	Cards            []aiCard   `json:"cards,omitempty"`
	Images           *imageMeta `json:"images,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type chatModel struct {
	ID        string
	Name      string
	BaseURL   string
	Token     string
	CanTools  bool
	CanVision bool
}

// Один ответ на чат и два на пользователя. Иначе второй запрос получает 409.
type chatRun struct {
	cancel  context.CancelFunc
	userID  string
	stopped atomic.Bool
}

type chatHub struct {
	mu     sync.Mutex
	byChat map[string]*chatRun
	byUser map[string]int
}

func newChatHub() *chatHub {
	return &chatHub{byChat: map[string]*chatRun{}, byUser: map[string]int{}}
}

func (h *chatHub) begin(chatID, userID string, cancel context.CancelFunc) error {
	if h == nil {
		return errAIBusy
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.byChat[chatID]; ok || h.byUser[userID] >= 2 {
		return errAIBusy
	}
	h.byChat[chatID] = &chatRun{cancel: cancel, userID: userID}
	h.byUser[userID]++
	return nil
}

func (h *chatHub) end(chatID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	run, ok := h.byChat[chatID]
	if !ok {
		return
	}
	delete(h.byChat, chatID)
	if n := h.byUser[run.userID] - 1; n <= 0 {
		delete(h.byUser, run.userID)
	} else {
		h.byUser[run.userID] = n
	}
}

func (h *chatHub) stop(chatID, userID string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	run, ok := h.byChat[chatID]
	if !ok || run.userID != userID {
		return false
	}
	run.stopped.Store(true)
	run.cancel()
	return true
}

func (h *chatHub) isStopped(chatID string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	run := h.byChat[chatID]
	return run != nil && run.stopped.Load()
}

func (h *chatHub) active() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.byChat)
}

func (s *Server) chatUser(r *http.Request) (principal, error) {
	p, err := s.aiUser(r)
	if err != nil {
		return principal{}, err
	}
	settings, err := s.loadAISettings(r.Context())
	if err != nil {
		return principal{}, err
	}
	if !settings.Enabled {
		return principal{}, errAIOff
	}
	return p, nil
}

func (s *Server) chatClient(baseURL, token string) *provider.Client {
	return &provider.Client{BaseURL: baseURL, Token: token, HTTP: s.aiHTTP}
}

func chatUUID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return errNotFound
	}
	return nil
}

func (s *Server) ownModel(ctx context.Context, userID, id string) error {
	if err := chatUUID(id); err != nil {
		return errRequest
	}
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM ai_models m
			JOIN ai_providers p ON p.id = m.provider_id
			WHERE m.id = $1 AND p.user_id = $2
		)
	`, id, userID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return errNotFound
	}
	return nil
}

func (s *Server) resolveChatModel(ctx context.Context, userID, prefer, chatModelID string) (chatModel, error) {
	id := strings.TrimSpace(prefer)
	if id == "" {
		id = strings.TrimSpace(chatModelID)
	}
	if id == "" {
		tasks, err := s.loadAITasks(ctx, userID)
		if err != nil {
			return chatModel{}, err
		}
		id = tasks["chat"]
	}
	if id == "" {
		return chatModel{}, errAIModel
	}
	if _, err := uuid.Parse(id); err != nil {
		return chatModel{}, errRequest
	}
	var name, providerID string
	var canTools, canVision bool
	err := s.db.QueryRow(ctx, `
		SELECT m.model, p.id::text, m.can_tools, m.can_vision
		FROM ai_models m
		JOIN ai_providers p ON p.id = m.provider_id
		WHERE m.id = $1 AND p.user_id = $2
	`, id, userID).Scan(&name, &providerID, &canTools, &canVision)
	if errors.Is(err, pgx.ErrNoRows) {
		return chatModel{}, errNotFound
	}
	if err != nil {
		return chatModel{}, err
	}
	base, token, err := s.aiToken(ctx, userID, providerID)
	if err != nil {
		return chatModel{}, err
	}
	return chatModel{ID: id, Name: name, BaseURL: base, Token: token, CanTools: canTools, CanVision: canVision}, nil
}

func (s *Server) titleModel(ctx context.Context, userID string, chat chatModel) chatModel {
	tasks, err := s.loadAITasks(ctx, userID)
	if err != nil || tasks["title"] == "" || tasks["title"] == chat.ID {
		return chat
	}
	titled, err := s.resolveChatModel(ctx, userID, tasks["title"], "")
	if err != nil {
		return chat
	}
	return titled
}

func (s *Server) loadAIChat(ctx context.Context, userID, id string) (aiChatView, error) {
	var chat aiChatView
	err := s.db.QueryRow(ctx, `
		SELECT id::text, title, COALESCE(model_id::text, ''), pinned, created_at, updated_at
		FROM ai_chats
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&chat.ID, &chat.Title, &chat.ModelID, &chat.Pinned, &chat.CreatedAt, &chat.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return aiChatView{}, errNotFound
	}
	if err != nil {
		return aiChatView{}, err
	}
	chat.CreatedAt = chat.CreatedAt.UTC()
	chat.UpdatedAt = chat.UpdatedAt.UTC()
	return chat, nil
}

func (s *Server) loadAIMessages(ctx context.Context, chatID string) ([]aiMsgView, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, seq, role, content, model, status, prompt_tokens, completion_tokens, cards, images, created_at
		FROM ai_messages
		WHERE chat_id = $1
		ORDER BY seq
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []aiMsgView{}
	for rows.Next() {
		var msg aiMsgView
		var cards, images []byte
		if err := rows.Scan(&msg.ID, &msg.Seq, &msg.Role, &msg.Content, &msg.Model, &msg.Status, &msg.PromptTokens, &msg.CompletionTokens, &cards, &images, &msg.CreatedAt); err != nil {
			return nil, err
		}
		msg.Images = parseImageMeta(images)
		if len(cards) > 0 && json.Unmarshal(cards, &msg.Cards) != nil {
			msg.Cards = nil
		}
		msg.CreatedAt = msg.CreatedAt.UTC()
		list = append(list, msg)
	}
	return list, rows.Err()
}

func (s *Server) listAIChats(w http.ResponseWriter, r *http.Request) {
	p, err := s.chatUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 200 {
		writeMapped(w, errRequest)
		return
	}
	args := []any{p.UserID}
	filter := ""
	if q != "" {
		args = append(args, likeContains(q))
		filter = ` AND (
			c.title ILIKE $2 OR EXISTS (
				SELECT 1 FROM ai_messages m WHERE m.chat_id = c.id AND m.content ILIKE $2
			)
		)`
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT c.id::text, c.title, COALESCE(c.model_id::text, ''), c.pinned, c.created_at, c.updated_at
		FROM ai_chats c
		WHERE c.user_id = $1`+filter+`
		ORDER BY c.pinned DESC, c.updated_at DESC
		LIMIT 50
	`, args...)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []aiChatView{}
	for rows.Next() {
		var chat aiChatView
		if err := rows.Scan(&chat.ID, &chat.Title, &chat.ModelID, &chat.Pinned, &chat.CreatedAt, &chat.UpdatedAt); err != nil {
			writeMapped(w, err)
			return
		}
		chat.CreatedAt = chat.CreatedAt.UTC()
		chat.UpdatedAt = chat.UpdatedAt.UTC()
		list = append(list, chat)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": list})
}

func (s *Server) createAIChat(w http.ResponseWriter, r *http.Request) {
	p, err := s.chatUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		ModelID string `json:"model_id"`
	}
	if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		writeMapped(w, errRequest)
		return
	}
	var modelID any
	if strings.TrimSpace(body.ModelID) != "" {
		if err := s.ownModel(r.Context(), p.UserID, strings.TrimSpace(body.ModelID)); err != nil {
			writeMapped(w, err)
			return
		}
		modelID = strings.TrimSpace(body.ModelID)
	}
	id := uuid.NewString()
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO ai_chats (id, user_id, model_id) VALUES ($1, $2, $3)
	`, id, p.UserID, modelID); err != nil {
		writeMapped(w, err)
		return
	}
	chat, err := s.loadAIChat(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chat)
}

func (s *Server) getAIChat(w http.ResponseWriter, r *http.Request) {
	p, err := s.chatUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if err := chatUUID(id); err != nil {
		writeMapped(w, err)
		return
	}
	chat, err := s.loadAIChat(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	messages, err := s.loadAIMessages(r.Context(), id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": chat, "messages": messages})
}

func (s *Server) patchAIChat(w http.ResponseWriter, r *http.Request) {
	p, err := s.chatUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if err := chatUUID(id); err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		Title   *string `json:"title"`
		Pinned  *bool   `json:"pinned"`
		ModelID *string `json:"model_id"`
	}
	if err := readJSON(r, &body); err != nil || (body.Title == nil && body.Pinned == nil && body.ModelID == nil) {
		writeMapped(w, errRequest)
		return
	}
	if _, err := s.loadAIChat(r.Context(), p.UserID, id); err != nil {
		writeMapped(w, err)
		return
	}
	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		if strings.ContainsAny(title, "\r\n") || utf8.RuneCountInString(title) > 80 {
			writeMapped(w, errRequest)
			return
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE ai_chats SET title = $3, updated_at = now() WHERE id = $1 AND user_id = $2`, id, p.UserID, title); err != nil {
			writeMapped(w, err)
			return
		}
	}
	if body.Pinned != nil {
		if _, err := s.db.Exec(r.Context(), `UPDATE ai_chats SET pinned = $3, updated_at = now() WHERE id = $1 AND user_id = $2`, id, p.UserID, *body.Pinned); err != nil {
			writeMapped(w, err)
			return
		}
	}
	if body.ModelID != nil {
		raw := strings.TrimSpace(*body.ModelID)
		var model any
		if raw != "" {
			if err := s.ownModel(r.Context(), p.UserID, raw); err != nil {
				writeMapped(w, err)
				return
			}
			model = raw
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE ai_chats SET model_id = $3, updated_at = now() WHERE id = $1 AND user_id = $2`, id, p.UserID, model); err != nil {
			writeMapped(w, err)
			return
		}
	}
	chat, err := s.loadAIChat(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chat)
}

func (s *Server) deleteAIChat(w http.ResponseWriter, r *http.Request) {
	p, err := s.chatUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if err := chatUUID(id); err != nil {
		writeMapped(w, err)
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM ai_chats WHERE id = $1 AND user_id = $2`, id, p.UserID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeMapped(w, errNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) stopAIChat(w http.ResponseWriter, r *http.Request) {
	p, err := s.chatUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if err := chatUUID(id); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := s.loadAIChat(r.Context(), p.UserID, id); err != nil {
		writeMapped(w, err)
		return
	}
	s.aiRuns.stop(id, p.UserID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) postAIMessage(w http.ResponseWriter, r *http.Request) {
	p, err := s.chatUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	chatID := r.PathValue("id")
	if err := chatUUID(chatID); err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		Text    string   `json:"text"`
		ModelID string   `json:"model_id"`
		FromSeq *int     `json:"from_seq"`
		Images  []string `json:"images"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	text := strings.TrimSpace(body.Text)
	var images *imageMeta
	if len(body.Images) > 0 {
		if err := checkChatImages(body.Images); err != nil {
			writeMapped(w, err)
			return
		}
		if text == "" {
			text = defaultImageAsk
		}
		images = &imageMeta{Count: len(body.Images)}
	}
	if body.FromSeq == nil && text == "" {
		writeMapped(w, errRequest)
		return
	}
	if text != "" && utf8.RuneCountInString(text) > 12000 {
		writeMapped(w, errRequest)
		return
	}
	ctx := r.Context()
	chat, err := s.loadAIChat(ctx, p.UserID, chatID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	model, err := s.resolveChatModel(ctx, p.UserID, body.ModelID, chat.ModelID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var reader chatModel
	if images != nil {
		if reader, err = s.imageReader(ctx, p.UserID, model); err != nil {
			writeMapped(w, err)
			return
		}
	}
	settings, err := s.loadAISettings(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeMapped(w, errRequest)
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := s.aiRuns.begin(chatID, p.UserID, cancel); err != nil {
		writeMapped(w, err)
		return
	}
	defer s.aiRuns.end(chatID)
	if !s.aiMsgs.allow(p.UserID, settings.MsgPer10Min, settings.MsgPerDay, time.Now()) {
		writeMapped(w, errAILimit)
		return
	}

	assistantID, turns, title, err := s.prepareTurn(ctx, p.UserID, chatID, text, images, model.ID, model.Name, body.FromSeq)
	if err != nil {
		writeMapped(w, err)
		return
	}
	question := lastUserText(turns)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	send := func(event string, payload any) error {
		return writeSSE(w, flusher, event, payload)
	}
	if err := send("message_start", map[string]string{"message_id": assistantID, "model": model.Name}); err != nil {
		s.finishAnswer(ctx, assistantID, "", model.Name, "stopped", provider.Usage{}, nil)
		return
	}
	var readUsage provider.Usage
	if images != nil {
		// Новый вопрос — последняя реплика. Модель с «фото» получает сами картинки,
		// иначе — текст, который с них прочитала модель задачи «Фото».
		last := &turns[len(turns)-1]
		if reader.ID == model.ID {
			last.Note, last.Images = "", body.Images
		} else {
			read, u, err := s.readImages(runCtx, reader, assistantID, body.Images, send)
			readUsage = u
			if err != nil {
				s.finishAnswer(ctx, assistantID, "", model.Name, "error", readUsage, nil)
				_ = send("error", map[string]string{"error": "Не получилось прочитать изображения: " + chatErrorText(err)})
				_ = send("done", map[string]any{"message_id": assistantID, "stopped": false})
				return
			}
			last.Note = imageNote(&imageMeta{Count: len(body.Images), Text: read})
		}
	}
	var buf strings.Builder
	var cards []aiCard
	usage, runErr := agent.Run(runCtx, s.chatClient(model.BaseURL, model.Token), model.Name, turns, time.Now(), s.chatTools(p.UserID, model, settings, send, &cards), func(delta string) error {
		buf.WriteString(delta)
		return send("delta", map[string]string{"text": delta})
	})
	usage = addReadUsage(usage, readUsage)
	status := "done"
	stopped := false
	errText := ""
	if runErr != nil {
		if s.aiRuns.isStopped(chatID) || ctx.Err() != nil || errors.Is(runErr, context.Canceled) {
			status = "stopped"
			stopped = true
		} else {
			status = "error"
			errText = chatErrorText(runErr)
		}
	}
	s.finishAnswer(ctx, assistantID, buf.String(), model.Name, status, usage, cards)
	if errText != "" {
		_ = send("error", map[string]string{"error": errText})
	}
	if strings.TrimSpace(title) == "" && question != "" {
		named := s.makeTitle(runCtx, p.UserID, question, model)
		if named != "" {
			saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			if err := s.saveChatTitle(saveCtx, chatID, named); err != nil {
				log.Printf("название чата: %v", err)
			}
			saveCancel()
			_ = send("title", map[string]string{"title": named})
		}
	}
	if usage.Reported {
		_ = send("usage", map[string]int{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
		})
	}
	_ = send("done", map[string]any{"message_id": assistantID, "stopped": stopped})
}

// chatTools — инструменты чтения и правки планов для модели с отметкой «tools». Каждый вызов
// показывается в интерфейсе событиями tool_start и tool_result.
func (s *Server) chatTools(userID string, model chatModel, settings aiSettings, send func(string, any) error, cards *[]aiCard) agent.Tools {
	if !model.CanTools {
		return agent.Tools{}
	}
	return agent.Tools{
		List:     chatToolList,
		MaxCalls: settings.MaxToolCalls,
		Run: func(ctx context.Context, call provider.ToolCall) string {
			var args any
			if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil {
				args = map[string]any{}
			}
			_ = send("tool_start", map[string]any{"call_id": call.ID, "name": call.Function.Name, "label": aiToolLabel(call), "args": args})
			out := s.runAITool(ctx, userID, call)
			payload := map[string]any{"call_id": call.ID, "ok": out.OK, "summary": out.Summary}
			if len(out.Cards) > 0 {
				payload["cards"] = out.Cards
				if cards != nil {
					*cards = appendCards(*cards, out.Cards)
				}
			}
			_ = send("tool_result", payload)
			return out.Content
		},
	}
}

func chatErrorText(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Время ответа вышло"
	case errors.Is(err, provider.ErrSilent):
		return "Провайдер не ответил"
	default:
		msg := strings.TrimSpace(err.Error())
		if msg == "" || msg == "context canceled" {
			return "Не получилось получить ответ"
		}
		return msg
	}
}

func lastUserText(turns []agent.Turn) string {
	question := ""
	for _, turn := range turns {
		if turn.Role == "user" {
			question = turn.Content
		}
	}
	return question
}

func (s *Server) makeTitle(ctx context.Context, userID, question string, chat chatModel) string {
	model := s.titleModel(ctx, userID, chat)
	titleCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	title, err := agent.Title(titleCtx, s.chatClient(model.BaseURL, model.Token), model.Name, question, time.Now())
	if err != nil || title == "" {
		return agent.FallbackTitle(question)
	}
	return title
}

func (s *Server) saveChatTitle(ctx context.Context, chatID, title string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE ai_chats SET title = $2, updated_at = now()
		WHERE id = $1 AND title = ''
	`, chatID, title)
	return err
}

func (s *Server) finishAnswer(ctx context.Context, id, content, model, status string, usage provider.Usage, cards []aiCard) {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var prompt, completion any
	if usage.Reported {
		prompt = usage.PromptTokens
		completion = usage.CompletionTokens
	}
	var raw any
	if len(cards) > 0 {
		b, err := json.Marshal(cards)
		if err != nil {
			log.Printf("карточки чата: %v", err)
		} else {
			raw = b
		}
	}
	if _, err := s.db.Exec(saveCtx, `
		UPDATE ai_messages
		SET content = $2, model = $3, status = $4, prompt_tokens = $5, completion_tokens = $6, cards = $7
		WHERE id = $1
	`, id, content, model, status, prompt, completion, raw); err != nil {
		log.Printf("ответ чата: %v", err)
	}
}

// prepareTurn пишет вопрос и пустой ответ и читает историю. images — пометка
// о картинках вопроса; в истории она дописывается к тексту реплики.
func (s *Server) prepareTurn(ctx context.Context, userID, chatID, text string, images *imageMeta, modelID, modelName string, fromSeq *int) (string, []agent.Turn, string, error) {
	if fromSeq != nil && *fromSeq < 1 {
		return "", nil, "", errRequest
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", nil, "", err
	}
	defer tx.Rollback(ctx)
	var title string
	err = tx.QueryRow(ctx, `
		SELECT title FROM ai_chats WHERE id = $1 AND user_id = $2 FOR UPDATE
	`, chatID, userID).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, "", errNotFound
	}
	if err != nil {
		return "", nil, "", err
	}
	if fromSeq != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM ai_messages WHERE chat_id = $1 AND seq >= $2`, chatID, *fromSeq); err != nil {
			return "", nil, "", err
		}
	}
	var maxSeq int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) FROM ai_messages WHERE chat_id = $1`, chatID).Scan(&maxSeq); err != nil {
		return "", nil, "", err
	}
	if text == "" {
		var role string
		err := tx.QueryRow(ctx, `SELECT role FROM ai_messages WHERE chat_id = $1 AND seq = $2`, chatID, maxSeq).Scan(&role)
		if err != nil || role != "user" {
			return "", nil, "", errRequest
		}
	} else {
		maxSeq++
		var meta any
		if images != nil {
			raw, err := json.Marshal(images)
			if err != nil {
				return "", nil, "", err
			}
			meta = raw
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_messages (id, chat_id, seq, role, content, images)
			VALUES ($1, $2, $3, 'user', $4, $5)
		`, uuid.NewString(), chatID, maxSeq, text, meta); err != nil {
			return "", nil, "", err
		}
	}
	assistantID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_messages (id, chat_id, seq, role, content, model)
		VALUES ($1, $2, $3, 'assistant', '', $4)
	`, assistantID, chatID, maxSeq+1, modelName); err != nil {
		return "", nil, "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_chats SET model_id = $2, updated_at = now() WHERE id = $1
	`, chatID, modelID); err != nil {
		return "", nil, "", err
	}
	rows, err := tx.Query(ctx, `
		SELECT role, content, images FROM ai_messages
		WHERE chat_id = $1 AND id <> $2
		ORDER BY seq
	`, chatID, assistantID)
	if err != nil {
		return "", nil, "", err
	}
	turns := []agent.Turn{}
	for rows.Next() {
		var turn agent.Turn
		var meta []byte
		if err := rows.Scan(&turn.Role, &turn.Content, &meta); err != nil {
			rows.Close()
			return "", nil, "", err
		}
		turn.Note = imageNote(parseImageMeta(meta))
		turns = append(turns, turn)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", nil, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, "", err
	}
	return assistantID, turns, title, nil
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func addReadUsage(total, read provider.Usage) provider.Usage {
	if !read.Reported {
		return total
	}
	total.PromptTokens += read.PromptTokens
	total.CompletionTokens += read.CompletionTokens
	total.Reported = true
	return total
}
