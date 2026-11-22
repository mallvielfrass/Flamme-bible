// Пакет provider — тонкий клиент OpenAI-совместимого сервера.
// Здесь нет знаний FlammeBible: только запрос списка моделей.
package provider

import (
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

// Client ходит в {BaseURL}/models. HTTP должен быть клиентом из netguard, не клиентом по умолчанию.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
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
