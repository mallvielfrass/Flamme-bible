package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

type aiSettings struct {
	Enabled      bool `json:"enabled"`
	MsgPer10Min  int  `json:"msg_per_10min"`
	MsgPerDay    int  `json:"msg_per_day"`
	MaxToolCalls int  `json:"max_tool_calls"`
}

type aiAdminView struct {
	Settings      aiSettings `json:"settings"`
	ActiveAnswers int        `json:"active_answers"`
	Messages24h   int        `json:"messages_24h"`
}

func defaultAISettings() aiSettings {
	return aiSettings{Enabled: true, MaxToolCalls: 8}
}

func (s *Server) ensureAISettings(ctx context.Context) error {
	cur := defaultAISettings()
	pairs := [][2]string{
		{"ai_enabled", aiBool(cur.Enabled)},
		{"ai_msg_per_10min", strconv.Itoa(cur.MsgPer10Min)},
		{"ai_msg_per_day", strconv.Itoa(cur.MsgPerDay)},
		{"ai_max_tool_calls", strconv.Itoa(cur.MaxToolCalls)},
	}
	for _, pair := range pairs {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO app_settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO NOTHING
		`, pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) loadAISettings(ctx context.Context) (aiSettings, error) {
	if err := s.ensureAISettings(ctx); err != nil {
		return defaultAISettings(), err
	}
	base := defaultAISettings()
	rows, err := s.db.Query(ctx, `
		SELECT key, value FROM app_settings
		WHERE key IN ('ai_enabled', 'ai_msg_per_10min', 'ai_msg_per_day', 'ai_max_tool_calls')
	`)
	if err != nil {
		return base, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return base, err
		}
		switch key {
		case "ai_enabled":
			base.Enabled = value == "true" || value == "1"
		case "ai_msg_per_10min":
			if n, err := strconv.Atoi(value); err == nil && n >= 0 {
				base.MsgPer10Min = n
			}
		case "ai_msg_per_day":
			if n, err := strconv.Atoi(value); err == nil && n >= 0 {
				base.MsgPerDay = n
			}
		case "ai_max_tool_calls":
			if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= 20 {
				base.MaxToolCalls = n
			}
		}
	}
	return base, rows.Err()
}

func validateAISettings(in aiSettings) (aiSettings, error) {
	if in.MsgPer10Min < 0 || in.MsgPerDay < 0 || in.MsgPer10Min > 100000 || in.MsgPerDay > 100000 {
		return aiSettings{}, errRequest
	}
	if in.MaxToolCalls < 1 || in.MaxToolCalls > 20 {
		return aiSettings{}, errRequest
	}
	return in, nil
}

func aiBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (s *Server) saveAISettings(ctx context.Context, actor string, in aiSettings) error {
	clean, err := validateAISettings(in)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	pairs := [][2]string{
		{"ai_enabled", aiBool(clean.Enabled)},
		{"ai_msg_per_10min", strconv.Itoa(clean.MsgPer10Min)},
		{"ai_msg_per_day", strconv.Itoa(clean.MsgPerDay)},
		{"ai_max_tool_calls", strconv.Itoa(clean.MaxToolCalls)},
	}
	for _, pair := range pairs {
		if err := s.putSetting(ctx, tx, pair[0], pair[1]); err != nil {
			return err
		}
	}
	state := "выкл"
	if clean.Enabled {
		state = "вкл"
	}
	note := fmt.Sprintf("%s, 10 мин %d, сутки %d, инструменты %d", state, clean.MsgPer10Min, clean.MsgPerDay, clean.MaxToolCalls)
	if err := writeAudit(ctx, tx, actor, "ai_settings", "ai", "settings", "ИИ", note); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) aiAdminView(ctx context.Context) (aiAdminView, error) {
	settings, err := s.loadAISettings(ctx)
	if err != nil {
		return aiAdminView{}, err
	}
	var messages int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM ai_messages WHERE created_at >= now() - interval '24 hours'`).Scan(&messages); err != nil {
		return aiAdminView{}, err
	}
	return aiAdminView{
		Settings:      settings,
		ActiveAnswers: s.aiRuns.active(),
		Messages24h:   messages,
	}, nil
}

func (s *Server) adminAI(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	view, err := s.aiAdminView(r.Context())
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) adminPutAI(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var body aiSettings
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	if err := s.saveAISettings(r.Context(), p.UserID, body); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := s.aiAdminView(r.Context())
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
