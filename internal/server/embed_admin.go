package server

import (
	"context"
	"net/http"
)

type embedQueueStats struct {
	Settings              embedSettings  `json:"settings"`
	MeasuredTokensPerSec  float64        `json:"measured_tokens_per_sec"`
	EffectiveTokensPerSec float64        `json:"effective_tokens_per_sec"`
	ModelState            string         `json:"model_state"`
	ActiveModel           string         `json:"active_model"`
	ActiveDim             int            `json:"active_dim"`
	LiveModel             string         `json:"live_model"`
	LiveDim               int            `json:"live_dim"`
	Pending               int            `json:"pending"`
	Processing            int            `json:"processing"`
	Failed                int            `json:"failed"`
	PendingTokens         int            `json:"pending_tokens"`
	ETASeconds            int            `json:"eta_seconds"`
	Songbooks             []bookQueue    `json:"songbooks"`
	FailedJobs            []failedJob    `json:"failed_jobs"`
	Model                 embedModelCard `json:"model"`
}

type bookQueue struct {
	SongbookID string `json:"songbook_id"`
	Title      string `json:"title"`
	Pending    int    `json:"pending"`
	Processing int    `json:"processing"`
	Failed     int    `json:"failed"`
}

type failedJob struct {
	ID         string `json:"id"`
	ObjectType string `json:"object_type"`
	SongbookID string `json:"songbook_id,omitempty"`
	FBID       string `json:"fbid,omitempty"`
	PartIndex  int    `json:"part_index,omitempty"`
	ModuleID   string `json:"module_id,omitempty"`
	Attempts   int    `json:"attempts"`
	LastError  string `json:"last_error"`
}

func (s *Server) adminEmbedQueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	stats, err := s.embedStats(r.Context())
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) adminPutEmbedSettings(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var body embedSettings
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	if err := s.saveEmbedSettings(r.Context(), p.UserID, body); err != nil {
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

func (s *Server) adminRetryEmbed(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE embedding_jobs
		SET status = 'pending', attempts = 0, last_error = '', not_before = now(), updated_at = now()
		WHERE status = 'failed'
	`)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := writeAudit(ctx, tx, p.UserID, "embed_retry", "embed", "queue", "индексация", fmtInt(tag.RowsAffected())); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	s.wakeEmbed()
	stats, err := s.embedStats(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) adminReindexEmbed(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := s.startShadowReindex(r.Context(), p.UserID); err != nil {
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

func (s *Server) embedStats(ctx context.Context) (embedQueueStats, error) {
	s.refreshEmbedLive(ctx)
	settings, err := s.loadEmbedSettings(ctx)
	if err != nil {
		return embedQueueStats{}, err
	}
	s.embedMu.Lock()
	measured := s.embedMeasured
	s.embedMu.Unlock()
	live := s.liveEmbed()
	activeModel, _ := s.setting(ctx, "embed_set_model")
	dimRaw, _ := s.setting(ctx, "embed_set_dim")
	activeDim := 0
	if n, err := parseInt(dimRaw); err == nil {
		activeDim = n
	}
	state := "ok"
	if s.shadowOpen(ctx) {
		state = "shadow"
	} else if s.modelMismatch(ctx) {
		state = "model_mismatch"
	}
	var pending, processing, failed, tokens int
	if err := s.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'pending'),
			count(*) FILTER (WHERE status = 'processing'),
			count(*) FILTER (WHERE status = 'failed'),
			COALESCE(sum(tokens) FILTER (WHERE status IN ('pending', 'processing')), 0)
		FROM embedding_jobs
	`).Scan(&pending, &processing, &failed, &tokens); err != nil {
		return embedQueueStats{}, err
	}
	rate := effectiveTokenRate(settings, measured)
	eta := 0
	if rate > 0 && tokens > 0 {
		eta = int(float64(tokens)/rate + 0.5)
	}
	books, err := s.bookQueues(ctx)
	if err != nil {
		return embedQueueStats{}, err
	}
	failedJobs, err := s.failedEmbedJobs(ctx)
	if err != nil {
		return embedQueueStats{}, err
	}
	return embedQueueStats{
		Settings:              settings,
		MeasuredTokensPerSec:  measured,
		EffectiveTokensPerSec: rate,
		ModelState:            state,
		ActiveModel:           activeModel,
		ActiveDim:             activeDim,
		LiveModel:             live.ModelID,
		LiveDim:               live.Dim,
		Pending:               pending,
		Processing:            processing,
		Failed:                failed,
		PendingTokens:         tokens,
		ETASeconds:            eta,
		Songbooks:             books,
		FailedJobs:            failedJobs,
		Model:                 s.modelCard(ctx, live, measured),
	}, nil
}

func parseInt(raw string) (int, error) {
	if raw == "" {
		return 0, strconvErr
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, strconvErr
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var strconvErr = errRequest

func (s *Server) bookQueues(ctx context.Context) ([]bookQueue, error) {
	rows, err := s.db.Query(ctx, `
		SELECT j.songbook_id::text, b.title,
		       count(*) FILTER (WHERE j.status = 'pending'),
		       count(*) FILTER (WHERE j.status = 'processing'),
		       count(*) FILTER (WHERE j.status = 'failed')
		FROM embedding_jobs j
		JOIN songbooks b ON b.id = j.songbook_id
		WHERE j.object_type = 'song_part' AND j.status <> 'done'
		GROUP BY j.songbook_id, b.title
		ORDER BY b.title, j.songbook_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []bookQueue{}
	for rows.Next() {
		var item bookQueue
		if err := rows.Scan(&item.SongbookID, &item.Title, &item.Pending, &item.Processing, &item.Failed); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *Server) failedEmbedJobs(ctx context.Context) ([]failedJob, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, object_type, COALESCE(songbook_id::text, ''), fbid, part_index,
		       COALESCE(module_id::text, ''), attempts, last_error
		FROM embedding_jobs
		WHERE status = 'failed'
		ORDER BY updated_at DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []failedJob{}
	for rows.Next() {
		var item failedJob
		if err := rows.Scan(&item.ID, &item.ObjectType, &item.SongbookID, &item.FBID, &item.PartIndex, &item.ModuleID, &item.Attempts, &item.LastError); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

type indexCount struct {
	Pending int
	Failed  int
}

func (s *Server) songIndexCounts(ctx context.Context, ids []string) (map[string]indexCount, error) {
	out := map[string]indexCount{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT songbook_id::text,
		       count(*) FILTER (WHERE status IN ('pending', 'processing')),
		       count(*) FILTER (WHERE status = 'failed')
		FROM embedding_jobs
		WHERE object_type = 'song_part' AND songbook_id::text = ANY($1::text[])
		GROUP BY songbook_id
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var item indexCount
		if err := rows.Scan(&id, &item.Pending, &item.Failed); err != nil {
			return nil, err
		}
		out[id] = item
	}
	return out, rows.Err()
}

func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
