package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxChain = 8

type headerTemplate struct {
	FormatVersion string `json:"format_version"`
	Flags         uint32 `json:"flags"`
	Charset       string `json:"charset"`
	HeaderShort   string `json:"header_short"`
	HeaderTitle   string `json:"header_title"`
}

type songbookView struct {
	ID               string         `json:"id"`
	DimensionID      string         `json:"dimension_id"`
	FBID             string         `json:"fbid"`
	Title            string         `json:"title"`
	ShortName        string         `json:"short_name"`
	Publisher        string         `json:"publisher"`
	RevisionDate     string         `json:"revision_date"`
	Comment          string         `json:"comment"`
	Header           headerTemplate `json:"header"`
	ParentID         *string        `json:"parent_id"`
	Hidden           bool           `json:"hidden"`
	PublishRequested bool           `json:"publish_requested"`
	Public           bool           `json:"public"`
	Status           string         `json:"status"`
	ModerationNote   string         `json:"moderation_note"`
	ReviewedBy       *string        `json:"reviewed_by"`
	ReviewedAt       *time.Time     `json:"reviewed_at"`
	IndexPending     int            `json:"index_pending"`
	IndexFailed      int            `json:"index_failed"`
}

func scanBook(row pgx.Row) (songbookView, error) {
	var view songbookView
	var header []byte
	var parent *string
	err := row.Scan(
		&view.ID, &view.DimensionID, &view.FBID, &view.Title, &view.ShortName,
		&view.Publisher, &view.RevisionDate, &view.Comment, &header, &parent, &view.Hidden,
		&view.PublishRequested, &view.Public, &view.Status, &view.ModerationNote, &view.ReviewedBy, &view.ReviewedAt,
	)
	if err != nil {
		return songbookView{}, err
	}
	if err := json.Unmarshal(header, &view.Header); err != nil {
		return songbookView{}, err
	}
	view.ParentID = parent
	return view, nil
}

const bookCols = `id::text, dimension_id::text, fbid, title, short_name, publisher, revision_date, comment, header_template, parent_id::text, hidden, publish_requested, public, status, moderation_note, reviewed_by::text, reviewed_at`

func (s *Server) createSongbook(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		ID           string         `json:"id"`
		FBID         string         `json:"fbid"`
		Title        string         `json:"title"`
		ShortName    string         `json:"short_name"`
		Publisher    string         `json:"publisher"`
		RevisionDate string         `json:"revision_date"`
		Comment      string         `json:"comment"`
		Header       headerTemplate `json:"header"`
		ParentID     string         `json:"parent_id"`
	}
	if err := readJSON(r, &body); err != nil || body.FBID == "" || body.Header.FormatVersion == "" || body.Header.Charset == "" {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	id := body.ID
	if id == "" {
		id = uuid.NewString()
	} else if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	if body.ParentID != "" {
		if _, err := uuid.Parse(body.ParentID); err != nil {
			writeMapped(w, errNotFound)
			return
		}
	}
	header, err := json.Marshal(body.Header)
	if err != nil {
		writeMapped(w, err)
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
	if body.ParentID == id {
		writeMapped(w, errCycle)
		return
	}
	if body.ParentID != "" {
		if err := lockBook(ctx, tx, body.ParentID); err != nil {
			writeMapped(w, err)
			return
		}
	}
	if err := parentOK(ctx, tx, id, body.ParentID); err != nil {
		writeMapped(w, err)
		return
	}
	var parent any
	if body.ParentID != "" {
		parent = body.ParentID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO songbooks (
			id, dimension_id, fbid, title, short_name, publisher, revision_date, comment, header_template, parent_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)
	`, id, dimID, body.FBID, body.Title, body.ShortName, body.Publisher, body.RevisionDate, body.Comment, string(header), parent)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, errJSON("сборник"))
			return
		}
		writeMapped(w, err)
		return
	}
	view, err := scanBook(tx.QueryRow(ctx, `SELECT `+bookCols+` FROM songbooks WHERE id = $1`, id))
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) listSongbooks(w http.ResponseWriter, r *http.Request) {
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
	if _, err := s.membership(r.Context(), p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT `+bookCols+` FROM songbooks WHERE dimension_id = $1 AND hidden_at IS NULL ORDER BY id`, dimID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []songbookView{}
	for rows.Next() {
		view, err := scanBook(rows)
		if err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, view)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].ID
	}
	counts, err := s.songIndexCounts(r.Context(), ids)
	if err != nil {
		writeMapped(w, err)
		return
	}
	for i := range list {
		if c, ok := counts[list[i].ID]; ok {
			list[i].IndexPending = c.Pending
			list[i].IndexFailed = c.Failed
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) hideSongbook(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
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
	var dimID string
	err = tx.QueryRow(ctx, `SELECT dimension_id::text FROM songbooks WHERE id = $1 FOR UPDATE`, id).Scan(&dimID)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := membershipTx(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE songbooks SET hidden = true WHERE id = $1`, id); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := scanBook(tx.QueryRow(ctx, `SELECT `+bookCols+` FROM songbooks WHERE id = $1`, id))
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) setParent(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
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
		ParentID string `json:"parent_id"`
	}
	if err := readJSON(r, &body); err != nil || body.ParentID == "" {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	if _, err := uuid.Parse(body.ParentID); err != nil {
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
	if err := lockBooks(ctx, tx, []string{id, body.ParentID}); err != nil {
		writeMapped(w, err)
		return
	}
	var dimID string
	if err := tx.QueryRow(ctx, `SELECT dimension_id::text FROM songbooks WHERE id = $1`, id).Scan(&dimID); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := membershipTx(ctx, tx, p.UserID, dimID); err != nil {
		writeMapped(w, err)
		return
	}
	if err := parentOK(ctx, tx, id, body.ParentID); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE songbooks SET parent_id = $2 WHERE id = $1`, id, body.ParentID); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := scanBook(tx.QueryRow(ctx, `SELECT `+bookCols+` FROM songbooks WHERE id = $1`, id))
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func lockBook(ctx context.Context, tx pgx.Tx, id string) error {
	var got string
	err := tx.QueryRow(ctx, `SELECT id::text FROM songbooks WHERE id = $1 FOR UPDATE`, id).Scan(&got)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	return err
}

func lockBooks(ctx context.Context, tx pgx.Tx, ids []string) error {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	prev := ""
	for _, id := range sorted {
		if id == prev {
			continue
		}
		prev = id
		if err := lockBook(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func parentOK(ctx context.Context, tx pgx.Tx, bookID, parentID string) error {
	if parentID == "" {
		return nil
	}
	if parentID == bookID {
		return errCycle
	}
	under, err := reaches(ctx, tx, bookID, parentID)
	if err != nil {
		return err
	}
	if under {
		return errCycle
	}
	up, err := chainLen(ctx, tx, parentID)
	if err != nil {
		return err
	}
	below, err := nodeHeight(ctx, tx, bookID)
	if err != nil {
		return err
	}
	if up+below > maxChain {
		return errDepth
	}
	return nil
}

func chainLen(ctx context.Context, tx pgx.Tx, start string) (int, error) {
	seen := map[string]bool{}
	cur := start
	n := 0
	for {
		if seen[cur] {
			return 0, errCycle
		}
		seen[cur] = true
		n++
		if n > maxChain {
			return 0, errDepth
		}
		var parent *string
		err := tx.QueryRow(ctx, `SELECT parent_id::text FROM songbooks WHERE id = $1`, cur).Scan(&parent)
		if err == pgx.ErrNoRows {
			return 0, errNotFound
		}
		if err != nil {
			return 0, err
		}
		if parent == nil || *parent == "" {
			return n, nil
		}
		cur = *parent
	}
}

func reaches(ctx context.Context, tx pgx.Tx, root, target string) (bool, error) {
	if root == "" || target == "" {
		return false, nil
	}
	seen := map[string]bool{}
	queue := []string{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		rows, err := tx.Query(ctx, `SELECT id::text FROM songbooks WHERE parent_id = $1`, cur)
		if err != nil {
			return false, err
		}
		var next []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return false, err
			}
			if id == target {
				rows.Close()
				return true, nil
			}
			if !seen[id] {
				next = append(next, id)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return false, err
		}
		rows.Close()
		queue = append(queue, next...)
	}
	return false, nil
}

func nodeHeight(ctx context.Context, tx pgx.Tx, id string) (int, error) {
	var present int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM songbooks WHERE id = $1`, id).Scan(&present); err != nil {
		return 0, err
	}
	if present == 0 {
		return 1, nil
	}
	return heightFrom(ctx, tx, id, map[string]bool{})
}

func heightFrom(ctx context.Context, tx pgx.Tx, id string, seen map[string]bool) (int, error) {
	if seen[id] {
		return 0, errCycle
	}
	seen[id] = true
	rows, err := tx.Query(ctx, `SELECT id::text FROM songbooks WHERE parent_id = $1`, id)
	if err != nil {
		return 0, err
	}
	var children []string
	for rows.Next() {
		var child string
		if err := rows.Scan(&child); err != nil {
			rows.Close()
			return 0, err
		}
		children = append(children, child)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	maxChild := 0
	for _, child := range children {
		h, err := heightFrom(ctx, tx, child, seen)
		if err != nil {
			return 0, err
		}
		if h > maxChild {
			maxChild = h
		}
	}
	return maxChild + 1, nil
}
