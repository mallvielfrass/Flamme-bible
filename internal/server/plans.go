package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type planItem struct {
	ID                    string    `json:"id"`
	Kind                  string    `json:"kind"`
	SongID                string    `json:"song_id"`
	SongbookID            string    `json:"songbook_id"`
	Title                 string    `json:"title"`
	ModuleShortName       string    `json:"module_short_name"`
	ModuleShortNameSecond string    `json:"module_short_name_second"`
	Book                  int       `json:"book"`
	Chapter               int       `json:"chapter"`
	Verse                 int       `json:"verse"`
	Caption               string    `json:"caption"`
	Reference             string    `json:"reference"`
	Quote                 string    `json:"quote"`
	Class                 string    `json:"class"`
	Position              int       `json:"position"`
	CreatedAt             time.Time `json:"created_at,omitempty"`
	UpdatedAt             time.Time `json:"updated_at,omitempty"`
	SongbookTitle         string    `json:"songbook_title,omitempty"`
	SectionTitle          string    `json:"section_title,omitempty"`
}

type planView struct {
	ID          string     `json:"id"`
	OwnerUserID *string    `json:"owner_user_id"`
	DimensionID *string    `json:"dimension_id"`
	Title       string     `json:"title"`
	Version     int64      `json:"version"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Deleted     bool       `json:"deleted"`
	Items       []planItem `json:"items"`
}

type planEncodingNotice struct {
	Mojibake        bool       `json:"mojibake,omitempty"`
	EncodingWarning string     `json:"encoding_warning,omitempty"`
	SuggestedText   string     `json:"suggested_text,omitempty"`
	SuggestedTitle  string     `json:"suggested_title,omitempty"`
	SuggestedItems  []planItem `json:"suggested_items,omitempty"`
}

type planImportResult struct {
	planView
	planEncodingNotice
}

type planWrite struct {
	Title     string     `json:"title"`
	Version   int64      `json:"version"`
	UpdatedAt *time.Time `json:"updated_at"`
	Items     []planItem `json:"items"`
}

type planConflict struct {
	plan planView
}

func (e *planConflict) Error() string { return "конфликт" }

func (e *planConflict) Unwrap() error { return errConflict }

type planQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		Title       string     `json:"title"`
		DimensionID string     `json:"dimension_id"`
		Items       []planItem `json:"items"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	view, err := s.insertPlan(r.Context(), p.UserID, body.Title, body.DimensionID, body.Items)
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) importPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		Format      string `json:"format"`
		Payload     string `json:"payload"`
		Title       string `json:"title"`
		DimensionID string `json:"dimension_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	parsedTitle, items, err := parsePlanPayload(body.Format, []byte(body.Payload))
	if err != nil {
		writePlanErr(w, err)
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = strings.TrimSpace(parsedTitle)
	}
	if title == "" {
		title = "План"
	}
	view, err := s.insertPlan(r.Context(), p.UserID, title, body.DimensionID, items)
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, planImportResult{planView: view, planEncodingNotice: encodingNotice(body.Format, body.Payload)})
}

func (s *Server) parsePlan(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		Format  string `json:"format"`
		Payload string `json:"payload"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	title, items, err := parsePlanPayload(body.Format, []byte(body.Payload))
	if err != nil {
		writePlanErr(w, err)
		return
	}
	items, err = normalizeItems(items)
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":           strings.TrimSpace(title),
		"items":           items,
		"encoding_notice": encodingNotice(body.Format, body.Payload),
	})
}

func (s *Server) insertPlan(ctx context.Context, userID, title, dimensionID string, items []planItem) (planView, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 200 {
		return planView{}, errRequest
	}
	items, err := normalizeItems(items)
	if err != nil {
		return planView{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return planView{}, err
	}
	defer tx.Rollback(ctx)
	var owner any
	var dim any
	if dimensionID == "" {
		owner = userID
	} else {
		if _, err := uuid.Parse(dimensionID); err != nil {
			return planView{}, errNotFound
		}
		if _, err := membershipTx(ctx, tx, userID, dimensionID); err != nil {
			return planView{}, err
		}
		var kind string
		if err := tx.QueryRow(ctx, `SELECT kind FROM dimensions WHERE id = $1`, dimensionID).Scan(&kind); err != nil {
			if err == pgx.ErrNoRows {
				return planView{}, errNotFound
			}
			return planView{}, err
		}
		if kind != "community" {
			return planView{}, errRequest
		}
		dim = dimensionID
	}
	id := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO plans (id, owner_user_id, dimension_id, title, version, updated_at)
		VALUES ($1, $2, $3, $4, 1, now())
	`, id, owner, dim, title); err != nil {
		return planView{}, err
	}
	if err := replaceItems(ctx, tx, id, items); err != nil {
		return planView{}, err
	}
	view, err := loadPlan(ctx, tx, id)
	if err != nil {
		return planView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return planView{}, err
	}
	return view, nil
}

func (s *Server) listPlans(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "all"
	}
	dimID := r.URL.Query().Get("dimension_id")
	ctx := r.Context()
	q := `
		SELECT id::text FROM plans
		WHERE NOT deleted AND `
	var args []any
	switch {
	case dimID != "":
		if _, err := uuid.Parse(dimID); err != nil {
			writeMapped(w, errNotFound)
			return
		}
		if _, err := s.membership(ctx, p.UserID, dimID); err != nil {
			writeMapped(w, err)
			return
		}
		q += `dimension_id = $1`
		args = append(args, dimID)
	case scope == "personal":
		q += `owner_user_id = $1`
		args = append(args, p.UserID)
	case scope == "community":
		q += `dimension_id IN (SELECT dimension_id FROM memberships WHERE user_id = $1 AND NOT excluded)`
		args = append(args, p.UserID)
	case scope == "all":
		q += `(owner_user_id = $1 OR dimension_id IN (SELECT dimension_id FROM memberships WHERE user_id = $1 AND NOT excluded))`
		args = append(args, p.UserID)
	default:
		writeMapped(w, errRequest)
		return
	}
	q += ` ORDER BY updated_at DESC, id`
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			writeMapped(w, err)
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	list := []planView{}
	for _, id := range ids {
		view, err := loadPlan(ctx, s.db, id)
		if err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, view)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	view, err := s.readPlan(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) updatePlan(w http.ResponseWriter, r *http.Request) {
	s.writePlan(w, r)
}

func (s *Server) pushPlan(w http.ResponseWriter, r *http.Request) {
	s.writePlan(w, r)
}

func (s *Server) writePlan(w http.ResponseWriter, r *http.Request) {
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
	var body planWrite
	if err := readJSON(r, &body); err != nil || body.Version < 1 {
		writeMapped(w, errRequest)
		return
	}
	view, err := s.savePlan(r.Context(), p.UserID, id, body, false)
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) deletePlan(w http.ResponseWriter, r *http.Request) {
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
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		writeMapped(w, errRequest)
		return
	}
	var updated *time.Time
	if raw := r.URL.Query().Get("updated_at"); raw != "" {
		t, err := parseTime(raw)
		if err != nil {
			writeMapped(w, errRequest)
			return
		}
		updated = &t
	}
	view, err := s.savePlan(r.Context(), p.UserID, id, planWrite{Version: version, UpdatedAt: updated}, true)
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) savePlan(ctx context.Context, userID, id string, body planWrite, remove bool) (planView, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return planView{}, err
	}
	defer tx.Rollback(ctx)
	var locked string
	err = tx.QueryRow(ctx, `SELECT id::text FROM plans WHERE id = $1 FOR UPDATE`, id).Scan(&locked)
	if err == pgx.ErrNoRows {
		return planView{}, errNotFound
	}
	if err != nil {
		return planView{}, err
	}
	view, err := loadPlan(ctx, tx, id)
	if err != nil {
		return planView{}, err
	}
	if err := planAccess(ctx, tx, userID, view); err != nil {
		return planView{}, err
	}
	if view.Version != body.Version || (body.UpdatedAt != nil && !sameInstant(view.UpdatedAt, *body.UpdatedAt)) {
		return planView{}, &planConflict{plan: view}
	}
	if view.Deleted {
		if remove {
			if err := tx.Commit(ctx); err != nil {
				return planView{}, err
			}
			return view, nil
		}
		return planView{}, &planConflict{plan: view}
	}
	if remove {
		if _, err := tx.Exec(ctx, `
			UPDATE plans SET deleted = true, version = version + 1, updated_at = now() WHERE id = $1
		`, id); err != nil {
			return planView{}, err
		}
	} else {
		items, err := normalizeItems(body.Items)
		if err != nil {
			return planView{}, err
		}
		title := strings.TrimSpace(body.Title)
		if title == "" {
			title = view.Title
		}
		if utf8.RuneCountInString(title) > 200 {
			return planView{}, errRequest
		}
		if _, err := tx.Exec(ctx, `
			UPDATE plans SET title = $2, version = version + 1, updated_at = now() WHERE id = $1
		`, id, title); err != nil {
			return planView{}, err
		}
		if err := replaceItems(ctx, tx, id, items); err != nil {
			return planView{}, err
		}
	}
	view, err = loadPlan(ctx, tx, id)
	if err != nil {
		return planView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return planView{}, err
	}
	return view, nil
}

func (s *Server) pullPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	view, err := s.readPlan(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writePlanErr(w, err)
		return
	}
	changed := true
	if raw := r.URL.Query().Get("version"); raw != "" {
		version, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || version < 1 {
			writeMapped(w, errRequest)
			return
		}
		changed = version != view.Version
		if rawTime := r.URL.Query().Get("updated_at"); rawTime != "" {
			got, err := parseTime(rawTime)
			if err != nil {
				writeMapped(w, errRequest)
				return
			}
			if !sameInstant(got, view.UpdatedAt) {
				changed = true
			}
		}
	}
	if !changed {
		writeJSON(w, http.StatusOK, map[string]any{
			"changed":    false,
			"version":    view.Version,
			"updated_at": view.UpdatedAt,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": true, "plan": view})
}

func (s *Server) exportPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	view, err := s.readPlan(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writePlanErr(w, err)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" || strings.EqualFold(format, "json") {
		writeJSON(w, http.StatusOK, view)
		return
	}
	payload, ctype, err := formatPlan(format, r.URL.Query().Get("encoding"), view)
	if err != nil {
		writePlanErr(w, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(payload))
}

func (s *Server) readPlan(ctx context.Context, userID, id string) (planView, error) {
	if _, err := uuid.Parse(id); err != nil {
		return planView{}, errNotFound
	}
	view, err := loadPlan(ctx, s.db, id)
	if err != nil {
		return planView{}, err
	}
	if err := planAccess(ctx, s.db, userID, view); err != nil {
		return planView{}, err
	}
	return view, nil
}

func planAccess(ctx context.Context, q planQuerier, userID string, view planView) error {
	if view.OwnerUserID != nil {
		if *view.OwnerUserID == userID {
			return nil
		}
		return errNotFound
	}
	if view.DimensionID == nil {
		return errNotFound
	}
	_, err := scanMembership(q.QueryRow(ctx, `
		SELECT role, excluded FROM memberships WHERE dimension_id = $1 AND user_id = $2
	`, *view.DimensionID, userID))
	return err
}

func loadPlan(ctx context.Context, q planQuerier, id string) (planView, error) {
	var view planView
	err := q.QueryRow(ctx, `
		SELECT id::text, owner_user_id::text, dimension_id::text, title, version, updated_at, deleted
		FROM plans WHERE id = $1
	`, id).Scan(&view.ID, &view.OwnerUserID, &view.DimensionID, &view.Title, &view.Version, &view.UpdatedAt, &view.Deleted)
	if err == pgx.ErrNoRows {
		return planView{}, errNotFound
	}
	if err != nil {
		return planView{}, err
	}
	view.UpdatedAt = view.UpdatedAt.UTC()
	rows, err := q.Query(ctx, `
		SELECT pi.id::text, pi.position, pi.kind, pi.song_id, pi.songbook_id::text, pi.title,
		       pi.module_short_name, pi.module_short_name_second, pi.book, pi.chapter, pi.verse,
		       pi.caption, pi.reference, pi.quote, pi.class, pi.created_at, pi.updated_at,
		       COALESCE(b.title, ''),
		       COALESCE((
		         SELECT sec.title
		         FROM songbook_section_songs ss
		         JOIN songbook_sections sec ON sec.id = ss.section_id
		         WHERE pi.songbook_id IS NOT NULL
		           AND ss.songbook_id = pi.songbook_id AND ss.fbid = pi.song_id
		         ORDER BY sec.position, sec.title, sec.id
		         LIMIT 1
		       ), '')
		FROM plan_items pi
		LEFT JOIN songbooks b ON b.id = pi.songbook_id
		WHERE pi.plan_id = $1
		ORDER BY pi.position, pi.id
	`, id)
	if err != nil {
		return planView{}, err
	}
	defer rows.Close()
	view.Items = []planItem{}
	for rows.Next() {
		var item planItem
		var songbook *string
		if err := rows.Scan(
			&item.ID, &item.Position, &item.Kind, &item.SongID, &songbook, &item.Title,
			&item.ModuleShortName, &item.ModuleShortNameSecond, &item.Book, &item.Chapter, &item.Verse,
			&item.Caption, &item.Reference, &item.Quote, &item.Class, &item.CreatedAt, &item.UpdatedAt,
			&item.SongbookTitle, &item.SectionTitle,
		); err != nil {
			return planView{}, err
		}
		item.CreatedAt = item.CreatedAt.UTC()
		item.UpdatedAt = item.UpdatedAt.UTC()
		if songbook != nil {
			item.SongbookID = *songbook
		}
		view.Items = append(view.Items, item)
	}
	if err := rows.Err(); err != nil {
		return planView{}, err
	}
	return view, nil
}

type keptItem struct {
	created time.Time
	updated time.Time
	body    string
}

func itemBody(item planItem) string {
	return strings.Join([]string{
		item.Kind,
		item.SongID,
		item.SongbookID,
		item.Title,
		item.ModuleShortName,
		item.ModuleShortNameSecond,
		strconv.Itoa(item.Book),
		strconv.Itoa(item.Chapter),
		strconv.Itoa(item.Verse),
		item.Caption,
		item.Reference,
		item.Quote,
		item.Class,
	}, "\x1f")
}

func replaceItems(ctx context.Context, tx pgx.Tx, planID string, items []planItem) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text, created_at, updated_at, kind, song_id, COALESCE(songbook_id::text, ''), title,
		       module_short_name, module_short_name_second, book, chapter, verse,
		       caption, reference, quote, class
		FROM plan_items
		WHERE plan_id = $1
	`, planID)
	if err != nil {
		return err
	}
	old := map[string]keptItem{}
	for rows.Next() {
		var item planItem
		var created, updated time.Time
		if err := rows.Scan(
			&item.ID, &created, &updated, &item.Kind, &item.SongID, &item.SongbookID, &item.Title,
			&item.ModuleShortName, &item.ModuleShortNameSecond, &item.Book, &item.Chapter, &item.Verse,
			&item.Caption, &item.Reference, &item.Quote, &item.Class,
		); err != nil {
			rows.Close()
			return err
		}
		old[item.ID] = keptItem{created: created.UTC(), updated: updated.UTC(), body: itemBody(item)}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err := tx.Exec(ctx, `DELETE FROM plan_items WHERE plan_id = $1`, planID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, item := range items {
		created, updated := now, now
		if prev, ok := old[item.ID]; ok {
			created = prev.created
			if prev.body == itemBody(item) {
				updated = prev.updated
			}
		}
		var songbook any
		if item.SongbookID != "" {
			songbook = item.SongbookID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO plan_items (
				id, plan_id, position, kind, song_id, songbook_id, title,
				module_short_name, module_short_name_second, book, chapter, verse,
				caption, reference, quote, class, created_at, updated_at
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,
				$8,$9,$10,$11,$12,
				$13,$14,$15,$16,$17,$18
			)
		`, item.ID, planID, item.Position, item.Kind, item.SongID, songbook, item.Title,
			item.ModuleShortName, item.ModuleShortNameSecond, item.Book, item.Chapter, item.Verse,
			item.Caption, item.Reference, item.Quote, item.Class, created, updated); err != nil {
			return err
		}
	}
	return nil
}

func normalizeItems(items []planItem) ([]planItem, error) {
	if len(items) > 500 {
		return nil, errRequest
	}
	out := make([]planItem, 0, len(items))
	seen := map[string]bool{}
	for i, item := range items {
		item.Kind = strings.TrimSpace(item.Kind)
		item.SongID = strings.TrimSpace(item.SongID)
		item.SongbookID = strings.TrimSpace(item.SongbookID)
		item.Title = strings.TrimSpace(item.Title)
		item.ModuleShortName = strings.TrimSpace(item.ModuleShortName)
		item.ModuleShortNameSecond = strings.TrimSpace(item.ModuleShortNameSecond)
		item.Caption = strings.TrimSpace(item.Caption)
		item.Reference = strings.TrimSpace(item.Reference)
		item.Quote = strings.TrimSpace(item.Quote)
		item.Class = strings.TrimSpace(item.Class)
		switch item.Kind {
		case "song":
			if item.SongID == "" && item.Title == "" {
				return nil, errRequest
			}
			if item.SongbookID != "" && !validUUID(item.SongbookID) {
				return nil, errRequest
			}
		case "bible":
			if !validModuleName(item.ModuleShortName) {
				return nil, errRequest
			}
			if item.ModuleShortNameSecond != "" && !validModuleName(item.ModuleShortNameSecond) {
				return nil, errRequest
			}
			if item.Book < 0 || item.Chapter < 0 || item.Verse < 0 || item.Book > 200 || item.Chapter > 200 || item.Verse > 500 {
				return nil, errRequest
			}
			if item.Book == 0 && item.Reference == "" {
				return nil, errRequest
			}
			if item.Class == "" {
				item.Class = "-1"
			}
		case "text":
			if item.Caption == "" && item.Quote == "" {
				return nil, errRequest
			}
			if item.Class == "" {
				item.Class = "-1"
			}
		default:
			return nil, errRequest
		}
		if textTooLong(item) {
			return nil, errRequest
		}
		if item.ID == "" {
			item.ID = uuid.NewString()
		} else if !validUUID(item.ID) || seen[item.ID] {
			return nil, errRequest
		}
		seen[item.ID] = true
		item.Position = i
		out = append(out, item)
	}
	return out, nil
}

func textTooLong(item planItem) bool {
	return utf8.RuneCountInString(item.Title) > 300 ||
		utf8.RuneCountInString(item.Caption) > 300 ||
		utf8.RuneCountInString(item.Reference) > 300 ||
		utf8.RuneCountInString(item.Quote) > 4000 ||
		utf8.RuneCountInString(item.Class) > 32 ||
		utf8.RuneCountInString(item.SongID) > 80
}

func sameInstant(a, b time.Time) bool {
	return a.UTC().Truncate(time.Microsecond).Equal(b.UTC().Truncate(time.Microsecond))
}

func parseTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func writePlanErr(w http.ResponseWriter, err error) {
	var conflict *planConflict
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "конфликт", "plan": conflict.plan})
		return
	}
	writeMapped(w, err)
}
