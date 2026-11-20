package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"flammebible/internal/vbm"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type BookBinding struct {
	ID       string
	Path     string
	Identity string
}

type Operation struct {
	Seq      int64
	BookID   string
	OpID     string
	Kind     string
	SongFBID string
	Body     string
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func sqliteDSN(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return (&url.URL{
		Scheme:   "file",
		Path:     abs,
		RawQuery: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",
	}).String()
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS books (
  id TEXT PRIMARY KEY,
  path TEXT UNIQUE,
  identity TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS snapshots (
  book_id TEXT PRIMARY KEY REFERENCES books(id),
  body TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS operations (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  book_id TEXT NOT NULL REFERENCES books(id),
  op_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  song_fbid TEXT NOT NULL,
  body TEXT NOT NULL,
  UNIQUE(book_id, op_id)
);
CREATE TABLE IF NOT EXISTS replace_stage (
  book_id TEXT PRIMARY KEY REFERENCES books(id),
  stage TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pending_fbid (
  book_id TEXT NOT NULL REFERENCES books(id),
  song_key TEXT NOT NULL,
  fbid TEXT NOT NULL,
  PRIMARY KEY (book_id, song_key)
);
CREATE TABLE IF NOT EXISTS remote_books (
  book_id TEXT PRIMARY KEY REFERENCES books(id),
  server_id TEXT NOT NULL,
  cursor INTEGER NOT NULL DEFAULT 0,
  loaded INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS song_conflicts (
  book_id TEXT NOT NULL REFERENCES books(id),
  song_fbid TEXT NOT NULL,
  server_body TEXT NOT NULL,
  client_body TEXT NOT NULL,
  PRIMARY KEY (book_id, song_fbid)
);
CREATE TABLE IF NOT EXISTS patch_decisions (
  server_id TEXT NOT NULL,
  song_fbid TEXT NOT NULL,
  decision TEXT NOT NULL,
  PRIMARY KEY (server_id, song_fbid)
);
`)
	return err
}

func (s *Store) Bind(modulePath string) (BookBinding, error) {
	data, err := os.ReadFile(modulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return BookBinding{}, errFileNotFound
		}
		return BookBinding{}, err
	}
	book, err := vbm.Parse(data)
	if err != nil {
		return BookBinding{}, err
	}
	identity := moduleIdentity(book, data)
	abs, err := filepath.Abs(modulePath)
	if err != nil {
		return BookBinding{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return BookBinding{}, err
	}
	defer tx.Rollback()

	var current BookBinding
	err = tx.QueryRow(`SELECT id, path, identity FROM books WHERE path = ?`, abs).
		Scan(&current.ID, &current.Path, &current.Identity)
	if err == nil && current.Identity == identity {
		return current, nil
	}
	if err == nil {
		if _, err := tx.Exec(`UPDATE books SET path = NULL WHERE id = ?`, current.ID); err != nil {
			return BookBinding{}, err
		}
	} else if err != sql.ErrNoRows {
		return BookBinding{}, err
	} else {
		folded, found, ferr := caseFoldedBinding(tx, abs, identity)
		if ferr != nil {
			return BookBinding{}, ferr
		}
		if found {
			if _, err := tx.Exec(`UPDATE books SET path = ? WHERE id = ?`, abs, folded.ID); err != nil {
				return BookBinding{}, err
			}
			if err := tx.Commit(); err != nil {
				return BookBinding{}, err
			}
			folded.Path = abs
			return folded, nil
		}
	}

	id, err := newUUID()
	if err != nil {
		return BookBinding{}, err
	}
	if _, err := tx.Exec(`INSERT INTO books(id, path, identity) VALUES(?, ?, ?)`, id, abs, identity); err != nil {
		return BookBinding{}, err
	}
	if _, err := tx.Exec(`INSERT INTO replace_stage(book_id, stage) VALUES(?, 'idle')`, id); err != nil {
		return BookBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return BookBinding{}, err
	}
	return BookBinding{ID: id, Path: abs, Identity: identity}, nil
}

func (s *Store) SaveSnapshot(bookID string, songs []vbm.Song) error {
	if songs == nil {
		songs = []vbm.Song{}
	}
	body, err := json.Marshal(songs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO snapshots(book_id, body) VALUES(?, ?)
ON CONFLICT(book_id) DO UPDATE SET body = excluded.body
`, bookID, string(body))
	return err
}

func (s *Store) Snapshot(bookID string) ([]vbm.Song, error) {
	var body string
	err := s.db.QueryRow(`SELECT body FROM snapshots WHERE book_id = ?`, bookID).Scan(&body)
	if err == sql.ErrNoRows {
		return []vbm.Song{}, nil
	}
	if err != nil {
		return nil, err
	}
	var songs []vbm.Song
	if err := json.Unmarshal([]byte(body), &songs); err != nil {
		return nil, err
	}
	if songs == nil {
		songs = []vbm.Song{}
	}
	return songs, nil
}

func (s *Store) Enqueue(bookID, kind, songFBID, body string) error {
	opID, err := newUUID()
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO operations(book_id, op_id, kind, song_fbid, body) VALUES(?, ?, ?, ?, ?)
`, bookID, opID, kind, songFBID, body)
	return err
}

func (s *Store) Operations(bookID string) ([]Operation, error) {
	rows, err := s.db.Query(`
SELECT seq, book_id, op_id, kind, song_fbid, body
FROM operations WHERE book_id = ? ORDER BY seq
`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.Seq, &op.BookID, &op.OpID, &op.Kind, &op.SongFBID, &op.Body); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	if ops == nil {
		ops = []Operation{}
	}
	return ops, rows.Err()
}

func (s *Store) SetStage(bookID, stage string) error {
	_, err := s.db.Exec(`
INSERT INTO replace_stage(book_id, stage) VALUES(?, ?)
ON CONFLICT(book_id) DO UPDATE SET stage = excluded.stage
`, bookID, stage)
	return err
}

func (s *Store) Stage(bookID string) (string, error) {
	var stage string
	err := s.db.QueryRow(`SELECT stage FROM replace_stage WHERE book_id = ?`, bookID).Scan(&stage)
	return stage, err
}

func (s *Store) bindingCount(path string) (int, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	var n int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE path = ?`, abs).Scan(&n)
	return n, err
}

func caseFoldedBinding(tx *sql.Tx, abs, identity string) (BookBinding, bool, error) {
	rows, err := tx.Query(`SELECT id, path, identity FROM books WHERE path IS NOT NULL AND identity = ?`, identity)
	if err != nil {
		return BookBinding{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var row BookBinding
		if err := rows.Scan(&row.ID, &row.Path, &row.Identity); err != nil {
			return BookBinding{}, false, err
		}
		if !strings.EqualFold(row.Path, abs) {
			continue
		}
		_, err := os.Stat(row.Path)
		if err == nil {
			continue
		}
		if !os.IsNotExist(err) {
			return BookBinding{}, false, err
		}
		return row, true, nil
	}
	return BookBinding{}, false, rows.Err()
}

// moduleIdentity отличает книгу по шаблону заголовка, а не по тексту песен:
// чужой файл на том же пути получает новый id, правка песни — нет.
func moduleIdentity(book *vbm.Book, template []byte) string {
	sum := sha256.New()
	if len(template) >= 72 {
		sum.Write(template[:20])
		sum.Write(template[28:72])
	}
	for _, part := range []string{
		book.Header.HeaderShort,
		book.Header.HeaderTitle,
		book.Title,
		book.ShortName,
		book.Publisher,
	} {
		sum.Write([]byte(part))
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
}
