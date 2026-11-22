package server

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type sectionView struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Position int      `json:"position"`
	Count    int      `json:"count"`
	Songs    []string `json:"songs"`
}

type treeSection struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

type treeBook struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	ShortName    string        `json:"short_name"`
	Status       string        `json:"status"`
	IndexPending int           `json:"index_pending"`
	IndexFailed  int           `json:"index_failed"`
	Sections     []treeSection `json:"sections"`
}

type treeGroup struct {
	Group       string     `json:"group"`
	DimensionID string     `json:"dimension_id,omitempty"`
	Name        string     `json:"name"`
	Songbooks   []treeBook `json:"songbooks"`
}

func (s *Server) songbookTree(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	out, err := s.loadTree(r.Context(), p.UserID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) loadTree(ctx context.Context, userID string) ([]treeGroup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT b.id::text, b.title, b.short_name, b.status, b.dimension_id::text, d.kind, d.name,
		       EXISTS (
		         SELECT 1 FROM memberships m
		         WHERE m.dimension_id = b.dimension_id AND m.user_id = $1 AND NOT m.excluded
		       )
		FROM songbooks b
		JOIN dimensions d ON d.id = b.dimension_id
		WHERE NOT b.hidden AND b.hidden_at IS NULL
		  AND (
		        b.status = 'approved'
		        OR EXISTS (
		            SELECT 1 FROM memberships m
		            WHERE m.dimension_id = b.dimension_id AND m.user_id = $1 AND NOT m.excluded
		        )
		      )
		ORDER BY d.name, b.title, b.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type bookRow struct {
		book treeBook
		dim  string
		kind string
		name string
		own  bool
	}
	var books []bookRow
	var ids []string
	for rows.Next() {
		var row bookRow
		if err := rows.Scan(&row.book.ID, &row.book.Title, &row.book.ShortName, &row.book.Status, &row.dim, &row.kind, &row.name, &row.own); err != nil {
			return nil, err
		}
		row.book.Sections = []treeSection{}
		books = append(books, row)
		ids = append(ids, row.book.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	byBook := map[string][]treeSection{}
	if len(ids) > 0 {
		secRows, err := s.db.Query(ctx, `
			SELECT sec.songbook_id::text, sec.id::text, sec.title,
			       (SELECT count(*) FROM songbook_section_songs ss WHERE ss.section_id = sec.id)
			FROM songbook_sections sec
			WHERE sec.songbook_id::text = ANY($1::text[])
			ORDER BY sec.position, sec.title, sec.id
		`, ids)
		if err != nil {
			return nil, err
		}
		defer secRows.Close()
		for secRows.Next() {
			var bookID string
			var sec treeSection
			if err := secRows.Scan(&bookID, &sec.ID, &sec.Title, &sec.Count); err != nil {
				return nil, err
			}
			byBook[bookID] = append(byBook[bookID], sec)
		}
		if err := secRows.Err(); err != nil {
			return nil, err
		}
	}
	indexCounts, err := s.songIndexCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	var personal *treeGroup
	var approved *treeGroup
	var communities []*treeGroup
	communityAt := map[string]int{}
	for _, row := range books {
		if c, ok := indexCounts[row.book.ID]; ok {
			row.book.IndexPending = c.Pending
			row.book.IndexFailed = c.Failed
		}
		row.book.Sections = byBook[row.book.ID]
		if row.book.Sections == nil {
			row.book.Sections = []treeSection{}
		}
		switch {
		case row.own && row.kind == "community":
			index, ok := communityAt[row.dim]
			if !ok {
				index = len(communities)
				communityAt[row.dim] = index
				communities = append(communities, &treeGroup{
					Group: "community", DimensionID: row.dim, Name: row.name, Songbooks: []treeBook{},
				})
			}
			communities[index].Songbooks = append(communities[index].Songbooks, row.book)
		case row.own:
			if personal == nil {
				personal = &treeGroup{Group: "personal", DimensionID: row.dim, Name: row.name, Songbooks: []treeBook{}}
			}
			personal.Songbooks = append(personal.Songbooks, row.book)
		case row.book.Status == "approved":
			if approved == nil {
				approved = &treeGroup{Group: "approved", Name: "Одобренные общие", Songbooks: []treeBook{}}
			}
			approved.Songbooks = append(approved.Songbooks, row.book)
		}
	}
	out := []treeGroup{}
	for _, group := range communities {
		out = append(out, *group)
	}
	if personal != nil {
		out = append(out, *personal)
	}
	if approved != nil {
		out = append(out, *approved)
	}
	return out, nil
}

func (s *Server) listSections(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	bookID := r.PathValue("id")
	if _, err := uuid.Parse(bookID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if _, err := s.songbookDimension(r.Context(), p.UserID, bookID); err != nil {
		writeMapped(w, err)
		return
	}
	list, err := loadSections(r.Context(), s.db, bookID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createSection(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	bookID := r.PathValue("id")
	if _, err := uuid.Parse(bookID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := readJSON(r, &body); err != nil || !sectionTitleOK(body.Title) {
		writeMapped(w, errRequest)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := s.songbookDimensionTx(ctx, tx, p.UserID, bookID); err != nil {
		writeMapped(w, err)
		return
	}
	var position int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), -1) + 1 FROM songbook_sections WHERE songbook_id = $1
	`, bookID).Scan(&position); err != nil {
		writeMapped(w, err)
		return
	}
	id := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO songbook_sections (id, songbook_id, title, position) VALUES ($1, $2, $3, $4)
	`, id, bookID, strings.TrimSpace(body.Title), position); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := loadSection(ctx, tx, id)
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

func (s *Server) renameSection(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	bookID := r.PathValue("id")
	sectionID := r.PathValue("sectionID")
	if _, err := uuid.Parse(bookID); err != nil || uuid.Validate(sectionID) != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := readJSON(r, &body); err != nil || !sectionTitleOK(body.Title) {
		writeMapped(w, errRequest)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if err := s.sectionInBook(ctx, tx, p.UserID, bookID, sectionID); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE songbook_sections SET title = $2 WHERE id = $1`, sectionID, strings.TrimSpace(body.Title)); err != nil {
		writeMapped(w, err)
		return
	}
	view, err := loadSection(ctx, tx, sectionID)
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

func (s *Server) deleteSection(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	bookID := r.PathValue("id")
	sectionID := r.PathValue("sectionID")
	if _, err := uuid.Parse(bookID); err != nil || uuid.Validate(sectionID) != nil {
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
	if err := s.sectionInBook(ctx, tx, p.UserID, bookID, sectionID); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM songbook_sections WHERE id = $1`, sectionID); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) setSectionSongs(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	sectionID := r.PathValue("id")
	if _, err := uuid.Parse(sectionID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	var body struct {
		FBIDs []string `json:"fbids"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	seen := map[string]bool{}
	for _, fbid := range body.FBIDs {
		fbid = strings.TrimSpace(fbid)
		if fbid == "" || utf8.RuneCountInString(fbid) > 80 || seen[fbid] {
			writeMapped(w, errRequest)
			return
		}
		seen[fbid] = true
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var bookID string
	err = tx.QueryRow(ctx, `SELECT songbook_id::text FROM songbook_sections WHERE id = $1 FOR UPDATE`, sectionID).Scan(&bookID)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := s.songbookDimensionTx(ctx, tx, p.UserID, bookID); err != nil {
		writeMapped(w, err)
		return
	}
	for _, fbid := range body.FBIDs {
		var deleted bool
		err := tx.QueryRow(ctx, `
			SELECT deleted FROM songs WHERE songbook_id = $1 AND fbid = $2
		`, bookID, strings.TrimSpace(fbid)).Scan(&deleted)
		if err == pgx.ErrNoRows || deleted {
			writeMapped(w, errRequest)
			return
		}
		if err != nil {
			writeMapped(w, err)
			return
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM songbook_section_songs WHERE section_id = $1`, sectionID); err != nil {
		writeMapped(w, err)
		return
	}
	for i, fbid := range body.FBIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO songbook_section_songs (section_id, songbook_id, fbid, position)
			VALUES ($1, $2, $3, $4)
		`, sectionID, bookID, strings.TrimSpace(fbid), i); err != nil {
			writeMapped(w, err)
			return
		}
	}
	view, err := loadSection(ctx, tx, sectionID)
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

func sectionTitleOK(title string) bool {
	title = strings.TrimSpace(title)
	return title != "" && utf8.RuneCountInString(title) <= 200
}

func (s *Server) songbookDimension(ctx context.Context, userID, bookID string) (string, error) {
	var dimID string
	err := s.db.QueryRow(ctx, `SELECT dimension_id::text FROM songbooks WHERE id = $1`, bookID).Scan(&dimID)
	if err == pgx.ErrNoRows {
		return "", errNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := s.membership(ctx, userID, dimID); err != nil {
		return "", err
	}
	return dimID, nil
}

func (s *Server) songbookDimensionTx(ctx context.Context, tx pgx.Tx, userID, bookID string) (string, error) {
	var dimID string
	err := tx.QueryRow(ctx, `SELECT dimension_id::text FROM songbooks WHERE id = $1 FOR UPDATE`, bookID).Scan(&dimID)
	if err == pgx.ErrNoRows {
		return "", errNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := membershipTx(ctx, tx, userID, dimID); err != nil {
		return "", err
	}
	return dimID, nil
}

func (s *Server) sectionInBook(ctx context.Context, tx pgx.Tx, userID, bookID, sectionID string) error {
	if _, err := s.songbookDimensionTx(ctx, tx, userID, bookID); err != nil {
		return err
	}
	var got string
	err := tx.QueryRow(ctx, `
		SELECT id::text FROM songbook_sections WHERE id = $1 AND songbook_id = $2 FOR UPDATE
	`, sectionID, bookID).Scan(&got)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	return err
}

func loadSections(ctx context.Context, q planQuerier, bookID string) ([]sectionView, error) {
	rows, err := q.Query(ctx, `
		SELECT id::text, title, position
		FROM songbook_sections
		WHERE songbook_id = $1
		ORDER BY position, title, id
	`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []sectionView{}
	for rows.Next() {
		var view sectionView
		if err := rows.Scan(&view.ID, &view.Title, &view.Position); err != nil {
			return nil, err
		}
		view.Songs = []string{}
		list = append(list, view)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range list {
		songs, err := sectionSongs(ctx, q, list[i].ID)
		if err != nil {
			return nil, err
		}
		list[i].Songs = songs
		list[i].Count = len(songs)
	}
	return list, nil
}

func loadSection(ctx context.Context, q planQuerier, id string) (sectionView, error) {
	var view sectionView
	err := q.QueryRow(ctx, `
		SELECT id::text, title, position FROM songbook_sections WHERE id = $1
	`, id).Scan(&view.ID, &view.Title, &view.Position)
	if err == pgx.ErrNoRows {
		return sectionView{}, errNotFound
	}
	if err != nil {
		return sectionView{}, err
	}
	songs, err := sectionSongs(ctx, q, id)
	if err != nil {
		return sectionView{}, err
	}
	view.Songs = songs
	view.Count = len(songs)
	return view, nil
}

func sectionSongs(ctx context.Context, q planQuerier, id string) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT fbid FROM songbook_section_songs WHERE section_id = $1 ORDER BY position, fbid
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	songs := []string{}
	for rows.Next() {
		var fbid string
		if err := rows.Scan(&fbid); err != nil {
			return nil, err
		}
		songs = append(songs, fbid)
	}
	return songs, rows.Err()
}
