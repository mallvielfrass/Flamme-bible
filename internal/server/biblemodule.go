package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxModuleFile  = 16 << 20
	maxModuleBytes = 64 << 20
	maxModuleFiles = 80
	maxModuleBody  = maxModuleBytes + (1 << 20)
)

// errTooLarge — сумма файлов модуля больше предела.
var errTooLarge = errors.New("слишком большой модуль")

// BibleIdentity — имя модуля и хеш содержимого.
// Тот же текст собирает клиент в desktop/src-tauri/src/bible.rs.
// BibleId и BookHash не входят: VisioBible дописывает их в ini, не меняя книги.
// visiobible.cache и visiobible.idx не входят.
type BibleIdentity struct {
	ShortName   string
	Title       string
	ContentHash string
	Files       []string
	Bytes       int64
}

type moduleSyncView struct {
	Exists      bool   `json:"exists"`
	Uploaded    bool   `json:"uploaded,omitempty"`
	ID          string `json:"id,omitempty"`
	ShortName   string `json:"short_name,omitempty"`
	Title       string `json:"title,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
}

type bibleKV struct {
	key   string
	value string
}

type bibleBookCanon struct {
	path string
	keys []bibleKV
	rel  string
	sum  string
	size int64
}

func (s *Server) bibleModuleExists(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		writeMapped(w, err)
		return
	}
	var body struct {
		ContentHash string `json:"content_hash"`
		ShortName   string `json:"short_name"`
		Title       string `json:"title"`
	}
	if err := readJSON(r, &body); err != nil {
		writeMapped(w, errRequest)
		return
	}
	hash, shortName, title, ok := moduleIdentityFields(body.ContentHash, body.ShortName, body.Title)
	if !ok {
		writeMapped(w, errRequest)
		return
	}
	view, err := s.findBibleModule(r.Context(), hash, shortName, title)
	if err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) uploadBibleModule(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if s.moduleDir == "" {
		writeMapped(w, errModuleStore)
		return
	}
	limit := s.moduleBodyLimit()
	payload, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil && !requestTooLarge(err) {
		writeMapped(w, err)
		return
	}
	if requestTooLarge(err) || int64(len(payload)) > limit {
		writeJSON(w, http.StatusRequestEntityTooLarge, errJSON("запрос"))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(payload))
	r.ContentLength = int64(len(payload))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeMapped(w, errRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	hash, shortName, title, ok := moduleIdentityFields(r.FormValue("content_hash"), r.FormValue("short_name"), r.FormValue("title"))
	if !ok {
		writeMapped(w, errRequest)
		return
	}
	parts := r.MultipartForm.File["file"]
	if len(parts) == 0 || len(parts) > maxModuleFiles {
		writeMapped(w, errRequest)
		return
	}
	tmp, err := os.MkdirTemp(s.moduleDir, "incoming-")
	if err != nil {
		writeMapped(w, err)
		return
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()
	seen := map[string]bool{}
	var total int64
	for _, hdr := range parts {
		rel, err := safeModuleRel(hdr.Filename)
		if err != nil {
			writeMapped(w, err)
			return
		}
		key := strings.ToLower(rel)
		if seen[key] {
			writeMapped(w, errRequest)
			return
		}
		seen[key] = true
		src, err := hdr.Open()
		if err != nil {
			writeMapped(w, err)
			return
		}
		err = writeModuleFile(tmp, rel, src, &total)
		src.Close()
		if err != nil {
			writeModuleErr(w, err)
			return
		}
	}
	ident, err := bibleIdentity(tmp)
	if err != nil {
		writeModuleErr(w, err)
		return
	}
	if err := rejectExtraModuleFiles(tmp, ident); err != nil {
		writeMapped(w, err)
		return
	}
	if ident.ContentHash != hash || ident.ShortName != shortName || ident.Title != title {
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
	existing, err := lockBibleModule(ctx, tx, ident.ContentHash)
	if err == nil {
		if existing.ShortName != ident.ShortName || existing.Title != ident.Title {
			writeMapped(w, errConflict)
			return
		}
		writeJSON(w, http.StatusOK, moduleSyncView{
			Exists: true, ID: existing.ID, ShortName: existing.ShortName,
			Title: existing.Title, ContentHash: ident.ContentHash,
		})
		return
	}
	if !errors.Is(err, errNotFound) {
		writeMapped(w, err)
		return
	}
	id := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO modules (id, owner_user_id, dimension_id, short_name, title, status, content_hash, content_bytes, updated_at)
		VALUES ($1, $2, NULL, $3, $4, 'draft', $5, $6, now())
	`, id, p.UserID, ident.ShortName, ident.Title, ident.ContentHash, ident.Bytes)
	if err != nil {
		if isUniqueViolation(err) {
			winner, findErr := s.findBibleModule(ctx, ident.ContentHash, ident.ShortName, ident.Title)
			if findErr != nil {
				writeMapped(w, findErr)
				return
			}
			if winner.Exists {
				writeJSON(w, http.StatusOK, winner)
				return
			}
			writeMapped(w, errConflict)
			return
		}
		writeMapped(w, err)
		return
	}
	final := filepath.Join(s.moduleDir, id)
	if err := os.Rename(tmp, final); err != nil {
		writeMapped(w, err)
		return
	}
	cleanup = false
	if err := tx.Commit(ctx); err != nil {
		_ = os.RemoveAll(final)
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, moduleSyncView{
		Exists: true, Uploaded: true, ID: id, ShortName: ident.ShortName,
		Title: ident.Title, ContentHash: ident.ContentHash,
	})
}

func (s *Server) findBibleModule(ctx context.Context, hash, shortName, title string) (moduleSyncView, error) {
	var id, storedShort, storedTitle string
	err := s.db.QueryRow(ctx, `SELECT id::text, short_name, title FROM modules WHERE content_hash = $1`, hash).Scan(&id, &storedShort, &storedTitle)
	if err == pgx.ErrNoRows {
		return moduleSyncView{}, nil
	}
	if err != nil {
		return moduleSyncView{}, err
	}
	if storedShort != shortName || storedTitle != title {
		return moduleSyncView{}, nil
	}
	return moduleSyncView{Exists: true, ID: id, ShortName: storedShort, Title: storedTitle, ContentHash: hash}, nil
}

func lockBibleModule(ctx context.Context, tx pgx.Tx, hash string) (moduleSyncView, error) {
	var id, shortName, title string
	err := tx.QueryRow(ctx, `
		SELECT id::text, short_name, title FROM modules WHERE content_hash = $1 FOR UPDATE
	`, hash).Scan(&id, &shortName, &title)
	if err == pgx.ErrNoRows {
		return moduleSyncView{}, errNotFound
	}
	if err != nil {
		return moduleSyncView{}, err
	}
	return moduleSyncView{Exists: true, ID: id, ShortName: shortName, Title: title, ContentHash: hash}, nil
}

func (s *Server) removeStoredModule(id string) {
	if s.moduleDir == "" {
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		return
	}
	_ = os.RemoveAll(filepath.Join(s.moduleDir, id))
}

func moduleIdentityFields(hash, shortName, title string) (string, string, string, bool) {
	hash, ok := normalizeContentHash(hash)
	shortName = strings.TrimSpace(shortName)
	title = strings.TrimSpace(title)
	if !ok || !validModuleName(shortName) || utf8.RuneCountInString(title) > 200 {
		return "", "", "", false
	}
	return hash, shortName, title, true
}

func normalizeContentHash(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 64 {
		return "", false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return "", false
		}
	}
	return value, true
}

func writeModuleErr(w http.ResponseWriter, err error) {
	if errors.Is(err, errTooLarge) || requestTooLarge(err) {
		writeJSON(w, http.StatusRequestEntityTooLarge, errJSON("запрос"))
		return
	}
	writeMapped(w, err)
}

func requestTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var tooBig *http.MaxBytesError
	return errors.As(err, &tooBig) || errors.Is(err, multipart.ErrMessageTooLarge) || strings.Contains(err.Error(), "too large")
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func safeModuleRel(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, `/`)
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.Contains(name, "\x00") {
		return "", errRequest
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errRequest
		}
	}
	return strings.Join(parts, "/"), nil
}

func writeModuleFile(root, rel string, src io.Reader, total *int64) error {
	parts := strings.Split(rel, "/")
	dest := root
	for _, part := range parts {
		dest = filepath.Join(dest, part)
	}
	back, err := filepath.Rel(root, dest)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(os.PathSeparator)) {
		return errRequest
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return errRequest
	}
	defer file.Close()
	var fileTotal int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			fileTotal += int64(n)
			*total += int64(n)
			if fileTotal > maxModuleFile || *total > maxModuleBytes {
				return errTooLarge
			}
			if _, err := file.Write(buf[:n]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func bibleIdentity(dir string) (BibleIdentity, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return BibleIdentity{}, errRequest
	}
	iniName, err := findBibleINI(dir)
	if err != nil {
		return BibleIdentity{}, err
	}
	raw, err := readInside(dir, iniName)
	if err != nil {
		return BibleIdentity{}, err
	}
	if int64(len(raw)) > maxModuleFile {
		return BibleIdentity{}, errTooLarge
	}
	text, err := decodePlanText(raw)
	if err != nil {
		return BibleIdentity{}, err
	}
	shortName, title, header, books, err := parseBibleCanon(text)
	if err != nil {
		return BibleIdentity{}, err
	}
	var total = int64(len(raw))
	files := []string{iniName}
	var b strings.Builder
	b.WriteString("short=")
	b.WriteString(shortName)
	b.WriteByte('\n')
	b.WriteString("title=")
	b.WriteString(title)
	b.WriteByte('\n')
	for _, item := range header {
		b.WriteString("h=")
		b.WriteString(item.key)
		b.WriteByte('\t')
		b.WriteString(item.value)
		b.WriteByte('\n')
	}
	for i := range books {
		data, rel, err := readModuleRel(dir, books[i].path)
		if err != nil {
			return BibleIdentity{}, err
		}
		if int64(len(data)) > maxModuleFile {
			return BibleIdentity{}, errTooLarge
		}
		total += int64(len(data))
		if total > maxModuleBytes {
			return BibleIdentity{}, errTooLarge
		}
		sum := sha256.Sum256(data)
		books[i].rel = rel
		books[i].sum = hex.EncodeToString(sum[:])
		books[i].size = int64(len(data))
		files = append(files, rel)
		b.WriteString("book=")
		b.WriteString(books[i].path)
		b.WriteByte('\n')
		for _, item := range books[i].keys {
			b.WriteString("k=")
			b.WriteString(item.key)
			b.WriteByte('\t')
			b.WriteString(item.value)
			b.WriteByte('\n')
		}
		b.WriteString("file=")
		b.WriteString(books[i].sum)
		b.WriteByte('\n')
	}
	if len(files) > maxModuleFiles {
		return BibleIdentity{}, errTooLarge
	}
	sum := sha256.Sum256([]byte(b.String()))
	return BibleIdentity{
		ShortName:   shortName,
		Title:       title,
		ContentHash: hex.EncodeToString(sum[:]),
		Files:       files,
		Bytes:       total,
	}, nil
}

func parseBibleCanon(text string) (string, string, []bibleKV, []bibleBookCanon, error) {
	var header []bibleKV
	var books []bibleBookCanon
	var book *bibleBookCanon
	var shortName, title string
	flush := func() {
		if book == nil {
			return
		}
		books = append(books, *book)
		book = nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := cutINI(line)
		if !ok || key == "" {
			continue
		}
		if strings.EqualFold(key, "BibleId") || strings.EqualFold(key, "BookHash") {
			continue
		}
		if strings.EqualFold(key, "PathName") {
			flush()
			book = &bibleBookCanon{path: value}
			continue
		}
		if book == nil {
			if strings.EqualFold(key, "BibleShortName") {
				shortName = value
				continue
			}
			if strings.EqualFold(key, "BibleName") {
				title = value
				continue
			}
			header = append(header, bibleKV{key: key, value: value})
			continue
		}
		book.keys = append(book.keys, bibleKV{key: key, value: value})
	}
	flush()
	if !validModuleName(shortName) || utf8.RuneCountInString(title) > 200 || len(books) == 0 {
		return "", "", nil, nil, errRequest
	}
	seen := map[string]bool{}
	for _, item := range books {
		parts, err := moduleRelParts(item.path)
		if err != nil {
			return "", "", nil, nil, err
		}
		key := strings.ToLower(strings.Join(parts, "/"))
		if seen[key] {
			return "", "", nil, nil, errRequest
		}
		seen[key] = true
	}
	return shortName, title, header, books, nil
}

func moduleRelParts(path string) ([]string, error) {
	path = strings.ReplaceAll(path, `\`, `/`)
	if strings.Contains(path, "\x00") {
		return nil, errRequest
	}
	var parts []string
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return nil, errRequest
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, errRequest
	}
	return parts, nil
}

func findBibleINI(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var name string
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if strings.EqualFold(entry.Name(), "bibleqt.ini") {
				return "", errRequest
			}
			continue
		}
		if info.IsDir() || !strings.EqualFold(entry.Name(), "bibleqt.ini") {
			continue
		}
		if name != "" {
			return "", errRequest
		}
		name = entry.Name()
	}
	if name == "" {
		return "", errRequest
	}
	return name, nil
}

func readModuleRel(root, path string) ([]byte, string, error) {
	parts, err := moduleRelParts(path)
	if err != nil {
		return nil, "", err
	}
	current := root
	built := make([]string, 0, len(parts))
	for i, part := range parts {
		entries, err := os.ReadDir(current)
		if err != nil {
			return nil, "", errRequest
		}
		var match string
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				if match != "" {
					return nil, "", errRequest
				}
				match = entry.Name()
			}
		}
		if match == "" {
			return nil, "", errRequest
		}
		built = append(built, match)
		current = filepath.Join(current, match)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, "", errRequest
		}
		last := i == len(parts)-1
		if last {
			if info.IsDir() || info.Size() > maxModuleFile {
				return nil, "", errRequest
			}
		} else if !info.IsDir() {
			return nil, "", errRequest
		}
	}
	data, err := os.ReadFile(current)
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxModuleFile {
		return nil, "", errTooLarge
	}
	return data, strings.Join(built, "/"), nil
}

func rejectExtraModuleFiles(root string, ident BibleIdentity) error {
	keep := map[string]bool{}
	for _, rel := range ident.Files {
		keep[rel] = true
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return errRequest
		}
		if !keep[filepath.ToSlash(rel)] {
			return errRequest
		}
		return nil
	})
}
