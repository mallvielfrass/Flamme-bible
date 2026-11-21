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

func (e *HTTPEmbedder) root() string {
	u := strings.TrimRight(e.endpoint, "/")
	u = strings.TrimSuffix(u, "/embed")
	return strings.TrimRight(u, "/")
}

type embedRuntime struct {
	ModelID   string
	Dim       int
	MaxTokens int
}

// Info читает TEI GET /info. Размерность в 1.9.4 в /info нет: тогда один пробный вектор.
func (e *HTTPEmbedder) Info(ctx context.Context) (embedRuntime, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.root()+"/info", nil)
	if err != nil {
		return embedRuntime{}, err
	}
	res, err := e.client.Do(req)
	if err != nil {
		return embedRuntime{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return embedRuntime{}, err
	}
	if res.StatusCode != http.StatusOK {
		return embedRuntime{}, fmt.Errorf("эмбеддер info: %s", res.Status)
	}
	info, err := parseEmbedInfo(raw)
	if err != nil {
		return embedRuntime{}, err
	}
	if info.Dim <= 0 {
		vecs, err := e.Embed(ctx, embedQuery, []string{"."})
		if err != nil {
			return embedRuntime{}, err
		}
		if len(vecs) != 1 || len(vecs[0]) == 0 {
			return embedRuntime{}, fmt.Errorf("эмбеддер не сообщил размерность")
		}
		info.Dim = len(vecs[0])
	}
	if info.MaxTokens <= 0 {
		info.MaxTokens = embedMaxTokens
	}
	return info, nil
}

func parseEmbedInfo(raw []byte) (embedRuntime, error) {
	var body struct {
		ModelID string `json:"model_id"`
		Dim     int    `json:"embedding_dimension"`
		DimAlt  int    `json:"dimension"`
		Max     int    `json:"max_input_length"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return embedRuntime{}, err
	}
	if strings.TrimSpace(body.ModelID) == "" {
		return embedRuntime{}, fmt.Errorf("эмбеддер info без model_id")
	}
	dim := body.Dim
	if dim <= 0 {
		dim = body.DimAlt
	}
	return embedRuntime{ModelID: body.ModelID, Dim: dim, MaxTokens: body.Max}, nil
}

// Tokenize вызывает TEI POST /tokenize. Ошибка значит, что воркер возьмёт approxTokens.
func (e *HTTPEmbedder) Tokenize(ctx context.Context, texts []string) ([]int, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(map[string]any{"inputs": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.root()+"/tokenize", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("эмбеддер tokenize: %s", res.Status)
	}
	return parseTokenCounts(raw, len(texts))
}

func parseTokenCounts(raw []byte, n int) ([]int, error) {
	var ids [][]int
	if err := json.Unmarshal(raw, &ids); err == nil && len(ids) == n {
		out := make([]int, n)
		for i, row := range ids {
			out[i] = len(row)
			if out[i] < 1 {
				out[i] = 1
			}
			if out[i] > embedMaxTokens {
				out[i] = embedMaxTokens
			}
		}
		return out, nil
	}
	var objects [][]struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &objects); err == nil && len(objects) == n {
		out := make([]int, n)
		for i, row := range objects {
			out[i] = len(row)
			if out[i] < 1 {
				out[i] = 1
			}
			if out[i] > embedMaxTokens {
				out[i] = embedMaxTokens
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("не разобран ответ tokenize")
}

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
