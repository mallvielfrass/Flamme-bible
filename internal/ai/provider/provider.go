// Пакет provider — тонкий клиент OpenAI-совместимого сервера.
// Здесь нет знаний FlammeBible: список моделей и потоковый чат.
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"flammebible/internal/ai/secret"
)

const listTimeout = 10 * time.Second

// Client ходит в {BaseURL}/models и {BaseURL}/chat/completions.
// HTTP должен быть клиентом из netguard, не клиентом по умолчанию.
// firstByte и answer короче боевых только в тестах этого пакета.
type Client struct {
	BaseURL   string
	Token     string
	HTTP      *http.Client
	firstByte time.Duration
	answer    time.Duration
}

// ListModels читает id моделей. Таймаут запроса — 10 секунд.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	if c == nil || c.HTTP == nil {
		return nil, errors.New("нет клиента")
	}
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	endpoint := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New(secret.Scrub(err.Error(), c.Token))
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, errors.New(secret.Scrub(err.Error(), c.Token))
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, errors.New(secret.Scrub(err.Error(), c.Token))
	}
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(secret.Scrub("провайдер "+errorText(res.Status, string(raw)), c.Token))
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errors.New("ответ провайдера")
	}
	out := make([]string, 0, len(body.Data))
	seen := map[string]struct{}{}
	for _, item := range body.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || strings.ContainsAny(id, "\r\n") || len(id) > 200 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// errorText оставляет код ответа. HTML-страницу в текст ошибки не кладём:
// иначе пользователь видит разметку чужого сайта.
func errorText(status, body string) string {
	body = strings.TrimSpace(body)
	lower := strings.ToLower(body)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") || strings.Contains(lower, "<html") {
		body = ""
	}
	return strings.TrimSpace(status + " " + body)
}

const (
	defaultFirstByte = 60 * time.Second
	defaultAnswer    = 120 * time.Second
)

// ErrSilent — за отведённое время не пришёл даже первый байт ответа.
var ErrSilent = errors.New("провайдер молчит")

// Message — одна реплика для /chat/completions.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage — счётчик из потока, если провайдер его прислал.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	Reported         bool
}

// ChatRequest — один запрос чата. MaxTokens 0 значит не передавать предел.
type ChatRequest struct {
	Model     string
	Messages  []Message
	MaxTokens int
}

// Chat читает SSE /chat/completions (stream=true) и отдаёт куски текста.
// Первый байт ждётся 60 секунд, весь ответ — 120. HTTP — клиент netguard.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onDelta func(string) error) (Usage, error) {
	if c == nil || c.HTTP == nil {
		return Usage{}, errors.New("нет клиента")
	}
	if strings.TrimSpace(req.Model) == "" || len(req.Messages) == 0 {
		return Usage{}, errors.New("запрос")
	}
	first := c.firstByte
	if first <= 0 {
		first = defaultFirstByte
	}
	answer := c.answer
	if answer <= 0 {
		answer = defaultAnswer
	}
	answerCtx, answerCancel := context.WithTimeout(ctx, answer)
	defer answerCancel()

	body := map[string]any{
		"model":          req.Model,
		"messages":       req.Messages,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Usage{}, err
	}
	endpoint := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(answerCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return Usage{}, errors.New(secret.Scrub(err.Error(), c.Token))
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	// Таймер первого байта живёт отдельно от общего предела: заголовки могут прийти сразу,
	// а первый токен — гораздо позже. Как только байт есть, таймер больше не отменяет запрос.
	firstCtx, firstCancel := context.WithCancel(answerCtx)
	timed := make(chan struct{})
	go func() {
		timer := time.NewTimer(first)
		defer timer.Stop()
		select {
		case <-firstCtx.Done():
		case <-timer.C:
			close(timed)
			answerCancel()
		}
	}()

	res, err := c.HTTP.Do(httpReq)
	firstCancel()
	if err != nil {
		if ctx.Err() != nil {
			return Usage{}, ctx.Err()
		}
		select {
		case <-timed:
			return Usage{}, ErrSilent
		default:
		}
		if errors.Is(answerCtx.Err(), context.DeadlineExceeded) {
			return Usage{}, answerCtx.Err()
		}
		return Usage{}, errors.New(secret.Scrub(err.Error(), c.Token))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return Usage{}, errors.New(secret.Scrub("провайдер "+errorText(res.Status, string(raw)), c.Token))
	}
	return readChatStream(ctx, res.Body, c.Token, onDelta)
}

func readChatStream(ctx context.Context, body io.Reader, token string, onDelta func(string) error) (Usage, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var data []string
	var usage Usage
	flush := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = data[:0]
		if payload == "[DONE]" {
			return errChatDone
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return nil
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return errors.New(secret.Scrub(chunk.Error.Message, token))
		}
		if chunk.Usage != nil {
			usage.PromptTokens = chunk.Usage.PromptTokens
			usage.CompletionTokens = chunk.Usage.CompletionTokens
			usage.Reported = true
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" || onDelta == nil {
				continue
			}
			if err := onDelta(choice.Delta.Content); err != nil {
				return err
			}
		}
		return nil
	}
	for sc.Scan() {
		if ctx.Err() != nil {
			return usage, ctx.Err()
		}
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				if errors.Is(err, errChatDone) {
					return usage, nil
				}
				return usage, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}
		data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return usage, ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return usage, err
		}
		return usage, errors.New(secret.Scrub(err.Error(), token))
	}
	if err := flush(); err != nil && !errors.Is(err, errChatDone) {
		return usage, err
	}
	return usage, nil
}

var errChatDone = errors.New("done")
