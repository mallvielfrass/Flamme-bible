package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxArtifact    = 200 << 20
	maxReleaseBody = 512 << 20
)

var releaseVersionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$`)

type clientRelease struct {
	Version       string     `json:"version"`
	Status        string     `json:"status"`
	LinuxSHA256   string     `json:"linux_sha256"`
	LinuxSize     int64      `json:"linux_size"`
	WindowsSHA256 string     `json:"windows_sha256"`
	WindowsSize   int64      `json:"windows_size"`
	MacosSHA256   string     `json:"macos_sha256"`
	MacosSize     int64      `json:"macos_size"`
	Notes         string     `json:"notes"`
	CreatedAt     time.Time  `json:"created_at"`
	PublishedAt   *time.Time `json:"published_at"`
}

func validReleaseVersion(version string) bool {
	if strings.Contains(version, "..") || strings.ContainsAny(version, `/\`) {
		return false
	}
	return releaseVersionRe.MatchString(version)
}

func artifactName(platform string) (string, error) {
	switch platform {
	case "linux":
		return "flammebible-linux", nil
	case "windows":
		return "flammebible-windows.exe", nil
	case "macos":
		return "flammebible-macos", nil
	default:
		return "", errRequest
	}
}

func (s *Server) uploadRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	root, err := releaseRoot()
	if err != nil {
		writeMapped(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxReleaseBody)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errJSON("запрос"))
			return
		}
		writeMapped(w, errRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	version := strings.TrimSpace(r.FormValue("version"))
	if !validReleaseVersion(version) {
		writeMapped(w, errRequest)
		return
	}
	notes := strings.TrimSpace(r.FormValue("notes"))
	if len(notes) > 4000 {
		writeMapped(w, errRequest)
		return
	}
	type incoming struct {
		platform string
		file     multipart.File
		expect   string
	}
	fields := []struct {
		platform string
		field    string
		sumField string
	}{
		{"linux", "linux_file", "linux_sha256"},
		{"windows", "windows_file", "windows_sha256"},
		{"macos", "macos_file", "macos_sha256"},
	}
	var present []incoming
	defer func() {
		for _, item := range present {
			item.file.Close()
		}
	}()
	for _, field := range fields {
		file, _, err := r.FormFile(field.field)
		if errors.Is(err, http.ErrMissingFile) {
			continue
		}
		if err != nil {
			writeMapped(w, errRequest)
			return
		}
		present = append(present, incoming{platform: field.platform, file: file, expect: r.FormValue(field.sumField)})
	}
	if len(present) == 0 {
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
	created, err := lockDevRelease(ctx, tx, version, notes)
	if err != nil {
		writeMapped(w, err)
		return
	}
	sums := map[string]string{}
	sizes := map[string]int64{}
	for _, part := range present {
		sum, size, err := writeArtifact(root, version, part.platform, part.file, part.expect)
		if err != nil {
			writeMapped(w, err)
			return
		}
		sums[part.platform] = sum
		sizes[part.platform] = size
	}
	if err := updateReleaseFiles(ctx, tx, version, notes, sums, sizes); err != nil {
		writeMapped(w, err)
		return
	}
	rel, err := loadRelease(ctx, tx, version)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]any{"release": rel})
}

func lockDevRelease(ctx context.Context, tx pgx.Tx, version, notes string) (bool, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM client_releases WHERE version = $1 FOR UPDATE`, version).Scan(&status)
	if err == pgx.ErrNoRows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO client_releases (version, status, notes) VALUES ($1, 'dev', $2)
		`, version, notes); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if status != "dev" {
		return false, errConflict
	}
	return false, nil
}

func updateReleaseFiles(ctx context.Context, tx pgx.Tx, version, notes string, sums map[string]string, sizes map[string]int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE client_releases SET
			linux_sha256 = CASE WHEN $2 <> '' THEN $2 ELSE linux_sha256 END,
			linux_size = CASE WHEN $3 > 0 THEN $3 ELSE linux_size END,
			windows_sha256 = CASE WHEN $4 <> '' THEN $4 ELSE windows_sha256 END,
			windows_size = CASE WHEN $5 > 0 THEN $5 ELSE windows_size END,
			macos_sha256 = CASE WHEN $6 <> '' THEN $6 ELSE macos_sha256 END,
			macos_size = CASE WHEN $7 > 0 THEN $7 ELSE macos_size END,
			notes = CASE WHEN $8 <> '' THEN $8 ELSE notes END
		WHERE version = $1 AND status = 'dev'
	`, version, sums["linux"], sizes["linux"], sums["windows"], sizes["windows"], sums["macos"], sizes["macos"], notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errConflict
	}
	return nil
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	ctx := r.Context()
	active, err := activeVersion(ctx, s.db)
	if err != nil {
		writeMapped(w, err)
		return
	}
	rows, err := s.db.Query(ctx, `
		SELECT version, status, linux_sha256, linux_size, windows_sha256, windows_size,
		       macos_sha256, macos_size, notes, created_at, published_at
		FROM client_releases
		ORDER BY created_at DESC, version
	`)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []clientRelease{}
	for rows.Next() {
		rel, err := scanRelease(rows)
		if err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, rel)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active_version": active, "releases": list})
}

func (s *Server) publishRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	version := r.PathValue("version")
	if !validReleaseVersion(version) {
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
	rel, err := loadRelease(ctx, tx, version)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if rel.LinuxSize <= 0 || rel.WindowsSize <= 0 || len(rel.LinuxSHA256) != 64 || len(rel.WindowsSHA256) != 64 {
		writeMapped(w, errConflict)
		return
	}
	root, err := releaseRoot()
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := artifactsPresent(root, version, "linux", "windows"); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE client_releases
		SET status = 'released',
		    published_at = COALESCE(published_at, now())
		WHERE version = $1
	`, version); err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ('active_client_version', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, version); err != nil {
		writeMapped(w, err)
		return
	}
	rel, err = loadRelease(ctx, tx, version)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active_version": version, "release": rel})
}

func (s *Server) deleteRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	version := r.PathValue("version")
	if !validReleaseVersion(version) {
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
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM client_releases WHERE version = $1 FOR UPDATE`, version).Scan(&status)
	if err == pgx.ErrNoRows {
		writeMapped(w, errNotFound)
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if status != "dev" {
		writeMapped(w, errConflict)
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM client_releases WHERE version = $1 AND status = 'dev'`, version); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	if root, err := releaseRoot(); err == nil {
		_ = removeReleaseDir(root, version)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) latestRelease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	active, err := activeVersion(ctx, s.db)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if active == "" {
		writeMapped(w, errNotFound)
		return
	}
	rel, err := loadRelease(ctx, s.db, active)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if rel.Status != "released" {
		writeMapped(w, errNotFound)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (s *Server) downloadRelease(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	platform := r.PathValue("platform")
	if !validReleaseVersion(version) {
		writeMapped(w, errNotFound)
		return
	}
	name, err := artifactName(platform)
	if err != nil {
		writeMapped(w, errNotFound)
		return
	}
	ctx := r.Context()
	rel, err := loadRelease(ctx, s.db, version)
	if err != nil {
		writeMapped(w, errNotFound)
		return
	}
	if rel.Status != "released" {
		writeMapped(w, errNotFound)
		return
	}
	want, size := releaseHash(rel, platform)
	if want == "" || size <= 0 {
		writeMapped(w, errNotFound)
		return
	}
	root, err := releaseRoot()
	if err != nil {
		writeMapped(w, err)
		return
	}
	file, err := openArtifact(root, version, platform)
	if err != nil {
		writeMapped(w, errNotFound)
		return
	}
	defer file.Close()
	sum, n, err := hashFile(file)
	if err != nil || sum != want || n != size {
		writeJSON(w, http.StatusInternalServerError, errJSON("ошибка"))
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeMapped(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-SHA256", sum)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeContent(w, r, name, time.Time{}, file)
}

func releaseHash(rel clientRelease, platform string) (string, int64) {
	switch platform {
	case "linux":
		return rel.LinuxSHA256, rel.LinuxSize
	case "windows":
		return rel.WindowsSHA256, rel.WindowsSize
	case "macos":
		return rel.MacosSHA256, rel.MacosSize
	default:
		return "", 0
	}
}

func activeVersion(ctx context.Context, q planQuerier) (string, error) {
	var value string
	err := q.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = 'active_client_version'`).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return value, err
}

func loadRelease(ctx context.Context, q planQuerier, version string) (clientRelease, error) {
	row := q.QueryRow(ctx, `
		SELECT version, status, linux_sha256, linux_size, windows_sha256, windows_size,
		       macos_sha256, macos_size, notes, created_at, published_at
		FROM client_releases WHERE version = $1
	`, version)
	rel, err := scanRelease(row)
	if err == pgx.ErrNoRows {
		return clientRelease{}, errNotFound
	}
	return rel, err
}

func scanRelease(row pgx.Row) (clientRelease, error) {
	var rel clientRelease
	err := row.Scan(
		&rel.Version, &rel.Status, &rel.LinuxSHA256, &rel.LinuxSize, &rel.WindowsSHA256, &rel.WindowsSize,
		&rel.MacosSHA256, &rel.MacosSize, &rel.Notes, &rel.CreatedAt, &rel.PublishedAt,
	)
	if err != nil {
		return clientRelease{}, err
	}
	rel.CreatedAt = rel.CreatedAt.UTC()
	if rel.PublishedAt != nil {
		t := rel.PublishedAt.UTC()
		rel.PublishedAt = &t
	}
	return rel, nil
}

func releaseRoot() (string, error) {
	dir := strings.TrimSpace(os.Getenv("RELEASES_DIR"))
	if dir == "" {
		return "", fmt.Errorf("не задан RELEASES_DIR")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return real, nil
}

func writeArtifact(root, version, platform string, r io.Reader, expect string) (string, int64, error) {
	dir, err := releaseDir(root, version)
	if err != nil {
		return "", 0, err
	}
	name, err := artifactName(platform)
	if err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(dir, ".part-*")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(r, maxArtifact+1))
	if err != nil {
		return "", 0, err
	}
	if n > maxArtifact || n == 0 {
		return "", 0, errRequest
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	expect = strings.ToLower(strings.TrimSpace(expect))
	if expect != "" && !shaEqual(sum, expect) {
		return "", 0, errRequest
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	dest := filepath.Join(dir, name)
	if err := pathInside(root, dest); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", 0, err
	}
	if err := os.Chmod(dest, 0o644); err != nil {
		return "", 0, err
	}
	success = true
	return sum, n, nil
}

func releaseDir(root, version string) (string, error) {
	if !validReleaseVersion(version) {
		return "", errRequest
	}
	dir := filepath.Join(root, version)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if err := pathInside(root, real); err != nil {
		return "", err
	}
	return real, nil
}

func openArtifact(root, version, platform string) (*os.File, error) {
	dir, err := releaseDir(root, version)
	if err != nil {
		return nil, err
	}
	name, err := artifactName(platform)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errNotFound
	}
	return os.Open(path)
}

func artifactsPresent(root, version string, platforms ...string) error {
	for _, platform := range platforms {
		file, err := openArtifact(root, version, platform)
		if err != nil {
			return errConflict
		}
		info, err := file.Stat()
		file.Close()
		if err != nil || info.Size() <= 0 {
			return errConflict
		}
	}
	return nil
}

func removeReleaseDir(root, version string) error {
	dir, err := releaseDir(root, version)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func hashFile(file *os.File) (string, int64, error) {
	hash := sha256.New()
	n, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), n, nil
}

func pathInside(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return errRequest
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errRequest
	}
	return nil
}

func shaEqual(got, want string) bool {
	if len(got) != len(want) || len(got) != 64 {
		return false
	}
	return subtleCompare(got, want)
}

func subtleCompare(a, b string) bool {
	diff := 0
	for i := 0; i < len(a); i++ {
		diff |= int(a[i] ^ b[i])
	}
	return diff == 0
}
