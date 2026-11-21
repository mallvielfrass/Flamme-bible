package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const embedLiveTTL = 30 * time.Second

type songPartJob struct {
	SongbookID string
	FBID       string
	Index      int
	Text       string
	Hash       string
}

type biblePartJob struct {
	ModuleID string
	Draft    passageDraft
	Hash     string
}

type embedJob struct {
	ID         string
	ObjectType string
	SongbookID string
	FBID       string
	PartIndex  int
	ModuleID   string
	Book       int
	Chapter    int
	VerseStart int
	VerseEnd   int
	BookName   string
	Reference  string
	Hash       string
	Body       string
	Model      string
	Dim        int
	Target     string
}

func EmbedConfigFromEnv() embedEnv {
	return embedEnv{
		TokensPerSecond: envInt("EMBED_TOKENS_PER_SECOND", 32),
		CapacityPercent: envInt("EMBED_CAPACITY_PERCENT", defaultCapacityPercent),
		BatchSize:       envInt("EMBED_BATCH_SIZE", defaultEmbedBatch),
		Interval:        envDuration("EMBED_INTERVAL", time.Second),
		Concurrency:     envInt("EMBED_CONCURRENCY", 1),
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 100*time.Millisecond {
		return fallback
	}
	return d
}

func (s *Server) SetEmbedConfig(cfg embedEnv) {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Second
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.Concurrency > 2 {
		cfg.Concurrency = 2
	}
	s.embedCfg = cfg
}

func (s *Server) EnsureEmbedSettings(ctx context.Context) error {
	return s.ensureEmbedSettings(ctx)
}

func (s *Server) DrainEmbedQueue(ctx context.Context) error {
	return s.drainEmbedQueue(ctx)
}

func (s *Server) ensureEmbedSettings(ctx context.Context) error {
	cur := defaultEmbedSettings(s.embedCfg)
	pairs := [][2]string{
		{"embed_mode", cur.Mode},
		{"embed_tokens_per_second", strconv.Itoa(cur.TokensPerSecond)},
		{"embed_capacity_percent", strconv.Itoa(cur.CapacityPercent)},
		{"embed_batch_size", strconv.Itoa(cur.BatchSize)},
		{"embed_paused", "0"},
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

func (s *Server) loadEmbedSettings(ctx context.Context) (embedSettings, error) {
	base := defaultEmbedSettings(s.embedCfg)
	rows, err := s.db.Query(ctx, `
		SELECT key, value FROM app_settings
		WHERE key IN (
			'embed_mode', 'embed_tokens_per_second', 'embed_capacity_percent',
			'embed_batch_size', 'embed_paused'
		)
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
		case "embed_mode":
			if value == "manual" || value == "auto" {
				base.Mode = value
			}
		case "embed_tokens_per_second":
			if n, err := strconv.Atoi(value); err == nil {
				base.TokensPerSecond = n
			}
		case "embed_capacity_percent":
			if n, err := strconv.Atoi(value); err == nil {
				base.CapacityPercent = n
			}
		case "embed_batch_size":
			if n, err := strconv.Atoi(value); err == nil {
				base.BatchSize = n
			}
		case "embed_paused":
			base.Paused = value == "1" || value == "true"
		}
	}
	return base, rows.Err()
}

func (s *Server) saveEmbedSettings(ctx context.Context, actor string, in embedSettings) error {
	clean, err := validateEmbedSettings(in)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	pairs := [][2]string{
		{"embed_mode", clean.Mode},
		{"embed_tokens_per_second", strconv.Itoa(clean.TokensPerSecond)},
		{"embed_capacity_percent", strconv.Itoa(clean.CapacityPercent)},
		{"embed_batch_size", strconv.Itoa(clean.BatchSize)},
		{"embed_paused", boolSetting(clean.Paused)},
	}
	for _, pair := range pairs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO app_settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
		`, pair[0], pair[1]); err != nil {
			return err
		}
	}
	note := fmt.Sprintf("%s %d ток/с %d%% пачка %d", clean.Mode, clean.TokensPerSecond, clean.CapacityPercent, clean.BatchSize)
	if clean.Paused {
		note += " пауза"
	}
	if err := writeAudit(ctx, tx, actor, "embed_settings", "embed", "queue", "индексация", note); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.wakeEmbed()
	return nil
}

func boolSetting(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func (s *Server) setting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = $1`, key).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Server) putSetting(ctx context.Context, tx pgx.Tx, key, value string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, key, value)
	return err
}

type modelSource interface {
	Info(ctx context.Context) (embedRuntime, error)
}

func (s *Server) refreshEmbedLive(ctx context.Context) {
	s.embedMu.Lock()
	fresh := time.Since(s.embedLiveAt) < embedLiveTTL && s.embedLive.ModelID != ""
	s.embedMu.Unlock()
	if fresh || s.embedder == nil {
		return
	}
	info, err := s.readEmbedLive(ctx)
	if err != nil {
		log.Printf("модель эмбеддинга: %v", err)
		return
	}
	s.embedMu.Lock()
	s.embedLive = info
	s.embedLiveAt = time.Now()
	s.embedMu.Unlock()
}

func (s *Server) readEmbedLive(ctx context.Context) (embedRuntime, error) {
	if src, ok := s.embedder.(modelSource); ok {
		return src.Info(ctx)
	}
	return embedRuntime{ModelID: s.embedder.Model(), Dim: embedDim, MaxTokens: embedMaxTokens}, nil
}

func (s *Server) liveEmbed() embedRuntime {
	s.embedMu.Lock()
	defer s.embedMu.Unlock()
	return s.embedLive
}

func (s *Server) blockActiveEmbed(ctx context.Context) bool {
	s.refreshEmbedLive(ctx)
	return s.modelMismatch(ctx)
}

func (s *Server) modelMismatch(ctx context.Context) bool {
	model, _ := s.setting(ctx, "embed_set_model")
	dimRaw, _ := s.setting(ctx, "embed_set_dim")
	if model == "" {
		return false
	}
	live := s.liveEmbed()
	if live.ModelID == "" {
		return false
	}
	if live.ModelID != model {
		return true
	}
	dim, _ := strconv.Atoi(dimRaw)
	return dim > 0 && live.Dim > 0 && dim != live.Dim
}

func (s *Server) shadowOpen(ctx context.Context) bool {
	v, _ := s.setting(ctx, "embed_shadow")
	return v == "1"
}

func (s *Server) wakeEmbed() {
	if s.embedWake == nil {
		return
	}
	select {
	case s.embedWake <- struct{}{}:
	default:
	}
}

func (s *Server) afterEmbedEnqueue(ctx context.Context) error {
	if s.indexInline {
		return s.drainEmbedQueue(ctx)
	}
	s.wakeEmbed()
	return nil
}

func (s *Server) drainEmbedQueue(ctx context.Context) error {
	for {
		n, err := s.embedOnce(ctx, true)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

func (s *Server) resetEmbedProcessing(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		UPDATE embedding_jobs
		SET status = 'pending', updated_at = now()
		WHERE status = 'processing'
	`)
	return err
}

func (s *Server) enqueueSongParts(ctx context.Context, target string, parts []songPartJob) error {
	if target != "next" && s.blockActiveEmbed(ctx) {
		return nil
	}
	if len(parts) == 0 {
		return nil
	}
	live := s.liveEmbed()
	model := s.embedder.Model()
	if live.ModelID != "" {
		model = live.ModelID
	}
	dim := live.Dim
	if dim <= 0 {
		dim = embedDim
	}
	const chunk = 200
	for start := 0; start < len(parts); start += chunk {
		end := start + chunk
		if end > len(parts) {
			end = len(parts)
		}
		if err := s.insertSongJobs(ctx, target, model, dim, parts[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) insertSongJobs(ctx context.Context, target, model string, dim int, parts []songPartJob) error {
	ids := make([]string, len(parts))
	books := make([]string, len(parts))
	fbids := make([]string, len(parts))
	indexes := make([]int32, len(parts))
	hashes := make([]string, len(parts))
	bodies := make([]string, len(parts))
	tokens := make([]int32, len(parts))
	for i, part := range parts {
		ids[i] = uuid.NewString()
		books[i] = part.SongbookID
		fbids[i] = part.FBID
		indexes[i] = int32(part.Index)
		hashes[i] = part.Hash
		bodies[i] = part.Text
		tokens[i] = int32(approxTokens(part.Text))
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO embedding_jobs (
			id, object_type, songbook_id, fbid, part_index, content_hash, body,
			model, dimension, target, tokens, status, attempts, last_error, not_before, updated_at
		)
		SELECT id::uuid, 'song_part', songbook_id::uuid, fbid, part_index, content_hash, body,
		       $2, $3, $4, tokens, 'pending', 0, '', now(), now()
		FROM unnest($1::text[], $5::text[], $6::text[], $7::int[], $8::text[], $9::text[], $10::int[])
		  AS t(id, songbook_id, fbid, part_index, content_hash, body, tokens)
		ON CONFLICT (songbook_id, fbid, part_index, target) WHERE object_type = 'song_part'
		DO UPDATE SET
			content_hash = EXCLUDED.content_hash,
			body = EXCLUDED.body,
			model = EXCLUDED.model,
			dimension = EXCLUDED.dimension,
			tokens = EXCLUDED.tokens,
			status = 'pending',
			attempts = 0,
			last_error = '',
			not_before = now(),
			updated_at = now()
		WHERE embedding_jobs.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		   OR embedding_jobs.status IN ('failed', 'done')
	`, ids, model, dim, target, books, fbids, indexes, hashes, bodies, tokens)
	return err
}

func (s *Server) enqueueBibleParts(ctx context.Context, target, moduleID string, parts []biblePartJob) error {
	if target != "next" && s.blockActiveEmbed(ctx) {
		return nil
	}
	if len(parts) == 0 || s.embedder == nil {
		return nil
	}
	live := s.liveEmbed()
	model := s.embedder.Model()
	if live.ModelID != "" {
		model = live.ModelID
	}
	dim := live.Dim
	if dim <= 0 {
		dim = embedDim
	}
	const chunk = 200
	for start := 0; start < len(parts); start += chunk {
		end := start + chunk
		if end > len(parts) {
			end = len(parts)
		}
		if err := s.insertBibleJobs(ctx, target, moduleID, model, dim, parts[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) insertBibleJobs(ctx context.Context, target, moduleID, model string, dim int, parts []biblePartJob) error {
	n := len(parts)
	ids := make([]string, n)
	books := make([]int32, n)
	chapters := make([]int32, n)
	starts := make([]int32, n)
	ends := make([]int32, n)
	names := make([]string, n)
	refs := make([]string, n)
	hashes := make([]string, n)
	bodies := make([]string, n)
	tokens := make([]int32, n)
	for i, part := range parts {
		ids[i] = uuid.NewString()
		books[i] = int32(part.Draft.Book)
		chapters[i] = int32(part.Draft.Chapter)
		starts[i] = int32(part.Draft.VerseStart)
		ends[i] = int32(part.Draft.VerseEnd)
		names[i] = part.Draft.BookName
		refs[i] = part.Draft.Reference
		hashes[i] = part.Hash
		bodies[i] = part.Draft.Body
		tokens[i] = int32(approxTokens(part.Draft.Body))
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO embedding_jobs (
			id, object_type, module_id, book, chapter, verse_start, verse_end, book_name, reference,
			content_hash, body, model, dimension, target, tokens, status, attempts, last_error, not_before, updated_at
		)
		SELECT id::uuid, 'bible_passage', $2::uuid, book, chapter, verse_start, verse_end, book_name, reference,
		       content_hash, body, $3, $4, $5, tokens, 'pending', 0, '', now(), now()
		FROM unnest($1::text[], $6::int[], $7::int[], $8::int[], $9::int[], $10::text[], $11::text[], $12::text[], $13::text[], $14::int[])
		  AS t(id, book, chapter, verse_start, verse_end, book_name, reference, content_hash, body, tokens)
		ON CONFLICT (module_id, book, chapter, verse_start, target) WHERE object_type = 'bible_passage'
		DO UPDATE SET
			verse_end = EXCLUDED.verse_end,
			book_name = EXCLUDED.book_name,
			reference = EXCLUDED.reference,
			content_hash = EXCLUDED.content_hash,
			body = EXCLUDED.body,
			model = EXCLUDED.model,
			dimension = EXCLUDED.dimension,
			tokens = EXCLUDED.tokens,
			status = 'pending',
			attempts = 0,
			last_error = '',
			not_before = now(),
			updated_at = now()
		WHERE embedding_jobs.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		   OR embedding_jobs.status IN ('failed', 'done')
	`, ids, moduleID, model, dim, target, books, chapters, starts, ends, names, refs, hashes, bodies, tokens)
	return err
}

func (s *Server) claimEmbedJobs(ctx context.Context, limit int, target string) ([]embedJob, error) {
	if limit < 1 {
		limit = 1
	}
	if target != "active" && target != "next" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		UPDATE embedding_jobs
		SET status = 'processing', updated_at = now()
		WHERE id IN (
			SELECT id FROM embedding_jobs
			WHERE status = 'pending' AND not_before <= now() AND target = $2
			ORDER BY updated_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		RETURNING id::text, object_type, COALESCE(songbook_id::text, ''), fbid, part_index,
		          COALESCE(module_id::text, ''), book, chapter, verse_start, verse_end, book_name, reference,
		          content_hash, body, model, dimension, target
	`, limit, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []embedJob
	for rows.Next() {
		var job embedJob
		if err := rows.Scan(
			&job.ID, &job.ObjectType, &job.SongbookID, &job.FBID, &job.PartIndex,
			&job.ModuleID, &job.Book, &job.Chapter, &job.VerseStart, &job.VerseEnd, &job.BookName, &job.Reference,
			&job.Hash, &job.Body, &job.Model, &job.Dim, &job.Target,
		); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *Server) countTokens(ctx context.Context, texts []string) []int {
	if tok, ok := s.embedder.(interface {
		Tokenize(context.Context, []string) ([]int, error)
	}); ok {
		counts, err := tok.Tokenize(ctx, texts)
		if err == nil && len(counts) == len(texts) {
			return counts
		}
	}
	out := make([]int, len(texts))
	for i, text := range texts {
		out[i] = approxTokens(text)
	}
	return out
}

func (s *Server) finishEmbedJobs(ctx context.Context, jobs []embedJob, vecs [][]float32) error {
	if len(jobs) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := upsertEmbedBatch(ctx, tx, jobs, vecs); err != nil {
		return err
	}
	ids := jobIDs(jobs)
	if _, err := tx.Exec(ctx, `
		UPDATE embedding_jobs
		SET status = 'done', last_error = '', updated_at = now()
		WHERE id::text = ANY($1::text[])
	`, ids); err != nil {
		return err
	}
	if jobs[0].Target == "active" {
		if err := rememberVectorSet(ctx, tx, jobs[0].Model, jobs[0].Dim); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func rememberVectorSet(ctx context.Context, tx pgx.Tx, model string, dim int) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ('embed_set_model', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
		WHERE app_settings.value = ''
	`, model); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ('embed_set_dim', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
		WHERE app_settings.value = ''
	`, strconv.Itoa(dim))
	return err
}

func upsertEmbedBatch(ctx context.Context, tx pgx.Tx, jobs []embedJob, vecs [][]float32) error {
	var songs, passages []int
	for i, job := range jobs {
		if len(vecs[i]) != job.Dim && job.Dim > 0 {
			return fmt.Errorf("вектор длины %d, у задания %d", len(vecs[i]), job.Dim)
		}
		switch job.ObjectType {
		case "song_part":
			songs = append(songs, i)
		case "bible_passage":
			passages = append(passages, i)
		default:
			return fmt.Errorf("неизвестный объект %s", job.ObjectType)
		}
	}
	if len(songs) > 0 {
		if err := upsertSongBatch(ctx, tx, songTable(jobs[songs[0]].Target), jobs, vecs, songs); err != nil {
			return err
		}
	}
	if len(passages) > 0 {
		if err := upsertPassageBatch(ctx, tx, passageTable(jobs[passages[0]].Target), jobs, vecs, passages); err != nil {
			return err
		}
	}
	return nil
}

func songTable(target string) string {
	if target == "next" {
		return "song_embeddings_next"
	}
	return "song_embeddings"
}

func passageTable(target string) string {
	if target == "next" {
		return "bible_passages_next"
	}
	return "bible_passages"
}

func upsertSongBatch(ctx context.Context, tx pgx.Tx, table string, jobs []embedJob, vecs [][]float32, idx []int) error {
	books := make([]string, len(idx))
	fbids := make([]string, len(idx))
	parts := make([]int32, len(idx))
	models := make([]string, len(idx))
	hashes := make([]string, len(idx))
	vectors := make([]string, len(idx))
	for i, n := range idx {
		books[i] = jobs[n].SongbookID
		fbids[i] = jobs[n].FBID
		parts[i] = int32(jobs[n].PartIndex)
		models[i] = jobs[n].Model
		hashes[i] = jobs[n].Hash
		vectors[i] = vectorLiteral(vecs[n])
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO `+table+` (songbook_id, fbid, part_index, model, text_hash, embedding)
		SELECT songbook_id::uuid, fbid, part_index, model, text_hash, embedding::vector
		FROM unnest($1::text[], $2::text[], $3::int[], $4::text[], $5::text[], $6::text[])
		  AS t(songbook_id, fbid, part_index, model, text_hash, embedding)
		ON CONFLICT (songbook_id, fbid, part_index) DO UPDATE SET
			model = EXCLUDED.model,
			text_hash = EXCLUDED.text_hash,
			embedding = EXCLUDED.embedding
	`, books, fbids, parts, models, hashes, vectors)
	return err
}

func upsertPassageBatch(ctx context.Context, tx pgx.Tx, table string, jobs []embedJob, vecs [][]float32, idx []int) error {
	n := len(idx)
	ids := make([]string, n)
	modules := make([]string, n)
	books := make([]int32, n)
	names := make([]string, n)
	chapters := make([]int32, n)
	starts := make([]int32, n)
	ends := make([]int32, n)
	refs := make([]string, n)
	bodies := make([]string, n)
	models := make([]string, n)
	hashes := make([]string, n)
	vectors := make([]string, n)
	for i, at := range idx {
		job := jobs[at]
		ids[i] = uuid.NewString()
		modules[i] = job.ModuleID
		books[i] = int32(job.Book)
		names[i] = job.BookName
		chapters[i] = int32(job.Chapter)
		starts[i] = int32(job.VerseStart)
		ends[i] = int32(job.VerseEnd)
		refs[i] = job.Reference
		bodies[i] = job.Body
		models[i] = job.Model
		hashes[i] = job.Hash
		vectors[i] = vectorLiteral(vecs[at])
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO `+table+` (
			id, module_id, book, book_name, chapter, verse_start, verse_end,
			reference, body, model, text_hash, embedding
		)
		SELECT id::uuid, module_id::uuid, book, book_name, chapter, verse_start, verse_end,
		       reference, body, model, text_hash, embedding::vector
		FROM unnest(
			$1::text[], $2::text[], $3::int[], $4::text[], $5::int[], $6::int[], $7::int[],
			$8::text[], $9::text[], $10::text[], $11::text[], $12::text[]
		) AS t(
			id, module_id, book, book_name, chapter, verse_start, verse_end,
			reference, body, model, text_hash, embedding
		)
		ON CONFLICT (module_id, book, chapter, verse_start) DO UPDATE SET
			verse_end = EXCLUDED.verse_end,
			book_name = EXCLUDED.book_name,
			reference = EXCLUDED.reference,
			body = EXCLUDED.body,
			model = EXCLUDED.model,
			text_hash = EXCLUDED.text_hash,
			embedding = EXCLUDED.embedding
	`, ids, modules, books, names, chapters, starts, ends, refs, bodies, models, hashes, vectors)
	return err
}

func (s *Server) failEmbedJobs(ctx context.Context, jobs []embedJob, cause error) error {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.db.Exec(ctx, `
		UPDATE embedding_jobs
		SET attempts = attempts + 1,
		    last_error = $2,
		    status = CASE WHEN attempts + 1 >= $3 THEN 'failed' ELSE 'pending' END,
		    not_before = CASE
		      WHEN attempts + 1 >= $3 THEN now()
		      ELSE now() + make_interval(secs => LEAST(60, power(2, attempts))::double precision)
		    END,
		    updated_at = now()
		WHERE id::text = ANY($1::text[])
	`, jobIDs(jobs), msg, embedAttemptLimit)
	return err
}

func (s *Server) releaseEmbedJobs(ctx context.Context, jobs []embedJob) {
	if len(jobs) == 0 {
		return
	}
	_, _ = s.db.Exec(ctx, `
		UPDATE embedding_jobs
		SET status = 'pending', updated_at = now()
		WHERE id::text = ANY($1::text[]) AND status = 'processing'
	`, jobIDs(jobs))
}

func jobIDs(jobs []embedJob) []string {
	ids := make([]string, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	return ids
}

func (s *Server) embedOnce(ctx context.Context, force bool) (int, error) {
	if s.embedder == nil {
		return 0, nil
	}
	s.refreshEmbedLive(ctx)
	settings, err := s.loadEmbedSettings(ctx)
	if err != nil {
		return 0, err
	}
	s.embedMu.Lock()
	measured := s.embedMeasured
	s.embedMu.Unlock()
	rate := effectiveTokenRate(settings, measured)
	if !force && rate <= 0 {
		return 0, nil
	}
	target := "active"
	if s.shadowOpen(ctx) {
		target = "next"
	} else if !force && s.modelMismatch(ctx) {
		return 0, nil
	}
	if target == "active" && s.modelMismatch(ctx) {
		return 0, nil
	}
	jobs, err := s.claimEmbedJobs(ctx, settings.BatchSize, target)
	if err != nil || len(jobs) == 0 {
		return 0, err
	}
	texts := make([]string, len(jobs))
	bodies := make([]string, len(jobs))
	for i, job := range jobs {
		bodies[i] = job.Body
		texts[i] = prefixEmbed(s.embedder.Model(), embedPassage, job.Body)
	}
	counts := s.countTokens(ctx, texts)
	total := 0
	for _, n := range counts {
		total += n
	}
	if !force {
		s.embedMu.Lock()
		s.embedBucket.setRate(rate, time.Now())
		wait := s.embedBucket.acquire(time.Now(), float64(total))
		s.embedMu.Unlock()
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				s.releaseEmbedJobs(ctx, jobs)
				return 0, ctx.Err()
			case <-timer.C:
			}
			s.embedMu.Lock()
			s.embedBucket.acquire(time.Now(), float64(total))
			s.embedMu.Unlock()
		}
	}
	started := time.Now()
	vecs, err := s.embedder.Embed(ctx, embedPassage, bodies)
	elapsed := time.Since(started)
	if err != nil {
		return 0, s.failEmbedJobs(ctx, jobs, err)
	}
	if len(vecs) != len(jobs) {
		return 0, s.failEmbedJobs(ctx, jobs, fmt.Errorf("эмбеддер вернул %d векторов на %d текстов", len(vecs), len(jobs)))
	}
	s.embedMu.Lock()
	s.embedMeasured, s.embedSamples = nextCapacity(s.embedMeasured, s.embedSamples, total, elapsed)
	s.embedMu.Unlock()
	if err := s.finishEmbedJobs(ctx, jobs, vecs); err != nil {
		return 0, s.failEmbedJobs(ctx, jobs, err)
	}
	if err := s.maybeSwitchShadow(ctx); err != nil {
		log.Printf("переключение индекса: %v", err)
	}
	return len(jobs), nil
}

func (s *Server) embedWorker(ctx context.Context, lead bool) {
	if lead {
		if err := s.ensureEmbedSettings(ctx); err != nil {
			log.Printf("настройки индекса: %v", err)
		}
		if err := s.resetEmbedProcessing(ctx); err != nil {
			log.Printf("очередь индекса: %v", err)
		}
		s.refreshEmbedLive(ctx)
		if !s.modelMismatch(ctx) {
			if _, _, err := s.reindexSongs(ctx); err != nil {
				log.Printf("индекс песен: %v", err)
			}
		} else {
			log.Printf("индекс: модель не совпадает с векторами, автопересчёт не запускается")
		}
	}
	idle := s.embedCfg.Interval
	if idle <= 0 {
		idle = time.Second
	}
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.embedWake:
		case <-timer.C:
		}
		if _, err := s.embedOnce(ctx, false); err != nil && ctx.Err() == nil {
			log.Printf("индекс: %v", err)
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(idle)
	}
}

func (s *Server) startEmbedWorkers(ctx context.Context) {
	if s.embedWake != nil || s.embedder == nil {
		return
	}
	s.embedWake = make(chan struct{}, 1)
	n := s.embedCfg.Concurrency
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		lead := i == 0
		go s.embedWorker(ctx, lead)
	}
}
