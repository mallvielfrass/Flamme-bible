package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	embedDim = 312
	// CPU-сборка text-embeddings-inference не берёт пачку больше 8 текстов.
	embedBatch        = 8
	embedPassage      = "passage"
	embedQuery        = "query"
	defaultEmbedModel = "cointegrated/rubert-tiny2"
)

// Embedder считает векторы локальной модели. kind — query или passage:
// для e5 к тексту добавляется соответствующий префикс.
type Embedder interface {
	Model() string
	Embed(ctx context.Context, kind string, texts []string) ([][]float32, error)
}

type HTTPEmbedder struct {
	endpoint string
	model    string
	client   *http.Client
}

func NewHTTPEmbedder(endpoint, model string) *HTTPEmbedder {
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultEmbedModel
	}
	return &HTTPEmbedder{
		endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		model:    model,
		client:   &http.Client{Timeout: 2 * time.Minute},
	}
}

func (e *HTTPEmbedder) Model() string { return e.model }

func (e *HTTPEmbedder) Embed(ctx context.Context, kind string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	inputs := make([]string, len(texts))
	for i, text := range texts {
		inputs[i] = prefixEmbed(e.model, kind, text)
	}
	payload, err := json.Marshal(map[string]any{
		"inputs":    inputs,
		"truncate":  true,
		"normalize": true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		snippet := raw
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("эмбеддер: %s: %s", res.Status, snippet)
	}
	vecs, err := parseEmbeddings(raw)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(texts) {
		return nil, fmt.Errorf("эмбеддер вернул %d векторов на %d текстов", len(vecs), len(texts))
	}
	for _, vec := range vecs {
		if err := validateVector(vec); err != nil {
			return nil, err
		}
	}
	return vecs, nil
}

// prefixEmbed добавляет query:/passage: моделям e5. Остальные получают текст как есть.
func prefixEmbed(model, kind, text string) string {
	if !strings.Contains(strings.ToLower(model), "e5") {
		return text
	}
	if kind == embedQuery {
		return "query: " + text
	}
	return "passage: " + text
}

func parseEmbeddings(raw []byte) ([][]float32, error) {
	var rows [][]float32
	if err := json.Unmarshal(raw, &rows); err == nil {
		return rows, nil
	}
	var one []float32
	if err := json.Unmarshal(raw, &one); err == nil {
		return [][]float32{one}, nil
	}
	return nil, fmt.Errorf("не разобран ответ эмбеддера")
}

func validateVector(v []float32) error {
	if len(v) != embedDim {
		return fmt.Errorf("эмбеддинг длины %d, нужна %d", len(v), embedDim)
	}
	var sum float64
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return fmt.Errorf("эмбеддинг содержит нечисло")
		}
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return fmt.Errorf("эмбеддинг нулевой")
	}
	return nil
}

func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 8)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
