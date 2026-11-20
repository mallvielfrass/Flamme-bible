package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

type songHit struct {
	SongbookID    string `json:"songbook_id"`
	FBID          string `json:"fbid"`
	Title         string `json:"title"`
	SongbookTitle string `json:"songbook_title"`
	DimensionID   string `json:"dimension_id"`
	Status        string `json:"status"`
}

func (s *Server) searchSongs(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || utf8.RuneCountInString(q) > 200 {
		writeMapped(w, errRequest)
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeMapped(w, errRequest)
			return
		}
		limit = n
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 10000 {
			writeMapped(w, errRequest)
			return
		}
		offset = n
	}
	bookArg, dimArg, err := songSearchScope(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT s.songbook_id::text, s.fbid, s.title, b.title, b.dimension_id::text, b.status
		FROM songs s
		JOIN songbooks b ON b.id = s.songbook_id
		WHERE `+songAccessWhere("$2", "$3", "$4")+`
		  AND (
		        s.title ILIKE $1 ESCAPE '\'
		        OR s.alt_title ILIKE $1 ESCAPE '\'
		        OR EXISTS (
		            SELECT 1
		            FROM jsonb_array_elements(CASE WHEN jsonb_typeof(s.parts) = 'array' THEN s.parts ELSE '[]'::jsonb END) elem
		            WHERE COALESCE(elem->>'text', '') ILIKE $1 ESCAPE '\'
		        )
		      )
		ORDER BY s.title, s.songbook_id, s.fbid
		LIMIT $5 OFFSET $6
	`, likeContains(q), bookArg, dimArg, p.UserID, limit, offset)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	hits := []songHit{}
	for rows.Next() {
		var hit songHit
		if err := rows.Scan(&hit.SongbookID, &hit.FBID, &hit.Title, &hit.SongbookTitle, &hit.DimensionID, &hit.Status); err != nil {
			writeMapped(w, err)
			return
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"songs": hits})
}

// songAccessWhere — то же правило, что у поиска по словам и по смыслу:
// песня не удалена, сборник свой (пользователь не исключён) или одобрен.
func songAccessWhere(bookArg, dimArg, userArg string) string {
	return fmt.Sprintf(`NOT s.deleted
		  AND (%s::uuid IS NULL OR s.songbook_id = %s::uuid)
		  AND (%s::uuid IS NULL OR b.dimension_id = %s::uuid)
		  AND (
		        b.status = 'approved'
		        OR EXISTS (
		            SELECT 1 FROM memberships m
		            WHERE m.dimension_id = b.dimension_id AND m.user_id = %s AND NOT m.excluded
		        )
		      )`, bookArg, bookArg, dimArg, dimArg, userArg)
}

func songSearchScope(r *http.Request) (book any, dim any, err error) {
	songbookID := strings.TrimSpace(r.URL.Query().Get("songbook_id"))
	if songbookID != "" {
		if !validUUID(songbookID) {
			return nil, nil, errRequest
		}
		book = songbookID
	}
	dimensionID := strings.TrimSpace(r.URL.Query().Get("dimension_id"))
	if dimensionID == "" {
		dimensionID = strings.TrimSpace(r.URL.Query().Get("community_id"))
	}
	if dimensionID != "" {
		if !validUUID(dimensionID) {
			return nil, nil, errRequest
		}
		dim = dimensionID
	}
	return book, dim, nil
}

func likeContains(q string) string {
	var buf strings.Builder
	buf.WriteByte('%')
	for _, r := range q {
		if r == '%' || r == '_' || r == '\\' {
			buf.WriteByte('\\')
		}
		buf.WriteRune(r)
	}
	buf.WriteByte('%')
	return buf.String()
}
