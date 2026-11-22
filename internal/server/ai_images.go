package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flammebible/internal/ai/agent"
	"flammebible/internal/ai/provider"
)

// Картинки к вопросу в чате. Клиент сам уменьшает фото (до 1600 px, JPEG),
// сервер только проверяет количество, тип и размер.
//
// Сами картинки в базе не хранятся: в ai_messages.images пишется число картинок
// и текст, если его распознавала отдельная модель для фото. В следующих
// вопросах модель видит эту пометку, а не картинки.
const (
	maxChatImages     = 4
	maxChatImageBytes = 2 << 20
	// Base64 длиннее байтов на треть; плюс обычный предел JSON на текст.
	maxAIMessageBody   = maxChatImages*(maxChatImageBytes/3+1)*4 + maxBody
	maxImageTextRunes  = 12000
	defaultImageAsk    = "Найди песни с этих изображений."
	imageReadTimeout   = 90 * time.Second
	imageReadCallID    = "read_images"
	imageReadToolLabel = "Читаю изображения"
)

var (
	errAIImage  = errors.New("Изображения: до 4 штук, JPEG, PNG или WebP, до 2 МБ каждое")
	errAIVision = errors.New("Эта модель не читает изображения. В «Настройках ИИ» отметьте у неё «фото» или выберите модель для задачи «Фото»")
)

var imageTypes = map[string]string{
	"data:image/jpeg;base64,": "image/jpeg",
	"data:image/png;base64,":  "image/png",
	"data:image/webp;base64,": "image/webp",
}

// checkChatImages пропускает только data: URL картинок, чей тип совпадает с содержимым.
func checkChatImages(images []string) error {
	if len(images) > maxChatImages {
		return errAIImage
	}
	for _, url := range images {
		mime := ""
		payload := ""
		for prefix, kind := range imageTypes {
			if rest, ok := strings.CutPrefix(url, prefix); ok {
				mime, payload = kind, rest
				break
			}
		}
		if mime == "" || len(payload) > (maxChatImageBytes/3+1)*4 {
			return errAIImage
		}
		raw, err := base64.StdEncoding.DecodeString(payload)
		if err != nil || len(raw) == 0 || len(raw) > maxChatImageBytes || http.DetectContentType(raw) != mime {
			return errAIImage
		}
	}
	return nil
}

func isAIMessagePath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/ai/chats/")
	if !ok {
		return false
	}
	id, ok := strings.CutSuffix(rest, "/messages")
	return ok && id != "" && !strings.Contains(id, "/")
}

// imageMeta — то, что остаётся от картинок в ai_messages.images.
type imageMeta struct {
	Count int    `json:"count"`
	Text  string `json:"text,omitempty"`
}

// imageNote — пометка к реплике в истории для модели.
func imageNote(meta *imageMeta) string {
	if meta == nil || meta.Count == 0 {
		return ""
	}
	note := "\n\n[Приложено изображений: " + strconv.Itoa(meta.Count) + "."
	if meta.Text == "" {
		return note + " Сами изображения в истории не сохраняются.]"
	}
	return note + " Текст с них, распознанный автоматически:]\n" + meta.Text
}

func parseImageMeta(raw []byte) *imageMeta {
	if len(raw) == 0 {
		return nil
	}
	var meta imageMeta
	if json.Unmarshal(raw, &meta) != nil || meta.Count <= 0 {
		return nil
	}
	return &meta
}

// imageReader — модель, которая прочитает картинки: сама модель чата, если у неё
// отмечено «фото», иначе модель задачи «Фото» из настроек.
func (s *Server) imageReader(ctx context.Context, userID string, chat chatModel) (chatModel, error) {
	if chat.CanVision {
		return chat, nil
	}
	tasks, err := s.loadAITasks(ctx, userID)
	if err != nil {
		return chatModel{}, err
	}
	if tasks["vision"] == "" {
		return chatModel{}, errAIVision
	}
	reader, err := s.resolveChatModel(ctx, userID, tasks["vision"], "")
	if errors.Is(err, errNotFound) {
		return chatModel{}, errAIVision
	}
	return reader, err
}

// readImages распознаёт текст отдельной моделью для фото и дописывает его
// в ai_messages.images вопроса, который стоит перед ответом assistantID.
// Интерфейс видит шаг событиями tool_start и tool_result.
func (s *Server) readImages(ctx context.Context, reader chatModel, assistantID string, images []string, send func(string, any) error) (string, provider.Usage, error) {
	_ = send("tool_start", map[string]any{"call_id": imageReadCallID, "name": imageReadCallID, "label": imageReadToolLabel, "args": map[string]int{"count": len(images)}})
	readCtx, cancel := context.WithTimeout(ctx, imageReadTimeout)
	defer cancel()
	text, usage, err := agent.ReadImages(readCtx, s.chatClient(reader.BaseURL, reader.Token), reader.Name, images)
	if err == nil && text == "" {
		err = errors.New("модель для фото ничего не ответила")
	}
	if err != nil {
		_ = send("tool_result", map[string]any{"call_id": imageReadCallID, "ok": false, "summary": "Не прочитал изображения"})
		return "", usage, err
	}
	text = clipRunes(text, maxImageTextRunes)
	raw, _ := json.Marshal(imageMeta{Count: len(images), Text: text})
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	if _, err := s.db.Exec(saveCtx, `
		UPDATE ai_messages q SET images = $2
		FROM ai_messages a
		WHERE a.id = $1 AND q.chat_id = a.chat_id AND q.seq = a.seq - 1 AND q.role = 'user'
	`, assistantID, raw); err != nil {
		return "", usage, err
	}
	_ = send("tool_result", map[string]any{"call_id": imageReadCallID, "ok": true, "summary": "Текст с изображений прочитан моделью " + reader.Name})
	return text, usage, nil
}
