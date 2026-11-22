package server

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func songOp(id, fbid, title string, order []string) map[string]any {
	op := map[string]any{
		"operation_id": id,
		"song": map[string]any{
			"fbid":  fbid,
			"title": title,
			"parts": []any{},
		},
	}
	if order != nil {
		op["order"] = order
	}
	return op
}

func pushBody(base int64, op map[string]any) map[string]any {
	return map[string]any{
		"base_revision": base,
		"operations":    []any{op},
		"owner_id":      uuid.NewString(),
	}
}

type pushResult struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Revision    int64  `json:"revision"`
	Server      *struct {
		Title string `json:"title"`
	} `json:"server"`
	Client struct {
		Title string `json:"title"`
	} `json:"client"`
}

type pushResp struct {
	Results []pushResult `json:"results"`
}

func TestApplyPushTxLeavesCommitToCaller(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	book := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "book"})
	s := New(testPool)
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	opID := uuid.NewString()
	p := principal{UserID: user.UserID, DeviceID: user.DeviceID}
	results, _, err := s.applyPushTx(ctx, tx, p, user.DimensionID, book.ID, 0, []operationDTO{{
		OperationID: opID,
		Song:        songDTO{FBID: "song-tx", Title: "внутри", Parts: []partDTO{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "applied" || results[0].Revision != 1 || results[0].OperationID != opID {
		t.Fatalf("результат %+v", results)
	}
	var n int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM songs WHERE songbook_id = $1 AND fbid = 'song-tx'`, book.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("транзакция видна снаружи: %d", n)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := testPool.QueryRow(ctx, `SELECT title FROM songs WHERE songbook_id = $1 AND fbid = 'song-tx'`, book.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "внутри" {
		t.Fatalf("название %s", title)
	}
	results, err = s.applyPush(ctx, p, user.DimensionID, book.ID, 1, []operationDTO{{
		OperationID: uuid.NewString(),
		Song:        songDTO{FBID: "song-wrap", Title: "обёртка", Parts: []partDTO{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "applied" || results[0].Revision != 2 {
		t.Fatalf("обёртка %+v", results)
	}
	if err := testPool.QueryRow(ctx, `SELECT title FROM songs WHERE songbook_id = $1 AND fbid = 'song-wrap'`, book.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "обёртка" {
		t.Fatalf("обёртка не записала %s", title)
	}
}

func TestPushSuccess(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	book := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "book"})
	opID := uuid.NewString()
	code, raw := api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+book.ID+"/push", pushBody(0, songOp(opID, "song-1", "раз", []string{"song-1"})))
	if code != http.StatusOK {
		t.Fatalf("push %d %s", code, raw)
	}
	var resp pushResp
	mustJSON(t, raw, &resp)
	if len(resp.Results) != 1 || resp.Results[0].Status != "applied" || resp.Results[0].Revision != 1 {
		t.Fatalf("результат %+v", resp.Results)
	}
	ctx := context.Background()
	var seq int64
	var title string
	if err := testPool.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1`, user.DimensionID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if seq != 1 {
		t.Fatalf("seq %d", seq)
	}
	if err := testPool.QueryRow(ctx, `SELECT title FROM songs WHERE songbook_id = $1 AND fbid = 'song-1'`, book.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "раз" {
		t.Fatalf("название %s", title)
	}
	var order string
	if err := testPool.QueryRow(ctx, `SELECT fbids::text FROM song_order WHERE songbook_id = $1 AND revision = 1`, book.ID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	if order != `["song-1"]` {
		t.Fatalf("порядок %s", order)
	}
	code, raw = api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+book.ID+"/push", pushBody(0, songOp(opID, "song-1", "раз", []string{"song-1"})))
	if code != http.StatusOK {
		t.Fatalf("повтор %d %s", code, raw)
	}
	mustJSON(t, raw, &resp)
	if resp.Results[0].Status != "applied" || resp.Results[0].Revision != 1 {
		t.Fatalf("повтор изменил результат %+v", resp.Results[0])
	}
	if err := testPool.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1`, user.DimensionID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if seq != 1 {
		t.Fatalf("повтор сдвинул seq %d", seq)
	}
	code, raw = api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+book.ID+"/push", pushBody(0, songOp(opID, "song-1", "другое", []string{"song-1"})))
	if code != http.StatusConflict {
		t.Fatalf("другое тело %d %s", code, raw)
	}
	if err := testPool.QueryRow(ctx, `SELECT title FROM songs WHERE fbid = 'song-1'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1`, user.DimensionID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if title != "раз" || seq != 1 {
		t.Fatalf("другое тело изменило данные title=%s seq=%d", title, seq)
	}
}

func TestPushConflict(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	book := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "book"})
	first := uuid.NewString()
	code, raw := api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+book.ID+"/push", pushBody(0, songOp(first, "song-1", "сервер", nil)))
	if code != http.StatusOK {
		t.Fatalf("первый push %d %s", code, raw)
	}
	second := uuid.NewString()
	stale := pushBody(0, songOp(second, "song-1", "клиент", nil))
	code, raw = api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+book.ID+"/push", stale)
	if code != http.StatusOK {
		t.Fatalf("конфликт %d %s", code, raw)
	}
	var resp pushResp
	mustJSON(t, raw, &resp)
	if resp.Results[0].Status != "conflict" || resp.Results[0].Server == nil || resp.Results[0].Server.Title != "сервер" || resp.Results[0].Client.Title != "клиент" {
		t.Fatalf("тексты конфликта %+v", resp.Results[0])
	}
	ctx := context.Background()
	var seq int64
	var title string
	var conflicts int
	if err := testPool.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1`, user.DimensionID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT title FROM songs WHERE fbid = 'song-1'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM conflicts`).Scan(&conflicts); err != nil {
		t.Fatal(err)
	}
	if seq != 1 || title != "сервер" || conflicts != 1 {
		t.Fatalf("seq=%d title=%s conflicts=%d", seq, title, conflicts)
	}
	code, raw = api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks/"+book.ID+"/push", stale)
	if code != http.StatusOK {
		t.Fatalf("повтор конфликта %d %s", code, raw)
	}
	mustJSON(t, raw, &resp)
	if resp.Results[0].Status != "conflict" || resp.Results[0].Client.Title != "клиент" {
		t.Fatalf("повтор конфликта %+v", resp.Results[0])
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM conflicts`).Scan(&conflicts); err != nil {
		t.Fatal(err)
	}
	if conflicts != 1 {
		t.Fatalf("повтор записал ещё конфликт: %d", conflicts)
	}
}

func TestPushParallelSeq(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	book := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "book"})
	path := "/dimensions/" + user.DimensionID + "/songbooks/" + book.ID + "/push"
	var wg sync.WaitGroup
	start := make(chan struct{})
	type outcome struct {
		code int
		raw  []byte
		err  error
	}
	got := make([]outcome, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			op := songOp(uuid.NewString(), "song-"+string(rune('a'+i)), "текст", nil)
			code, raw, err := do(srv, user.Token, http.MethodPost, path, pushBody(0, op))
			got[i] = outcome{code, raw, err}
		}(i)
	}
	close(start)
	wg.Wait()
	applied, conflict := 0, 0
	for _, item := range got {
		if item.err != nil {
			t.Fatal(item.err)
		}
		if item.code != http.StatusOK {
			t.Fatalf("параллельный push %d %s", item.code, item.raw)
		}
		var resp pushResp
		mustJSON(t, item.raw, &resp)
		switch resp.Results[0].Status {
		case "applied":
			applied++
			if resp.Results[0].Revision != 1 {
				t.Fatalf("ревизия %d", resp.Results[0].Revision)
			}
		case "conflict":
			conflict++
		default:
			t.Fatalf("статус %s", resp.Results[0].Status)
		}
	}
	if applied != 1 || conflict != 1 {
		t.Fatalf("applied=%d conflict=%d", applied, conflict)
	}
	ctx := context.Background()
	var seq int64
	var rows int
	if err := testPool.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1`, user.DimensionID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM change_log WHERE dimension_id = $1`, user.DimensionID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if seq != 1 || rows != 1 {
		t.Fatalf("после гонки seq=%d rows=%d", seq, rows)
	}
	code, raw := api(t, srv, user.Token, http.MethodPost, path, pushBody(1, songOp(uuid.NewString(), "song-c", "дальше", nil)))
	if code != http.StatusOK {
		t.Fatalf("следующий push %d %s", code, raw)
	}
	var resp pushResp
	mustJSON(t, raw, &resp)
	if resp.Results[0].Status != "applied" || resp.Results[0].Revision != 2 {
		t.Fatalf("следующая ревизия %+v", resp.Results[0])
	}
	logSeq := []int64{}
	q, err := testPool.Query(ctx, `SELECT seq FROM change_log WHERE dimension_id = $1 ORDER BY seq`, user.DimensionID)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	for q.Next() {
		var n int64
		if err := q.Scan(&n); err != nil {
			t.Fatal(err)
		}
		logSeq = append(logSeq, n)
	}
	if len(logSeq) != 2 || logSeq[0] != 1 || logSeq[1] != 2 {
		t.Fatalf("пропуск seq %v", logSeq)
	}
}
