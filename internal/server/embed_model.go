package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const defaultGGUFMax = 64 << 20

func validHFModel(id string) bool {
	if len(id) < 3 || len(id) > 128 || strings.Contains(id, "..") || strings.Contains(id, "://") {
		return false
	}
	if !strings.Contains(id, "/") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '/':
		default:
			return false
		}
	}
	return true
}

func validGGUFName(name string) bool {
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return false
	}
	if len(name) > 80 || !strings.HasSuffix(strings.ToLower(name), ".gguf") || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.TrimSuffix(strings.ToLower(name), ".gguf") == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func ggufMaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("GGUF_MAX_BYTES"))
	if raw == "" {
		return defaultGGUFMax
	}
	var n int64
	for _, r := range raw {
		if r < '0' || r > '9' {
			return defaultGGUFMax
		}
		n = n*10 + int64(r-'0')
	}
	if n < 1 {
		return defaultGGUFMax
	}
	return n
}

func modelsDir() (string, error) {
	dir := strings.TrimSpace(os.Getenv("EMBED_MODELS_DIR"))
	if dir == "" {
		return "", errRequest
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return real, nil
}

func (s *Server) adminSetEmbedModel(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Source  string `json:"source"`
		ModelID string `json:"model_id"`
		File    string `json:"file"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	source := strings.TrimSpace(body.Source)
	if source == "" {
		source = "tei"
	}
	modelID := strings.TrimSpace(body.ModelID)
	file := strings.TrimSpace(body.File)
	if source == "tei" {
		if !validHFModel(modelID) {
			writeMapped(w, errRequest)
			return
		}
		file = ""
	} else if source == "llama" {
		if !validGGUFName(file) {
			writeMapped(w, errRequest)
			return
		}
		if _, err := ggufPath(file); err != nil {
			writeMapped(w, errRequest)
			return
		}
		if modelID == "" {
			modelID = file
		}
	} else {
		writeMapped(w, errRequest)
		return
	}
	if err := s.writeDesiredModel(r.Context(), p.UserID, "embed_model", source, modelID, file); err != nil {
		writeMapped(w, err)
		return
	}
	stats, err := s.embedStats(r.Context())
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) adminResetEmbedModel(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := s.writeDesiredModel(r.Context(), p.UserID, "embed_model_reset", "tei", defaultEmbedModel, ""); err != nil {
		writeMapped(w, err)
		return
	}
	stats, err := s.embedStats(r.Context())
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) writeDesiredModel(ctx context.Context, actor, action, source, modelID, file string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.putSetting(ctx, tx, "embed_desired_source", source); err != nil {
		return err
	}
	if err := s.putSetting(ctx, tx, "embed_desired_model", modelID); err != nil {
		return err
	}
	if err := s.putSetting(ctx, tx, "embed_desired_file", file); err != nil {
		return err
	}
	note := source + " " + modelID
	if file != "" {
		note += " " + file
	}
	if err := writeAudit(ctx, tx, actor, action, "embed", "model", modelID, note); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) adminUploadGGUF(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if !validGGUFName(name) {
		writeMapped(w, errRequest)
		return
	}
	n, err := saveGGUF(name, r.Body)
	if err != nil {
		writeMapped(w, err)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if err := writeAudit(r.Context(), tx, p.UserID, "embed_gguf", "embed", name, name, fmt.Sprintf("%d байт", n)); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeMapped(w, err)
		return
	}
	stats, err := s.embedStats(r.Context())
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func tooLarge(err error) bool {
	if err == nil {
		return false
	}
	var tooBig *http.MaxBytesError
	return errors.As(err, &tooBig) || strings.Contains(err.Error(), "too large")
}

func ggufPath(name string) (string, error) {
	if !validGGUFName(name) {
		return "", errRequest
	}
	root, err := modelsDir()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(root, name)
	if err := pathInside(root, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func saveGGUF(name string, body io.Reader) (int64, error) {
	dest, err := ggufPath(name)
	if err != nil {
		return 0, err
	}
	root := filepath.Dir(dest)
	tmp, err := os.CreateTemp(root, ".gguf-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	var magic [4]byte
	if _, err := io.ReadFull(body, magic[:]); err != nil || string(magic[:]) != "GGUF" {
		if tooLarge(err) {
			return 0, errRequest
		}
		return 0, errRequest
	}
	if _, err := tmp.Write(magic[:]); err != nil {
		return 0, err
	}
	limit := ggufMaxBytes()
	n, err := io.Copy(tmp, io.LimitReader(body, limit-3))
	if err != nil {
		if tooLarge(err) {
			return 0, errRequest
		}
		return 0, err
	}
	total := n + 4
	if total > limit {
		return 0, errRequest
	}
	if err := tmp.Sync(); err != nil {
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return 0, err
	}
	if err := os.Chmod(dest, 0o644); err != nil {
		return 0, err
	}
	ok = true
	return total, nil
}

func listGGUF() []string {
	root, err := modelsDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !validGGUFName(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	return names
}

func ggufSize(name string) int64 {
	if name == "" {
		return 0
	}
	path, err := ggufPath(name)
	if err != nil {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}

type embedModelCard struct {
	Source               string   `json:"source"`
	ID                   string   `json:"id"`
	Dimension            int      `json:"dimension"`
	MaxTokens            int      `json:"max_tokens"`
	FileSize             int64    `json:"file_size"`
	MeasuredTokensPerSec float64  `json:"measured_tokens_per_sec"`
	DesiredSource        string   `json:"desired_source"`
	DesiredModel         string   `json:"desired_model"`
	DesiredFile          string   `json:"desired_file"`
	Restart              string   `json:"restart"`
	Files                []string `json:"files"`
}

func (s *Server) modelCard(ctx context.Context, live embedRuntime, measured float64) embedModelCard {
	source, _ := s.setting(ctx, "embed_desired_source")
	modelID, _ := s.setting(ctx, "embed_desired_model")
	file, _ := s.setting(ctx, "embed_desired_file")
	if source == "" {
		source = "tei"
	}
	if modelID == "" {
		modelID = defaultEmbedModel
	}
	liveSource := live.Source
	if liveSource == "" {
		liveSource = "tei"
	}
	sizeName := file
	if liveSource == "llama" && strings.HasSuffix(strings.ToLower(live.ModelID), ".gguf") {
		sizeName = live.ModelID
	}
	card := embedModelCard{
		Source:               liveSource,
		ID:                   live.ModelID,
		Dimension:            live.Dim,
		MaxTokens:            live.MaxTokens,
		FileSize:             ggufSize(sizeName),
		MeasuredTokensPerSec: measured,
		DesiredSource:        source,
		DesiredModel:         modelID,
		DesiredFile:          file,
		Restart:              restartHint(source, modelID, file),
		Files:                listGGUF(),
	}
	if card.Files == nil {
		card.Files = []string{}
	}
	return card
}

func restartHint(source, modelID, file string) string {
	if source == "llama" {
		return "LLAMA_GGUF=" + file + " docker compose --profile llama up -d --force-recreate llama"
	}
	return "EMBED_MODEL=" + modelID + " docker compose --profile semantic up -d --force-recreate embed"
}
