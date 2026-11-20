package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"flammebible/internal/vbm"
)

var (
	errFileNotFound  = errors.New("файл не найден")
	errBadFBID       = errors.New("битый FBID")
	errDuplicateFBID = errors.New("повторный FBID")
)

type Change struct {
	Kind  string
	FBID  string
	Index int
	Title string
}

func (s *Store) Diff(modulePath string) ([]Change, error) {
	if _, err := os.Stat(modulePath); err != nil {
		if os.IsNotExist(err) {
			return nil, errFileNotFound
		}
		return nil, err
	}
	book, err := vbm.ParseFile(modulePath)
	if err != nil {
		return nil, err
	}
	fileIDs := make([]string, len(book.Songs))
	fileHas := make([]bool, len(book.Songs))
	seen := map[string]struct{}{}
	for i, song := range book.Songs {
		fbid, ok, err := songFBID(song.Properties)
		if err != nil {
			return nil, err
		}
		if ok {
			if _, dup := seen[fbid]; dup {
				return nil, errDuplicateFBID
			}
			seen[fbid] = struct{}{}
		}
		fileIDs[i] = fbid
		fileHas[i] = ok
	}
	binding, err := s.Bind(modulePath)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.Snapshot(binding.ID)
	if err != nil {
		return nil, err
	}
	snapByID := map[string]vbm.Song{}
	for _, song := range snapshot {
		fbid, ok, err := songFBID(song.Properties)
		if err != nil || !ok {
			continue
		}
		snapByID[fbid] = song
	}
	changes := []Change{}
	keys := make([]string, 0)
	used := map[string]struct{}{}
	for i, song := range book.Songs {
		if !fileHas[i] {
			changes = append(changes, Change{Kind: "added", Index: i, Title: song.Title})
			keys = append(keys, newSongKey(i, song))
			continue
		}
		used[fileIDs[i]] = struct{}{}
		prev, known := snapByID[fileIDs[i]]
		if !known {
			changes = append(changes, Change{Kind: "added", FBID: fileIDs[i], Index: i, Title: song.Title})
			keys = append(keys, "")
			continue
		}
		if !songEqual(prev, song) {
			changes = append(changes, Change{Kind: "changed", FBID: fileIDs[i], Index: i, Title: song.Title})
			keys = append(keys, "")
		}
	}
	for _, song := range snapshot {
		fbid, ok, err := songFBID(song.Properties)
		if err != nil || !ok {
			continue
		}
		if _, keep := used[fbid]; keep {
			continue
		}
		changes = append(changes, Change{Kind: "deleted", FBID: fbid, Index: -1, Title: song.Title})
		keys = append(keys, "")
	}
	if err := s.replaceQueue(binding.ID, changes, keys); err != nil {
		return nil, err
	}
	return changes, nil
}

func (s *Store) replaceQueue(bookID string, changes []Change, keys []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := range changes {
		if keys[i] == "" {
			continue
		}
		fbid, err := pendingFBID(tx, bookID, keys[i])
		if err != nil {
			return err
		}
		changes[i].FBID = fbid
	}
	if _, err := tx.Exec(`DELETE FROM operations WHERE book_id = ?`, bookID); err != nil {
		return err
	}
	for _, change := range changes {
		opID, err := newUUID()
		if err != nil {
			return err
		}
		body, err := json.Marshal(change)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
INSERT INTO operations(book_id, op_id, kind, song_fbid, body) VALUES(?, ?, ?, ?, ?)
`, bookID, opID, change.Kind, change.FBID, string(body)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func pendingFBID(tx *sql.Tx, bookID, key string) (string, error) {
	var fbid string
	err := tx.QueryRow(`SELECT fbid FROM pending_fbid WHERE book_id = ? AND song_key = ?`, bookID, key).Scan(&fbid)
	if err == nil {
		return fbid, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	fbid, err = newUUID()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(`INSERT INTO pending_fbid(book_id, song_key, fbid) VALUES(?, ?, ?)`, bookID, key, fbid)
	return fbid, err
}

func songFBID(properties string) (string, bool, error) {
	text := strings.ReplaceAll(properties, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	found := ""
	ok := false
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "$FBID$=") {
			continue
		}
		if ok {
			return "", false, errDuplicateFBID
		}
		value := strings.TrimPrefix(line, "$FBID$=")
		if !validUUID(value) {
			return "", false, errBadFBID
		}
		found = strings.ToLower(value)
		ok = true
	}
	return found, ok, nil
}

func songEqual(a, b vbm.Song) bool {
	if a.Title != b.Title || a.AltTitle != b.AltTitle || a.Author != b.Author || a.Composer != b.Composer || a.Note != b.Note || a.Properties != b.Properties {
		return false
	}
	if len(a.Parts) != len(b.Parts) {
		return false
	}
	for i := range a.Parts {
		if a.Parts[i].Kind != b.Parts[i].Kind || a.Parts[i].Align != b.Parts[i].Align {
			return false
		}
		if normalizePart(a.Parts[i].Text) != normalizePart(b.Parts[i].Text) {
			return false
		}
	}
	return true
}

// normalizePart уравнивает LF и CRLF и отбрасывает конечный перевод строки.
// Пробелы внутри текста не трогает.
func normalizePart(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimRight(text, "\n")
}

func newSongKey(index int, song vbm.Song) string {
	sum := sha256.New()
	fmt.Fprintf(sum, "%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s",
		index, song.Title, song.AltTitle, song.Author, song.Composer, song.Note, song.Properties)
	for _, part := range song.Parts {
		fmt.Fprintf(sum, "\x00%s\x00%s\x00%d", part.Kind, part.Text, part.Align)
	}
	return hex.EncodeToString(sum.Sum(nil))
}
