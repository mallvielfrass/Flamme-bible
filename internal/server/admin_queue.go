package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) writeSubmissions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	kind := r.URL.Query().Get("kind")
	if !validModerationStatus(status) || (kind != "" && kind != "songbook" && kind != "module") {
		writeMapped(w, errRequest)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 200 {
		writeMapped(w, errRequest)
		return
	}
	limit, offset, err := pageWindow(r, 50, 200)
	if err != nil {
		writeMapped(w, err)
		return
	}
	ctx := r.Context()
	items, err := s.loadSubmissions(ctx, q)
	if err != nil {
		writeMapped(w, err)
		return
	}
	counts := submissionCounts(items, status, kind)
	filtered := filterSubmissions(items, status, kind)
	sort.Slice(filtered, func(i, j int) bool {
		return submissionBefore(filtered[i], filtered[j])
	})
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[offset:end]
	if page == nil {
		page = []submission{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"submissions": page,
		"total":       total,
		"counts":      counts,
	})
}

func (s *Server) loadSubmissions(ctx context.Context, q string) ([]submission, error) {
	books, err := s.querySongbookSubmissions(ctx, q)
	if err != nil {
		return nil, err
	}
	modules, err := s.queryModuleSubmissions(ctx, q)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(books))
	for i := range books {
		ids[i] = books[i].ID
	}
	counts, err := s.songIndexCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range books {
		if c, ok := counts[books[i].ID]; ok {
			books[i].IndexPending = c.Pending
			books[i].IndexFailed = c.Failed
		}
	}
	return append(books, modules...), nil
}

func (s *Server) querySongbookSubmissions(ctx context.Context, q string) ([]submission, error) {
	sql := `
		SELECT s.id::text, s.title, s.short_name, s.status, s.moderation_note, s.dimension_id::text,
		       s.publisher, s.submitted_at, s.hidden_at,
		       CASE WHEN d.kind = 'personal' THEN 'Личное' ELSE d.name END,
		       COALESCE((
		         SELECT u.email FROM memberships m
		         JOIN users u ON u.id = m.user_id
		         WHERE m.dimension_id = s.dimension_id AND m.role = 'admin' AND NOT m.excluded
		         ORDER BY u.email LIMIT 1
		       ), ''),
		       (SELECT count(*) FROM songs sg WHERE sg.songbook_id = s.id AND NOT sg.deleted),
		       (SELECT count(*) FROM songbooks child WHERE child.parent_id = s.id)
		FROM songbooks s
		JOIN dimensions d ON d.id = s.dimension_id
		WHERE ($1 = '' OR s.title ILIKE $1 OR s.short_name ILIKE $1 OR s.publisher ILIKE $1
		   OR EXISTS (
		        SELECT 1 FROM memberships m
		        JOIN users u ON u.id = m.user_id
		        WHERE m.dimension_id = s.dimension_id AND u.email ILIKE $1
		   ))`
	rows, err := s.db.Query(ctx, sql, likeOrEmpty(q))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []submission{}
	for rows.Next() {
		var item submission
		item.Kind = "songbook"
		if err := rows.Scan(
			&item.ID, &item.Title, &item.ShortName, &item.Status, &item.ModerationNote, &item.DimensionID,
			&item.Publisher, &item.SubmittedAt, &item.HiddenAt, &item.CommunityName, &item.AuthorEmail, &item.SongsCount, &item.PatchCount,
		); err != nil {
			return nil, err
		}
		if item.SubmittedAt != nil {
			t := item.SubmittedAt.UTC()
			item.SubmittedAt = &t
		}
		if item.HiddenAt != nil {
			t := item.HiddenAt.UTC()
			item.HiddenAt = &t
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *Server) queryModuleSubmissions(ctx context.Context, q string) ([]submission, error) {
	sql := `
		SELECT m.id::text, m.title, m.short_name, m.status, m.moderation_note, m.dimension_id::text, m.owner_user_id::text,
		       m.submitted_at,
		       CASE WHEN d.kind = 'personal' THEN 'Личное' WHEN d.id IS NULL THEN '' ELSE d.name END,
		       COALESCE(owner.email, (
		         SELECT u.email FROM memberships ms
		         JOIN users u ON u.id = ms.user_id
		         WHERE ms.dimension_id = m.dimension_id AND ms.role = 'admin' AND NOT ms.excluded
		         ORDER BY u.email LIMIT 1
		       ), ''),
		       (SELECT count(DISTINCT book) FROM module_verses v WHERE v.module_id = m.id)
		FROM modules m
		LEFT JOIN dimensions d ON d.id = m.dimension_id
		LEFT JOIN users owner ON owner.id = m.owner_user_id
		WHERE ($1 = '' OR m.title ILIKE $1 OR m.short_name ILIKE $1 OR owner.email ILIKE $1)`
	rows, err := s.db.Query(ctx, sql, likeOrEmpty(q))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []submission{}
	for rows.Next() {
		var item submission
		item.Kind = "module"
		if err := rows.Scan(
			&item.ID, &item.Title, &item.ShortName, &item.Status, &item.ModerationNote, &item.DimensionID, &item.OwnerUserID,
			&item.SubmittedAt, &item.CommunityName, &item.AuthorEmail, &item.BooksCount,
		); err != nil {
			return nil, err
		}
		if item.SubmittedAt != nil {
			t := item.SubmittedAt.UTC()
			item.SubmittedAt = &t
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func likeOrEmpty(q string) string {
	if strings.TrimSpace(q) == "" {
		return ""
	}
	return likeContains(q)
}

func hiddenRow(item submission) bool {
	return item.HiddenAt != nil
}

func filterSubmissions(items []submission, status, kind string) []submission {
	out := []submission{}
	for _, item := range items {
		if kind != "" && item.Kind != kind {
			continue
		}
		if status == "hidden" {
			if hiddenRow(item) {
				out = append(out, item)
			}
			continue
		}
		if hiddenRow(item) {
			continue
		}
		if status != "all" && item.Status != status {
			continue
		}
		out = append(out, item)
	}
	return out
}

func submissionCounts(items []submission, status, kind string) map[string]any {
	kindCounts := map[string]int{"all": 0, "songbook": 0, "module": 0}
	for _, item := range filterSubmissions(items, status, "") {
		kindCounts["all"]++
		kindCounts[item.Kind]++
	}
	statusCounts := map[string]int{"pending": 0, "approved": 0, "rejected": 0, "draft": 0, "hidden": 0, "all": 0}
	for _, item := range items {
		if kind != "" && item.Kind != kind {
			continue
		}
		if hiddenRow(item) {
			statusCounts["hidden"]++
			continue
		}
		statusCounts[item.Status]++
		statusCounts["all"]++
	}
	return map[string]any{"kind": kindCounts, "status": statusCounts}
}

func submissionBefore(a, b submission) bool {
	if a.SubmittedAt == nil && b.SubmittedAt != nil {
		return false
	}
	if a.SubmittedAt != nil && b.SubmittedAt == nil {
		return true
	}
	if a.SubmittedAt != nil && b.SubmittedAt != nil && !a.SubmittedAt.Equal(*b.SubmittedAt) {
		return a.SubmittedAt.Before(*b.SubmittedAt)
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	return a.ID < b.ID
}

func pageWindow(r *http.Request, def, max int) (int, int, error) {
	limit := def
	offset := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > max {
			return 0, 0, errRequest
		}
		limit = n
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 100000 {
			return 0, 0, errRequest
		}
		offset = n
	}
	return limit, offset, nil
}

func (s *Server) adminSummary(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireModerator(w, r); !ok {
		return
	}
	ctx := r.Context()
	var songbooks, modules, drafts int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM songbooks WHERE status = 'pending' AND hidden_at IS NULL`).Scan(&songbooks); err != nil {
		writeMapped(w, err)
		return
	}
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM modules WHERE status = 'pending'`).Scan(&modules); err != nil {
		writeMapped(w, err)
		return
	}
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM client_releases WHERE status = 'dev'`).Scan(&drafts); err != nil {
		writeMapped(w, err)
		return
	}
	active, err := activeVersion(ctx, s.db)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pending_songbooks": songbooks,
		"pending_modules":   modules,
		"draft_releases":    drafts,
		"active_version":    active,
	})
}

func (s *Server) adminSongbookView(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireModerator(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	var view struct {
		ID             string        `json:"id"`
		Title          string        `json:"title"`
		ShortName      string        `json:"short_name"`
		Publisher      string        `json:"publisher"`
		Status         string        `json:"status"`
		ModerationNote string        `json:"moderation_note"`
		CommunityName  string        `json:"community_name"`
		AuthorEmail    string        `json:"author_email"`
		SubmittedAt    *time.Time    `json:"submitted_at"`
		SongsCount     int           `json:"songs_count"`
		PatchCount     int           `json:"patch_count"`
		Songs          []songPreview `json:"songs"`
		Similar        []similarHit  `json:"similar"`
		Audit          []auditEntry  `json:"audit"`
	}
	err := s.db.QueryRow(ctx, `
		SELECT s.id::text, s.title, s.short_name, s.publisher, s.status, s.moderation_note, s.submitted_at,
		       CASE WHEN d.kind = 'personal' THEN 'Личное' ELSE d.name END,
		       COALESCE((
		         SELECT u.email FROM memberships m
		         JOIN users u ON u.id = m.user_id
		         WHERE m.dimension_id = s.dimension_id AND m.role = 'admin' AND NOT m.excluded
		         ORDER BY u.email LIMIT 1
		       ), ''),
		       (SELECT count(*) FROM songs sg WHERE sg.songbook_id = s.id AND NOT sg.deleted),
		       (SELECT count(*) FROM songbooks child WHERE child.parent_id = s.id)
		FROM songbooks s
		JOIN dimensions d ON d.id = s.dimension_id
		WHERE s.id = $1
	`, id).Scan(
		&view.ID, &view.Title, &view.ShortName, &view.Publisher, &view.Status, &view.ModerationNote, &view.SubmittedAt,
		&view.CommunityName, &view.AuthorEmail, &view.SongsCount, &view.PatchCount,
	)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if view.SubmittedAt != nil {
		t := view.SubmittedAt.UTC()
		view.SubmittedAt = &t
	}
	songs, err := s.previewSongs(ctx, id, 8)
	if err != nil {
		writeMapped(w, err)
		return
	}
	view.Songs = songs
	similar, err := s.similarInCatalog(ctx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	view.Similar = similar
	audit, err := s.recentAudit(ctx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	view.Audit = audit
	writeJSON(w, http.StatusOK, view)
}

type songPreview struct {
	FBID  string `json:"fbid"`
	Title string `json:"title"`
}

type similarHit struct {
	SongbookID    string `json:"songbook_id"`
	SongbookTitle string `json:"songbook_title"`
	FBID          string `json:"fbid"`
	Title         string `json:"title"`
	Match         string `json:"match"`
}

func (s *Server) previewSongs(ctx context.Context, bookID string, limit int) ([]songPreview, error) {
	rows, err := s.db.Query(ctx, `
		SELECT fbid, title FROM songs
		WHERE songbook_id = $1 AND NOT deleted
		ORDER BY title, fbid
		LIMIT $2
	`, bookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []songPreview{}
	for rows.Next() {
		var song songPreview
		if err := rows.Scan(&song.FBID, &song.Title); err != nil {
			return nil, err
		}
		list = append(list, song)
	}
	return list, rows.Err()
}

func (s *Server) similarInCatalog(ctx context.Context, bookID string) ([]similarHit, error) {
	rows, err := s.db.Query(ctx, `
		SELECT fbid, title, parts FROM songs
		WHERE songbook_id = $1 AND NOT deleted
		ORDER BY title, fbid
	`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mine []catalogSong
	for rows.Next() {
		var song catalogSong
		var raw []byte
		song.SongbookID = bookID
		if err := rows.Scan(&song.FBID, &song.Title, &raw); err != nil {
			return nil, err
		}
		if err := decodeParts(raw, &song); err != nil {
			return nil, err
		}
		mine = append(mine, song)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	public, err := s.publicSongs(ctx)
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	if len(public) > 0 {
		titleRows, err := s.db.Query(ctx, `SELECT id::text, title FROM songbooks WHERE public AND status = 'approved'`)
		if err != nil {
			return nil, err
		}
		defer titleRows.Close()
		for titleRows.Next() {
			var id, title string
			if err := titleRows.Scan(&id, &title); err != nil {
				return nil, err
			}
			titles[id] = title
		}
		if err := titleRows.Err(); err != nil {
			return nil, err
		}
	}
	hits := []similarHit{}
	for _, source := range mine {
		for _, song := range public {
			if song.SongbookID == source.SongbookID && song.FBID == source.FBID {
				continue
			}
			kind := matchKind(source.Parts, song.Parts)
			if kind == "" {
				continue
			}
			hits = append(hits, similarHit{
				SongbookID:    song.SongbookID,
				SongbookTitle: titles[song.SongbookID],
				FBID:          song.FBID,
				Title:         song.Title,
				Match:         kind,
			})
			if len(hits) >= 20 {
				return hits, nil
			}
		}
	}
	return hits, nil
}

func decodeParts(raw []byte, song *catalogSong) error {
	return json.Unmarshal(raw, &song.Parts)
}

func (s *Server) adminModuleView(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireModerator(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	view, err := loadModule(ctx, s.db, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var community, email string
	var submitted *time.Time
	err = s.db.QueryRow(ctx, `
		SELECT m.submitted_at,
		       CASE WHEN d.kind = 'personal' THEN 'Личное' WHEN d.id IS NULL THEN '' ELSE d.name END,
		       COALESCE(owner.email, '')
		FROM modules m
		LEFT JOIN dimensions d ON d.id = m.dimension_id
		LEFT JOIN users owner ON owner.id = m.owner_user_id
		WHERE m.id = $1
	`, id).Scan(&submitted, &community, &email)
	if err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(ctx, `
		SELECT book, book_name, count(*)
		FROM module_verses WHERE module_id = $1
		GROUP BY book, book_name
		ORDER BY book
	`, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	type bookCount struct {
		Book     int    `json:"book"`
		BookName string `json:"book_name"`
		Verses   int    `json:"verses"`
	}
	books := []bookCount{}
	verses := 0
	for rows.Next() {
		var book bookCount
		if err := rows.Scan(&book.Book, &book.BookName, &book.Verses); err != nil {
			writeMapped(w, err)
			return
		}
		verses += book.Verses
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	audit, err := s.recentAudit(ctx, id)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":              view.ID,
		"title":           view.Title,
		"short_name":      view.ShortName,
		"status":          view.Status,
		"moderation_note": view.ModerationNote,
		"community_name":  community,
		"author_email":    email,
		"submitted_at":    submitted,
		"books":           books,
		"books_count":     len(books),
		"verses_count":    verses,
		"audit":           audit,
	})
}
