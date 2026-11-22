package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type songHit struct {
	SongbookID    string `json:"songbook_id"`
	FBID          string `json:"fbid"`
	Title         string `json:"title"`
	SongbookTitle string `json:"songbook_title"`
	DimensionID   string `json:"dimension_id"`
	Status        string `json:"status"`
	SectionTitle  string `json:"section_title,omitempty"`
}

func (s *Server) searchSongs(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 200 {
		writeMapped(w, errRequest)
		return
	}
	book, dim, section, err := songSearchScope(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	// Пустой запрос допустим только со сборником: тогда это список его песен.
	if q == "" && book == nil {
		writeMapped(w, errRequest)
		return
	}
	limit := 20
	maxLimit := 100
	if q == "" {
		limit = 500
		maxLimit = 1000
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxLimit {
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
	hits, err := s.findSongs(r.Context(), p.UserID, q, book, dim, section, limit, offset)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"songs": hits})
}

func (s *Server) findSongs(ctx context.Context, userID, q string, book, dim, section any, limit, offset int) ([]songHit, error) {
	if err := s.usableSection(ctx, userID, section, book); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT s.songbook_id::text, s.fbid, s.title, b.title, b.dimension_id::text, b.status,
		       `+sectionTitleSQL+`
		FROM songs s
		JOIN songbooks b ON b.id = s.songbook_id
		WHERE `+songAccessWhere("$2", "$3", "$4")+`
		  `+sectionWhere("$7")+`
		  AND (
		        $1::text IS NULL
		        OR s.title ILIKE $1 ESCAPE '\'
		        OR s.alt_title ILIKE $1 ESCAPE '\'
		        OR EXISTS (
		            SELECT 1
		            FROM jsonb_array_elements(CASE WHEN jsonb_typeof(s.parts) = 'array' THEN s.parts ELSE '[]'::jsonb END) elem
		            WHERE COALESCE(elem->>'text', '') ILIKE $1 ESCAPE '\'
		        )
		      )
		ORDER BY s.title, s.songbook_id, s.fbid
		LIMIT $5 OFFSET $6
	`, likePattern(q), book, dim, userID, limit, offset, section)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []songHit{}
	for rows.Next() {
		var hit songHit
		if err := rows.Scan(&hit.SongbookID, &hit.FBID, &hit.Title, &hit.SongbookTitle, &hit.DimensionID, &hit.Status, &hit.SectionTitle); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hits, nil
}

// songAccessWhere — то же правило, что у поиска по словам и по смыслу:
// песня не удалена, сборник не скрыт админкой (hidden_at), свой или одобрен.
func songAccessWhere(bookArg, dimArg, userArg string) string {
	return fmt.Sprintf(`NOT s.deleted
		  AND b.hidden_at IS NULL
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

const sectionTitleSQL = `COALESCE((
  SELECT sec.title
  FROM songbook_section_songs ss
  JOIN songbook_sections sec ON sec.id = ss.section_id
  WHERE ss.songbook_id = s.songbook_id AND ss.fbid = s.fbid
  ORDER BY sec.position, sec.title, sec.id
  LIMIT 1
), '')`

func sectionWhere(arg string) string {
	return fmt.Sprintf(`AND (%s::uuid IS NULL OR EXISTS (
		SELECT 1 FROM songbook_section_songs filter_ss
		WHERE filter_ss.section_id = %s::uuid
		  AND filter_ss.songbook_id = s.songbook_id
		  AND filter_ss.fbid = s.fbid
	))`, arg, arg)
}

func (s *Server) usableSection(ctx context.Context, userID string, section, book any) error {
	raw, _ := section.(string)
	if raw == "" {
		return nil
	}
	var bookID string
	err := s.db.QueryRow(ctx, `
		SELECT sec.songbook_id::text
		FROM songbook_sections sec
		JOIN songbooks b ON b.id = sec.songbook_id
		WHERE sec.id = $1
		  AND (
		        b.status = 'approved'
		        OR EXISTS (
		            SELECT 1 FROM memberships m
		            WHERE m.dimension_id = b.dimension_id AND m.user_id = $2 AND NOT m.excluded
		        )
		      )
	`, raw, userID).Scan(&bookID)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if id, ok := book.(string); ok && id != "" && id != bookID {
		return errRequest
	}
	return nil
}

func songSearchScope(r *http.Request) (book any, dim any, section any, err error) {
	songbookID := strings.TrimSpace(r.URL.Query().Get("songbook_id"))
	if songbookID != "" {
		if !validUUID(songbookID) {
			return nil, nil, nil, errRequest
		}
		book = songbookID
	}
	dimensionID := strings.TrimSpace(r.URL.Query().Get("dimension_id"))
	if dimensionID == "" {
		dimensionID = strings.TrimSpace(r.URL.Query().Get("community_id"))
	}
	if dimensionID != "" {
		if !validUUID(dimensionID) {
			return nil, nil, nil, errRequest
		}
		dim = dimensionID
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("section_id")); raw != "" {
		if !validUUID(raw) {
			return nil, nil, nil, errRequest
		}
		section = raw
	}
	return book, dim, section, nil
}

func likePattern(q string) any {
	if strings.TrimSpace(q) == "" {
		return nil
	}
	return likeContains(q)
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
