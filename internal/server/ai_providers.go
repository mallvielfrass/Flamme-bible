package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"flammebible/internal/ai/netguard"
	"flammebible/internal/ai/provider"
	"flammebible/internal/ai/secret"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type aiModelView struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
	CanTools    bool   `json:"can_tools"`
	CanVision   bool   `json:"can_vision"`
}

type aiProviderView struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	BaseURL    string        `json:"base_url"`
	TokenLast4 string        `json:"token_last4"`
	CheckedAt  *time.Time    `json:"checked_at"`
	CheckError string        `json:"check_error"`
	Models     []aiModelView `json:"models"`
}

type aiState struct {
	Providers []aiProviderView  `json:"providers"`
	Tasks     map[string]string `json:"tasks"`
}

type aiModelIn struct {
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
	CanTools    bool   `json:"can_tools"`
	CanVision   bool   `json:"can_vision"`
}

type aiTasksIn struct {
	Chat      string `json:"chat"`
	Vision    string `json:"vision"`
	ParseSong string `json:"parse_song"`
	Title     string `json:"title"`
}

type aiTestResult struct {
	OK    bool   `json:"ok"`
	Count int    `json:"count"`
	MS    int64  `json:"ms"`
	Error string `json:"error"`
}

func (s *Server) aiUser(r *http.Request) (principal, error) {
	if len(s.aiKey) != 32 {
		return principal{}, errAIDisabled
	}
	return s.principal(r)
}

func aiAAD(userID, providerID string) string {
	return userID + "|" + providerID
}

func cleanAIName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > 80 || strings.ContainsAny(name, "\r\n") {
		return "", errRequest
	}
	return name, nil
}

func cleanAIToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" || len(token) > 512 || strings.ContainsAny(token, "\r\n") {
		return "", errRequest
	}
	return token, nil
}

func cleanAIModel(raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model == "" || utf8.RuneCountInString(model) > 200 || strings.ContainsAny(model, "\r\n") {
		return "", errRequest
	}
	return model, nil
}

func (s *Server) cleanAIBase(ctx context.Context, raw string) (string, error) {
	if utf8.RuneCountInString(strings.TrimSpace(raw)) > 300 {
		return "", errRequest
	}
	var (
		canon string
		err   error
	)
	if s.aiResolve != nil {
		canon, err = netguard.ValidateBaseURLWith(ctx, raw, s.aiResolve)
	} else {
		canon, err = netguard.ValidateBaseURL(ctx, raw)
	}
	if err != nil {
		return "", errRequest
	}
	return canon, nil
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Server) listAIProviders(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	state, err := s.loadAIState(r.Context(), p.UserID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) createAIProvider(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
		Token   string `json:"token"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	name, err := cleanAIName(body.Name)
	if err != nil {
		writeMapped(w, err)
		return
	}
	baseURL, err := s.cleanAIBase(r.Context(), body.BaseURL)
	if err != nil {
		writeMapped(w, err)
		return
	}
	token, err := cleanAIToken(body.Token)
	if err != nil {
		writeMapped(w, err)
		return
	}
	view, err := s.insertAIProvider(r.Context(), p.UserID, name, baseURL, token)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) insertAIProvider(ctx context.Context, userID, name, baseURL, token string) (aiProviderView, error) {
	id := uuid.NewString()
	box, nonce, err := secret.Seal(s.aiKey, []byte(token), aiAAD(userID, id))
	if err != nil {
		return aiProviderView{}, err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO ai_providers (id, user_id, name, base_url, token_enc, token_nonce, token_last4)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, userID, name, baseURL, box, nonce, secret.Last4(token))
	if uniqueViolation(err) {
		return aiProviderView{}, errAIName
	}
	if err != nil {
		return aiProviderView{}, err
	}
	return s.loadAIProvider(ctx, userID, id)
}

func (s *Server) patchAIProvider(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		Name    *string `json:"name"`
		BaseURL *string `json:"base_url"`
	}
	if err := readJSON(r, &body); err != nil || (body.Name == nil && body.BaseURL == nil) {
		writeMapped(w, errRequest)
		return
	}
	current, err := s.loadAIProvider(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	name := current.Name
	if body.Name != nil {
		name, err = cleanAIName(*body.Name)
		if err != nil {
			writeMapped(w, err)
			return
		}
	}
	baseURL := current.BaseURL
	clearCheck := false
	if body.BaseURL != nil {
		baseURL, err = s.cleanAIBase(r.Context(), *body.BaseURL)
		if err != nil {
			writeMapped(w, err)
			return
		}
		clearCheck = baseURL != current.BaseURL
	}
	tag, err := s.db.Exec(r.Context(), `
		UPDATE ai_providers
		SET name = $3, base_url = $4,
		    checked_at = CASE WHEN $5 THEN NULL ELSE checked_at END,
		    check_error = CASE WHEN $5 THEN '' ELSE check_error END,
		    updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, p.UserID, name, baseURL, clearCheck)
	if uniqueViolation(err) {
		writeMapped(w, errAIName)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeMapped(w, errNotFound)
		return
	}
	view, err := s.loadAIProvider(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) putAIProviderToken(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	token, err := cleanAIToken(body.Token)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := s.loadAIProvider(r.Context(), p.UserID, id); err != nil {
		writeMapped(w, err)
		return
	}
	box, nonce, err := secret.Seal(s.aiKey, []byte(token), aiAAD(p.UserID, id))
	if err != nil {
		writeMapped(w, err)
		return
	}
	tag, err := s.db.Exec(r.Context(), `
		UPDATE ai_providers
		SET token_enc = $3, token_nonce = $4, token_last4 = $5,
		    checked_at = NULL, check_error = '', updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, p.UserID, box, nonce, secret.Last4(token))
	if err != nil {
		writeMapped(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeMapped(w, errNotFound)
		return
	}
	view, err := s.loadAIProvider(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) deleteAIProvider(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM ai_providers WHERE id = $1 AND user_id = $2`, id, p.UserID)
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

func (s *Server) testAIProvider(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	baseURL, token, err := s.aiToken(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	start := time.Now()
	ids, callErr := s.aiClient(baseURL, token).ListModels(r.Context())
	result := aiTestResult{MS: time.Since(start).Milliseconds()}
	if callErr != nil {
		result.Error = secret.Scrub(callErr.Error(), token)
	} else {
		result.OK = true
		result.Count = len(ids)
	}
	if err := s.saveAICheck(r.Context(), p.UserID, id, result.Error); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) remoteAIModels(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	baseURL, token, err := s.aiToken(r.Context(), p.UserID, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	ids, err := s.aiClient(baseURL, token).ListModels(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errJSON(secret.Scrub(err.Error(), token)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": ids})
}

func (s *Server) putAIModels(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var items []aiModelIn
	if err := readJSON(r, &items); err != nil || items == nil {
		writeMapped(w, errRequest)
		return
	}
	models, err := s.replaceAIModels(r.Context(), p.UserID, id, items)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) putAITasks(w http.ResponseWriter, r *http.Request) {
	p, err := s.aiUser(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body aiTasksIn
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	tasks, err := s.setAITasks(r.Context(), p.UserID, body)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) aiClient(baseURL, token string) *provider.Client {
	return &provider.Client{BaseURL: baseURL, Token: token, HTTP: s.aiHTTP}
}

func (s *Server) aiToken(ctx context.Context, userID, id string) (string, string, error) {
	var baseURL string
	var box, nonce []byte
	err := s.db.QueryRow(ctx, `
		SELECT base_url, token_enc, token_nonce
		FROM ai_providers
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&baseURL, &box, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", errNotFound
	}
	if err != nil {
		return "", "", err
	}
	plain, err := secret.Open(s.aiKey, box, nonce, aiAAD(userID, id))
	if err != nil {
		return "", "", err
	}
	return baseURL, string(plain), nil
}

func (s *Server) saveAICheck(ctx context.Context, userID, id, checkErr string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE ai_providers
		SET checked_at = now(), check_error = $3, updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID, checkErr)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound
	}
	return nil
}

func (s *Server) loadAIState(ctx context.Context, userID string) (aiState, error) {
	state := aiState{Providers: []aiProviderView{}}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, base_url, token_last4, checked_at, check_error
		FROM ai_providers
		WHERE user_id = $1
		ORDER BY name
	`, userID)
	if err != nil {
		return aiState{}, err
	}
	defer rows.Close()
	index := map[string]int{}
	for rows.Next() {
		var view aiProviderView
		if err := rows.Scan(&view.ID, &view.Name, &view.BaseURL, &view.TokenLast4, &view.CheckedAt, &view.CheckError); err != nil {
			return aiState{}, err
		}
		view.Models = []aiModelView{}
		index[view.ID] = len(state.Providers)
		state.Providers = append(state.Providers, view)
	}
	if err := rows.Err(); err != nil {
		return aiState{}, err
	}
	modelRows, err := s.db.Query(ctx, `
		SELECT m.provider_id::text, m.id::text, m.model, m.display_name, m.can_tools, m.can_vision
		FROM ai_models m
		JOIN ai_providers p ON p.id = m.provider_id
		WHERE p.user_id = $1
		ORDER BY m.model
	`, userID)
	if err != nil {
		return aiState{}, err
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var providerID string
		var model aiModelView
		if err := modelRows.Scan(&providerID, &model.ID, &model.Model, &model.DisplayName, &model.CanTools, &model.CanVision); err != nil {
			return aiState{}, err
		}
		if n, ok := index[providerID]; ok {
			state.Providers[n].Models = append(state.Providers[n].Models, model)
		}
	}
	if err := modelRows.Err(); err != nil {
		return aiState{}, err
	}
	state.Tasks, err = s.loadAITasks(ctx, userID)
	if err != nil {
		return aiState{}, err
	}
	return state, nil
}

func (s *Server) loadAIProvider(ctx context.Context, userID, id string) (aiProviderView, error) {
	state, err := s.loadAIState(ctx, userID)
	if err != nil {
		return aiProviderView{}, err
	}
	for _, view := range state.Providers {
		if view.ID == id {
			return view, nil
		}
	}
	return aiProviderView{}, errNotFound
}

func (s *Server) loadAIModels(ctx context.Context, providerID string) ([]aiModelView, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, model, display_name, can_tools, can_vision
		FROM ai_models
		WHERE provider_id = $1
		ORDER BY model
	`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []aiModelView{}
	for rows.Next() {
		var model aiModelView
		if err := rows.Scan(&model.ID, &model.Model, &model.DisplayName, &model.CanTools, &model.CanVision); err != nil {
			return nil, err
		}
		out = append(out, model)
	}
	return out, rows.Err()
}

func (s *Server) loadAITasks(ctx context.Context, userID string) (map[string]string, error) {
	out := map[string]string{"chat": "", "vision": "", "parse_song": "", "title": ""}
	rows, err := s.db.Query(ctx, `SELECT task, model_id::text FROM ai_task_models WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var task, modelID string
		if err := rows.Scan(&task, &modelID); err != nil {
			return nil, err
		}
		if _, ok := out[task]; ok {
			out[task] = modelID
		}
	}
	return out, rows.Err()
}

func (s *Server) replaceAIModels(ctx context.Context, userID, providerID string, items []aiModelIn) ([]aiModelView, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var owns bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_providers WHERE id = $1 AND user_id = $2)`, providerID, userID).Scan(&owns); err != nil {
		return nil, err
	}
	if !owns {
		return nil, errNotFound
	}
	rows, err := tx.Query(ctx, `SELECT id::text, model FROM ai_models WHERE provider_id = $1`, providerID)
	if err != nil {
		return nil, err
	}
	existing := map[string]string{}
	for rows.Next() {
		var id, model string
		if err := rows.Scan(&id, &model); err != nil {
			rows.Close()
			return nil, err
		}
		existing[model] = id
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	keep := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		model, err := cleanAIModel(item.Model)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[model]; ok {
			return nil, errRequest
		}
		seen[model] = struct{}{}
		display := strings.TrimSpace(item.DisplayName)
		if strings.ContainsAny(display, "\r\n") || utf8.RuneCountInString(display) > 200 {
			return nil, errRequest
		}
		if display == "" {
			display = model
		}
		if id, ok := existing[model]; ok {
			_, err = tx.Exec(ctx, `
				UPDATE ai_models
				SET display_name = $2, can_tools = $3, can_vision = $4
				WHERE id = $1 AND provider_id = $5
			`, id, display, item.CanTools, item.CanVision, providerID)
		} else {
			_, err = tx.Exec(ctx, `
				INSERT INTO ai_models (id, provider_id, model, display_name, can_tools, can_vision)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, uuid.NewString(), providerID, model, display, item.CanTools, item.CanVision)
		}
		if err != nil {
			return nil, err
		}
		keep = append(keep, model)
	}
	if len(keep) == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM ai_models WHERE provider_id = $1`, providerID)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM ai_models WHERE provider_id = $1 AND NOT (model = ANY($2))`, providerID, keep)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.loadAIModels(ctx, providerID)
}

func (s *Server) setAITasks(ctx context.Context, userID string, in aiTasksIn) (map[string]string, error) {
	pairs := []struct {
		task  string
		model string
	}{
		{"chat", in.Chat},
		{"vision", in.Vision},
		{"parse_song", in.ParseSong},
		{"title", in.Title},
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	for _, pair := range pairs {
		if _, err := tx.Exec(ctx, `DELETE FROM ai_task_models WHERE user_id = $1 AND task = $2`, userID, pair.task); err != nil {
			return nil, err
		}
		modelID := strings.TrimSpace(pair.model)
		if modelID == "" {
			continue
		}
		if _, err := uuid.Parse(modelID); err != nil {
			return nil, errRequest
		}
		var owns bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM ai_models m
				JOIN ai_providers p ON p.id = m.provider_id
				WHERE m.id = $1 AND p.user_id = $2
			)
		`, modelID, userID).Scan(&owns); err != nil {
			return nil, err
		}
		if !owns {
			return nil, errNotFound
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_task_models (user_id, task, model_id) VALUES ($1, $2, $3)
		`, userID, pair.task, modelID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.loadAITasks(ctx, userID)
}
