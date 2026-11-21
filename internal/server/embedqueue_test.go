package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

type batchEmbedder struct {
	countingEmbedder
	mu      sync.Mutex
	batches []int
	fail    bool
}

func (e *batchEmbedder) Embed(ctx context.Context, kind string, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.batches = append(e.batches, len(texts))
	fail := e.fail
	e.mu.Unlock()
	if fail {
		return nil, errors.New("модель занята")
	}
	return e.countingEmbedder.Embed(ctx, kind, texts)
}

func (e *batchEmbedder) batchSizes() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := append([]int(nil), e.batches...)
	return out
}

func TestEmbedQueueBatch(t *testing.T) {
	emb := &batchEmbedder{}
	srv, svc := newSemanticServer(t, emb)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	pushParts(t, srv, owner, book.ID, 0, "grace", "Песня", []string{"обычный куплет", "припев про море", "ещё строка"}, false)
	sizes := emb.batchSizes()
	if len(sizes) != 1 || sizes[0] != 3 {
		t.Fatalf("пачки %v", sizes)
	}
	var pending, done int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE status = 'pending'), count(*) FILTER (WHERE status = 'done') FROM embedding_jobs`).Scan(&pending, &done); err != nil {
		t.Fatal(err)
	}
	if pending != 0 || done != 3 {
		t.Fatalf("очередь pending %d done %d", pending, done)
	}
	var rows int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM song_embeddings WHERE fbid = 'grace'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("векторы %d", rows)
	}

	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `UPDATE embedding_jobs SET status = 'processing'`); err != nil {
		t.Fatal(err)
	}
	if err := svc.resetEmbedProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	var processing int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM embedding_jobs WHERE status = 'processing'`).Scan(&processing); err != nil {
		t.Fatal(err)
	}
	if processing != 0 {
		t.Fatalf("зависшие processing %d", processing)
	}

	claimed, err := svc.claimEmbedJobs(ctx, 2, "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claim %d", len(claimed))
	}
	svc.releaseEmbedJobs(ctx, claimed)
}

func TestEmbedQueuePauseAndFail(t *testing.T) {
	emb := &batchEmbedder{}
	srv, svc := newSemanticServer(t, emb)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	svc.indexInline = false
	pushParts(t, srv, owner, book.ID, 0, "grace", "Песня", []string{"куплет один", "куплет два"}, false)
	if _, _, err := svc.indexSongs(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.saveEmbedSettings(context.Background(), owner.UserID, embedSettings{
		Mode: "manual", TokensPerSecond: 0, CapacityPercent: 50, BatchSize: 4, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}
	n, err := svc.embedOnce(context.Background(), false)
	if err != nil || n != 0 {
		t.Fatalf("пауза обработала %d %v", n, err)
	}
	var pending int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM embedding_jobs WHERE status = 'pending'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 2 {
		t.Fatalf("на паузе pending %d", pending)
	}
	emb.fail = true
	if err := svc.saveEmbedSettings(context.Background(), owner.UserID, embedSettings{
		Mode: "manual", TokensPerSecond: 1000, CapacityPercent: 50, BatchSize: 4,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.embedOnce(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	var failed, attempts int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*), COALESCE(max(attempts), 0) FROM embedding_jobs WHERE status = 'pending'`).Scan(&failed, &attempts); err != nil {
		t.Fatal(err)
	}
	if failed != 2 || attempts != 1 {
		t.Fatalf("ошибка пачки failed-status %d attempts %d", failed, attempts)
	}
	for i := 0; i < embedAttemptLimit+2; i++ {
		if _, err := testPool.Exec(context.Background(), `UPDATE embedding_jobs SET not_before = now() WHERE status = 'pending'`); err != nil {
			t.Fatal(err)
		}
		_, _ = svc.embedOnce(context.Background(), true)
		if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM embedding_jobs WHERE status = 'failed'`).Scan(&failed); err != nil {
			t.Fatal(err)
		}
		if failed == 2 {
			break
		}
	}
	if failed != 2 {
		t.Fatalf("не помечены failed: %d", failed)
	}
	if _, err := testPool.Exec(context.Background(), `
		UPDATE embedding_jobs SET status = 'pending', attempts = 0, last_error = '', not_before = now() WHERE status = 'failed'
	`); err != nil {
		t.Fatal(err)
	}
	emb.fail = false
	if err := svc.drainEmbedQueue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM song_embeddings`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("после повтора %d", rows)
	}
}

func TestEmbedSettingsAndMismatch(t *testing.T) {
	emb := &countingEmbedder{}
	srv, svc := newSemanticServer(t, emb)
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	code, raw := api(t, srv, owner.Token, http.MethodPut, "/api/admin/embed", map[string]any{
		"mode": "auto", "embed_tokens_per_second": 10, "embed_capacity_percent": 50, "embed_batch_size": 4,
	})
	if code != http.StatusForbidden {
		t.Fatalf("чужой %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPut, "/api/admin/embed", map[string]any{
		"mode": "manual", "embed_tokens_per_second": -1, "embed_capacity_percent": 50, "embed_batch_size": 4,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("границы %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPut, "/api/admin/embed", map[string]any{
		"mode": "manual", "embed_tokens_per_second": 20, "embed_capacity_percent": 40, "embed_batch_size": 2, "paused": true,
	})
	if code != http.StatusOK {
		t.Fatalf("сохранение %d %s", code, raw)
	}
	var stats embedQueueStats
	mustJSON(t, raw, &stats)
	if stats.Settings.Mode != "manual" || stats.Settings.TokensPerSecond != 20 || !stats.Settings.Paused || stats.EffectiveTokensPerSec != 0 {
		t.Fatalf("ответ %+v", stats.Settings)
	}
	var audits int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit WHERE action = 'embed_settings'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("журнал %d", audits)
	}

	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	rev := pushAt(t, srv, owner, book.ID, 0, []testSong{{"forgive", "После падения", "Бог даёт прощение тому, кто встал", false}})
	before := emb.Calls()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO app_settings (key, value) VALUES ('embed_set_model', 'other-model')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO app_settings (key, value) VALUES ('embed_set_dim', '312')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`); err != nil {
		t.Fatal(err)
	}
	svc.embedLive = embedRuntime{}
	svc.embedLiveAt = time.Time{}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/semantic?q="+urlQuery("прощение"), nil)
	if code != http.StatusOK {
		t.Fatalf("поиск %d %s", code, raw)
	}
	hits := semanticHits(t, raw)
	if len(hits) != 1 || hits[0].FBID != "forgive" {
		t.Fatalf("словесный запасной %+v", hits)
	}
	if emb.Calls() != before {
		t.Fatalf("при несовпадении модели вызвана модель %d -> %d", before, emb.Calls())
	}
	var vectors int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM song_embeddings`).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	pushAt(t, srv, owner, book.ID, rev, []testSong{{"sea", "Тихая вода", "лодка вышла в открытое море", false}})
	var jobs int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM embedding_jobs WHERE fbid = 'sea'`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("при несовпадении поставлено заданий %d", jobs)
	}
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM song_embeddings`).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/embed", nil)
	if code != http.StatusOK {
		t.Fatalf("статус %d %s", code, raw)
	}
	mustJSON(t, raw, &stats)
	if stats.ModelState != "model_mismatch" {
		t.Fatalf("состояние %s", stats.ModelState)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/embed/reindex", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("пересчёт %d %s", code, raw)
	}
	mustJSON(t, raw, &stats)
	if stats.ModelState == "model_mismatch" {
		t.Fatalf("после пересчёта %+v", stats)
	}
	var model string
	if err := testPool.QueryRow(context.Background(), `SELECT value FROM app_settings WHERE key = 'embed_set_model'`).Scan(&model); err != nil {
		t.Fatal(err)
	}
	if model != defaultEmbedModel {
		t.Fatalf("активная модель %s", model)
	}
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM song_embeddings WHERE fbid = 'sea'`).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	if vectors != 1 {
		t.Fatalf("теневой индекс не включился %d", vectors)
	}
}
