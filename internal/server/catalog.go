package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const minhashSlots = 16

func (s *Server) requestPublish(w http.ResponseWriter, r *http.Request) {
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
	if _, err := tx.Exec(ctx, `UPDATE songbooks SET publish_requested = true WHERE id = $1`, id); err != nil {
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

func (s *Server) decidePublish(w http.ResponseWriter, r *http.Request) {
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
		Public *bool `json:"public"`
	}
	if err := readJSON(r, &body); err != nil || body.Public == nil {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
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
	var requested bool
	err = tx.QueryRow(ctx, `SELECT dimension_id::text, publish_requested FROM songbooks WHERE id = $1 FOR UPDATE`, id).Scan(&dimID, &requested)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	role, err := membershipTx(ctx, tx, p.UserID, dimID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if role != "admin" {
		writeMapped(w, errForbidden)
		return
	}
	if *body.Public && !requested {
		writeJSON(w, http.StatusConflict, errJSON("запрос"))
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE songbooks SET public = $2, publish_requested = false WHERE id = $1`, id, *body.Public); err != nil {
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

type catalogSong struct {
	SongbookID string
	FBID       string
	Title      string
	Parts      []partDTO
}

type catalogMatch struct {
	SongbookID string `json:"songbook_id"`
	FBID       string `json:"fbid"`
	Title      string `json:"title"`
	Match      string `json:"match"`
}

func (s *Server) catalogMatches(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	bookID := r.URL.Query().Get("songbook")
	fbid := r.URL.Query().Get("fbid")
	if _, err := uuid.Parse(bookID); err != nil || fbid == "" {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	ctx := r.Context()
	songs, err := s.publicSongs(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var source *catalogSong
	for i := range songs {
		if songs[i].SongbookID == bookID && songs[i].FBID == fbid {
			source = &songs[i]
			break
		}
	}
	if source == nil {
		writeMapped(w, errNotFound)
		return
	}
	hidden, err := s.hiddenCopies(ctx, p.UserID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	matches := []catalogMatch{}
	for _, song := range songs {
		if song.SongbookID == source.SongbookID && song.FBID == source.FBID {
			continue
		}
		if hidden[song.SongbookID+"\x00"+song.FBID] {
			continue
		}
		kind := matchKind(source.Parts, song.Parts)
		if kind == "" {
			continue
		}
		matches = append(matches, catalogMatch{
			SongbookID: song.SongbookID,
			FBID:       song.FBID,
			Title:      song.Title,
			Match:      kind,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

func (s *Server) hideCatalogCopy(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		SongbookID      string `json:"songbook_id"`
		FBID            string `json:"fbid"`
		AgainstSongbook string `json:"against_songbook"`
		AgainstFBID     string `json:"against_fbid"`
	}
	if err := readJSON(r, &body); err != nil || body.FBID == "" || body.AgainstFBID == "" {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	if _, err := uuid.Parse(body.SongbookID); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if _, err := uuid.Parse(body.AgainstSongbook); err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if body.SongbookID == body.AgainstSongbook && body.FBID == body.AgainstFBID {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
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
	err = tx.QueryRow(ctx, `
		SELECT dimension_id::text FROM songbooks WHERE id = $1 AND public
	`, body.AgainstSongbook).Scan(&dimID)
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
	songs, err := loadPublicSongs(ctx, tx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var source, other *catalogSong
	for i := range songs {
		if songs[i].SongbookID == body.AgainstSongbook && songs[i].FBID == body.AgainstFBID {
			source = &songs[i]
		}
		if songs[i].SongbookID == body.SongbookID && songs[i].FBID == body.FBID {
			other = &songs[i]
		}
	}
	if source == nil || other == nil || matchKind(source.Parts, other.Parts) == "" {
		writeMapped(w, errNotFound)
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO song_hides (user_id, songbook_id, fbid) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, p.UserID, body.SongbookID, body.FBID); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"hidden": true})
}

const publicSongsSQL = `
	SELECT s.songbook_id::text, s.fbid, s.title, s.parts
	FROM songs s
	JOIN songbooks b ON b.id = s.songbook_id
	WHERE b.public AND NOT s.deleted
	ORDER BY s.songbook_id, s.fbid`

func (s *Server) publicSongs(ctx context.Context) ([]catalogSong, error) {
	rows, err := s.db.Query(ctx, publicSongsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectSongs(rows)
}

func loadPublicSongs(ctx context.Context, tx pgx.Tx) ([]catalogSong, error) {
	rows, err := tx.Query(ctx, publicSongsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectSongs(rows)
}

func collectSongs(rows pgx.Rows) ([]catalogSong, error) {
	var list []catalogSong
	for rows.Next() {
		var song catalogSong
		var raw []byte
		if err := rows.Scan(&song.SongbookID, &song.FBID, &song.Title, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &song.Parts); err != nil {
			return nil, err
		}
		list = append(list, song)
	}
	return list, rows.Err()
}

func (s *Server) hiddenCopies(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT songbook_id::text, fbid FROM song_hides WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hidden := map[string]bool{}
	for rows.Next() {
		var book, fbid string
		if err := rows.Scan(&book, &fbid); err != nil {
			return nil, err
		}
		hidden[book+"\x00"+fbid] = true
	}
	return hidden, rows.Err()
}

func matchKind(a, b []partDTO) string {
	ah, asig := partSigs(a)
	bh, bsig := partSigs(b)
	for digest := range ah {
		if bh[digest] {
			return "hash"
		}
	}
	if minhashHit(asig, bsig) {
		return "minhash"
	}
	return ""
}

func partSigs(parts []partDTO) (map[string]bool, [][minhashSlots]uint64) {
	hashes := map[string]bool{}
	var sigs [][minhashSlots]uint64
	for _, part := range parts {
		text := normalizePartText(part.Text)
		if text == "" {
			continue
		}
		sum := sha256.Sum256([]byte(part.Kind + "\n" + text))
		hashes[hex.EncodeToString(sum[:])] = true
		if words := wordsOf(text); len(words) > 0 {
			sigs = append(sigs, minhash(words))
		}
	}
	return hashes, sigs
}

func normalizePartText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimRight(text, "\n")
}

func wordsOf(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	words := make([]string, 0, len(fields))
	for _, word := range fields {
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}

func minhash(words []string) [minhashSlots]uint64 {
	var slots [minhashSlots]uint64
	for i := range slots {
		slots[i] = ^uint64(0)
		prefix := strconv.Itoa(i) + "\x00"
		for _, word := range words {
			h := fnv.New64a()
			_, _ = h.Write([]byte(prefix))
			_, _ = h.Write([]byte(word))
			sum := h.Sum64()
			if sum < slots[i] {
				slots[i] = sum
			}
		}
	}
	return slots
}

func minhashHit(a, b [][minhashSlots]uint64) bool {
	for _, left := range a {
		for _, right := range b {
			same := 0
			for i := range left {
				if left[i] == right[i] {
					same++
				}
			}
			if same*2 >= minhashSlots {
				return true
			}
		}
	}
	return false
}
