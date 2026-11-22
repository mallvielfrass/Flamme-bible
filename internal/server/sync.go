package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type partDTO struct {
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Align uint32 `json:"align"`
}

type songDTO struct {
	FBID       string    `json:"fbid"`
	Title      string    `json:"title"`
	AltTitle   string    `json:"alt_title"`
	Author     string    `json:"author"`
	Composer   string    `json:"composer"`
	Note       string    `json:"note"`
	Properties string    `json:"properties"`
	Parts      []partDTO `json:"parts"`
	Deleted    bool      `json:"deleted"`
}

type operationDTO struct {
	OperationID string    `json:"operation_id"`
	Song        songDTO   `json:"song"`
	Order       *[]string `json:"order,omitempty"`
}

type opResult struct {
	OperationID string          `json:"operation_id"`
	Status      string          `json:"status"`
	Revision    int64           `json:"revision,omitempty"`
	Server      json.RawMessage `json:"server,omitempty"`
	Client      json.RawMessage `json:"client,omitempty"`
}

func normalizeSong(song *songDTO) {
	if song.Parts == nil {
		song.Parts = []partDTO{}
	}
}

func hashOp(op operationDTO) []byte {
	normalizeSong(&op.Song)
	body, _ := json.Marshal(struct {
		Song  songDTO   `json:"song"`
		Order *[]string `json:"order,omitempty"`
	}{op.Song, op.Order})
	sum := sha256.Sum256(body)
	return sum[:]
}

func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	bookID := r.PathValue("bookID")
	if _, err := uuid.Parse(dimID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if _, err := uuid.Parse(bookID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var req struct {
		BaseRevision int64          `json:"base_revision"`
		Operations   []operationDTO `json:"operations"`
	}
	if err := readJSON(r, &req); err != nil || len(req.Operations) == 0 || len(req.Operations) > 100 || req.BaseRevision < 0 {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	seen := map[string]bool{}
	for i := range req.Operations {
		normalizeSong(&req.Operations[i].Song)
		if _, err := uuid.Parse(req.Operations[i].OperationID); err != nil || req.Operations[i].Song.FBID == "" || seen[req.Operations[i].OperationID] {
			writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
			return
		}
		seen[req.Operations[i].OperationID] = true
	}
	results, err := s.applyPush(r.Context(), p, dimID, bookID, req.BaseRevision, req.Operations)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

type preparedOp struct {
	op     operationDTO
	hash   []byte
	replay json.RawMessage
}

func (s *Server) applyPush(ctx context.Context, p principal, dimID, bookID string, base int64, ops []operationDTO) ([]opResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	results, prepared, err := s.applyPushTx(ctx, tx, p, dimID, bookID, base, ops)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.noteAppliedSongs(bookID, prepared, results)
	return results, nil
}

func (s *Server) applyPushTx(ctx context.Context, tx pgx.Tx, p principal, dimID, bookID string, base int64, ops []operationDTO) ([]opResult, []preparedOp, error) {
	if _, err := membershipTx(ctx, tx, p.UserID, dimID); err != nil {
		return nil, nil, err
	}
	var bookDim string
	err := tx.QueryRow(ctx, `SELECT dimension_id::text FROM songbooks WHERE id = $1`, bookID).Scan(&bookDim)
	if err == pgx.ErrNoRows || (err == nil && bookDim != dimID) {
		return nil, nil, errNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	// Номер выдаётся только под блокировкой счётчика этого пространства.
	var seq int64
	if err := tx.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1 FOR UPDATE`, dimID).Scan(&seq); err != nil {
		return nil, nil, err
	}
	prepared := make([]preparedOp, len(ops))
	hasNew := false
	for i, op := range ops {
		var got []byte
		var stored []byte
		err := tx.QueryRow(ctx, `
			SELECT body_hash, result
			FROM operation_results
			WHERE dimension_id = $1 AND device_id = $2 AND operation_id = $3
			FOR UPDATE
		`, dimID, p.DeviceID, op.OperationID).Scan(&got, &stored)
		hash := hashOp(op)
		if err == pgx.ErrNoRows {
			prepared[i] = preparedOp{op: op, hash: hash}
			hasNew = true
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(got, hash) {
			return nil, nil, errOpBody
		}
		prepared[i] = preparedOp{op: op, hash: hash, replay: stored}
	}
	conflict := hasNew && base != seq
	results := make([]opResult, len(prepared))
	next := seq
	for i, item := range prepared {
		if item.replay != nil {
			var prev opResult
			if err := json.Unmarshal(item.replay, &prev); err != nil {
				return nil, nil, err
			}
			results[i] = prev
			continue
		}
		if conflict {
			res, err := saveConflict(ctx, tx, dimID, bookID, p.DeviceID, item)
			if err != nil {
				return nil, nil, err
			}
			results[i] = res
			continue
		}
		next++
		res, err := saveApplied(ctx, tx, dimID, bookID, p.DeviceID, next, item)
		if err != nil {
			return nil, nil, err
		}
		results[i] = res
	}
	if next != seq {
		if _, err := tx.Exec(ctx, `UPDATE sync_counters SET seq = $2 WHERE dimension_id = $1`, dimID, next); err != nil {
			return nil, nil, err
		}
	}
	return results, prepared, nil
}

func saveApplied(ctx context.Context, tx pgx.Tx, dimID, bookID, deviceID string, seq int64, item preparedOp) (opResult, error) {
	if err := writeSong(ctx, tx, bookID, seq, item.op.Song); err != nil {
		return opResult{}, err
	}
	if item.op.Order != nil {
		if err := writeOrder(ctx, tx, bookID, seq, *item.op.Order); err != nil {
			return opResult{}, err
		}
	}
	body, err := json.Marshal(struct {
		Song  songDTO   `json:"song"`
		Order *[]string `json:"order,omitempty"`
	}{item.op.Song, item.op.Order})
	if err != nil {
		return opResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO change_log (dimension_id, seq, songbook_id, body) VALUES ($1, $2, $3, $4::jsonb)
	`, dimID, seq, bookID, string(body)); err != nil {
		return opResult{}, err
	}
	res := opResult{OperationID: item.op.OperationID, Status: "applied", Revision: seq}
	if err := storeResult(ctx, tx, dimID, deviceID, item, res); err != nil {
		return opResult{}, err
	}
	return res, nil
}

func saveConflict(ctx context.Context, tx pgx.Tx, dimID, bookID, deviceID string, item preparedOp) (opResult, error) {
	server, err := loadSong(ctx, tx, bookID, item.op.Song.FBID)
	if err != nil {
		return opResult{}, err
	}
	client, err := json.Marshal(item.op.Song)
	if err != nil {
		return opResult{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO conflicts (id, dimension_id, songbook_id, song_fbid, operation_id, server_body, client_body)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)
	`, uuid.NewString(), dimID, bookID, item.op.Song.FBID, item.op.OperationID, string(server), string(client))
	if err != nil {
		return opResult{}, err
	}
	res := opResult{OperationID: item.op.OperationID, Status: "conflict", Server: server, Client: client}
	if err := storeResult(ctx, tx, dimID, deviceID, item, res); err != nil {
		return opResult{}, err
	}
	return res, nil
}

func storeResult(ctx context.Context, tx pgx.Tx, dimID, deviceID string, item preparedOp, res opResult) error {
	raw, err := json.Marshal(res)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO operation_results (dimension_id, device_id, operation_id, body_hash, result)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`, dimID, deviceID, item.op.OperationID, item.hash, string(raw))
	return err
}

func writeSong(ctx context.Context, tx pgx.Tx, bookID string, seq int64, song songDTO) error {
	parts, err := json.Marshal(song.Parts)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO songs (
			songbook_id, fbid, title, alt_title, author, composer, note, properties, parts, revision, deleted
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)
		ON CONFLICT (songbook_id, fbid) DO UPDATE SET
			title = EXCLUDED.title,
			alt_title = EXCLUDED.alt_title,
			author = EXCLUDED.author,
			composer = EXCLUDED.composer,
			note = EXCLUDED.note,
			properties = EXCLUDED.properties,
			parts = EXCLUDED.parts,
			revision = EXCLUDED.revision,
			deleted = EXCLUDED.deleted
	`, bookID, song.FBID, song.Title, song.AltTitle, song.Author, song.Composer, song.Note, song.Properties, string(parts), seq, song.Deleted)
	return err
}

func writeOrder(ctx context.Context, tx pgx.Tx, bookID string, seq int64, order []string) error {
	raw, err := json.Marshal(order)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO song_order (songbook_id, revision, fbids) VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (songbook_id) DO UPDATE SET revision = EXCLUDED.revision, fbids = EXCLUDED.fbids
	`, bookID, seq, string(raw))
	return err
}

func loadSong(ctx context.Context, tx pgx.Tx, bookID, fbid string) (json.RawMessage, error) {
	var song songDTO
	var parts []byte
	err := tx.QueryRow(ctx, `
		SELECT fbid, title, alt_title, author, composer, note, properties, parts, deleted
		FROM songs WHERE songbook_id = $1 AND fbid = $2
	`, bookID, fbid).Scan(&song.FBID, &song.Title, &song.AltTitle, &song.Author, &song.Composer, &song.Note, &song.Properties, &parts, &song.Deleted)
	if err == pgx.ErrNoRows {
		return json.RawMessage("null"), nil
	}
	if err != nil {
		return nil, err
	}
	if len(parts) > 0 {
		if err := json.Unmarshal(parts, &song.Parts); err != nil {
			return nil, err
		}
	}
	normalizeSong(&song)
	return json.Marshal(song)
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	bookID := r.PathValue("bookID")
	if _, err := uuid.Parse(dimID); err != nil || uuid.Validate(bookID) != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := membershipTx(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	var bookDim string
	err = tx.QueryRow(ctx, `SELECT dimension_id::text FROM songbooks WHERE id = $1`, bookID).Scan(&bookDim)
	if err == pgx.ErrNoRows || (err == nil && bookDim != dimID) {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	var cursor int64
	if err := tx.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1 FOR SHARE`, dimID).Scan(&cursor); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := scanBook(tx.QueryRow(ctx, `SELECT `+bookCols+` FROM songbooks WHERE id = $1`, bookID))
	if err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := tx.Query(ctx, `
		SELECT fbid, title, alt_title, author, composer, note, properties, parts, deleted
		FROM songs WHERE songbook_id = $1 ORDER BY fbid
	`, bookID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	songs := []songDTO{}
	for rows.Next() {
		var song songDTO
		var parts []byte
		if err := rows.Scan(&song.FBID, &song.Title, &song.AltTitle, &song.Author, &song.Composer, &song.Note, &song.Properties, &parts, &song.Deleted); err != nil {
			rows.Close()
			writeMapped(w, err)
			return
		}
		if len(parts) > 0 {
			if err := json.Unmarshal(parts, &song.Parts); err != nil {
				rows.Close()
				writeMapped(w, err)
				return
			}
		}
		normalizeSong(&song)
		songs = append(songs, song)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeMapped(w, err)
		return
	}
	rows.Close()
	order := []string{}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT fbids FROM song_order WHERE songbook_id = $1`, bookID).Scan(&raw)
	if err == nil {
		if err := json.Unmarshal(raw, &order); err != nil {
			writeMapped(w, err)
			return
		}
	} else if !errors.Is(err, pgx.ErrNoRows) && err != pgx.ErrNoRows {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cursor":   cursor,
		"songbook": view,
		"songs":    songs,
		"order":    order,
	})
}

func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	dimID := r.PathValue("id")
	if _, err := uuid.Parse(dimID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	cursorText := r.URL.Query().Get("cursor")
	if cursorText == "" {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	cursor, err := strconv.ParseInt(cursorText, 10, 64)
	if err != nil || cursor < 0 {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
			return
		}
		limit = n
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := membershipTx(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	var head int64
	if err := tx.QueryRow(ctx, `SELECT seq FROM sync_counters WHERE dimension_id = $1 FOR SHARE`, dimID).Scan(&head); err != nil {
		writeMapped(w, err)
		return
	}
	if cursor > head {
		writeMapped(w, errReload)
		return
	}
	if cursor > 0 {
		var one int
		err := tx.QueryRow(ctx, `SELECT 1 FROM change_log WHERE dimension_id = $1 AND seq = $2`, dimID, cursor).Scan(&one)
		if err == pgx.ErrNoRows {
			writeMapped(w, errReload)
			return
		}
		if err != nil {
			writeMapped(w, err)
			return
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT seq, songbook_id::text, body
		FROM change_log
		WHERE dimension_id = $1 AND seq > $2
		ORDER BY seq
		LIMIT $3
	`, dimID, cursor, limit)
	if err != nil {
		writeMapped(w, err)
		return
	}
	events := []map[string]any{}
	for rows.Next() {
		var seq int64
		var book string
		var body []byte
		if err := rows.Scan(&seq, &book, &body); err != nil {
			rows.Close()
			writeMapped(w, err)
			return
		}
		events = append(events, map[string]any{"seq": seq, "songbook_id": book, "body": json.RawMessage(body)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeMapped(w, err)
		return
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
