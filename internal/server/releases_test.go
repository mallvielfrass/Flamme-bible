package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELEASES_DIR", dir)
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	user := registerUser(t, srv, "user@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)

	linux := []byte("linux-binary")
	windows := []byte("windows-binary")
	macos := []byte("macos-binary")
	code, raw := uploadReleaseFiles(t, srv, user.Token, "1.0.0", map[string][]byte{"linux_file": linux, "windows_file": windows}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("не админ %d %s", code, raw)
	}
	code, raw = uploadReleaseFiles(t, srv, admin.Token, "../1.0.0", map[string][]byte{"linux_file": linux}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("путь %d %s", code, raw)
	}
	bad := map[string]string{"linux_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	code, raw = uploadReleaseFiles(t, srv, admin.Token, "0.0.1", map[string][]byte{"linux_file": linux}, bad)
	if code != http.StatusBadRequest {
		t.Fatalf("чужой хеш %d %s", code, raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "0.0.1", "flammebible-linux")); !os.IsNotExist(err) {
		t.Fatal("файл записан при неверном хеше")
	}

	code, raw = uploadReleaseFiles(t, srv, admin.Token, "1.0.0", map[string][]byte{
		"linux_file": linux, "windows_file": windows, "macos_file": macos,
	}, map[string]string{"linux_sha256": shaHex(linux)})
	if code != http.StatusCreated {
		t.Fatalf("загрузка %d %s", code, raw)
	}
	var uploaded struct {
		Release clientRelease `json:"release"`
	}
	mustJSON(t, raw, &uploaded)
	if uploaded.Release.Status != "dev" || uploaded.Release.LinuxSHA256 != shaHex(linux) || uploaded.Release.MacosSize != int64(len(macos)) {
		t.Fatalf("релиз %+v", uploaded.Release)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/releases/1.0.0/publish", nil)
	if code != http.StatusOK {
		t.Fatalf("publish %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/releases/latest", nil)
	if code != http.StatusOK {
		t.Fatalf("latest %d %s", code, raw)
	}
	var latest clientRelease
	mustJSON(t, raw, &latest)
	if latest.Version != "1.0.0" || latest.Status != "released" || latest.WindowsSHA256 != shaHex(windows) {
		t.Fatalf("latest %+v", latest)
	}
	status, body, header := downloadRelease(t, srv, "/api/releases/1.0.0/download/linux")
	if status != http.StatusOK || !bytes.Equal(body, linux) || header != shaHex(linux) {
		t.Fatalf("скачивание %d %s header %s", status, body, header)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/releases/1.0.0/download/dev", nil)
	if code != http.StatusNotFound {
		t.Fatalf("платформа %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/releases/1.0.0", nil)
	if code != http.StatusConflict {
		t.Fatalf("удаление выпущенного %d %s", code, raw)
	}
	code, raw = uploadReleaseFiles(t, srv, admin.Token, "1.0.0", map[string][]byte{"linux_file": []byte("newer")}, nil)
	if code != http.StatusConflict {
		t.Fatalf("перезапись выпущенного %d %s", code, raw)
	}

	code, raw = uploadReleaseFiles(t, srv, admin.Token, "1.1.0", map[string][]byte{"linux_file": linux}, nil)
	if code != http.StatusCreated {
		t.Fatalf("dev %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/releases/1.1.0/publish", nil)
	if code != http.StatusConflict {
		t.Fatalf("без windows %d %s", code, raw)
	}
	code, raw = uploadReleaseFiles(t, srv, admin.Token, "1.1.0", map[string][]byte{"windows_file": windows}, nil)
	if code != http.StatusOK {
		t.Fatalf("дозагрузка %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/releases/1.1.0/publish", nil)
	if code != http.StatusOK {
		t.Fatalf("publish 1.1 %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/releases/latest", nil)
	if code != http.StatusOK {
		t.Fatalf("новый latest %d %s", code, raw)
	}
	mustJSON(t, raw, &latest)
	if latest.Version != "1.1.0" {
		t.Fatalf("активная %s", latest.Version)
	}
	status, body, _ = downloadRelease(t, srv, "/api/releases/1.0.0/download/macos")
	if status != http.StatusOK || !bytes.Equal(body, macos) {
		t.Fatalf("старый macos %d %s", status, body)
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "9.9.9")); err != nil {
		t.Fatal(err)
	}
	code, raw = uploadReleaseFiles(t, srv, admin.Token, "9.9.9", map[string][]byte{"linux_file": linux}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("symlink %d %s", code, raw)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("файл вышел из каталога релизов: %+v", entries)
	}

	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/releases/1.1.0", nil)
	if code != http.StatusConflict {
		t.Fatalf("1.1 уже выпущен %d %s", code, raw)
	}
	code, raw = uploadReleaseFiles(t, srv, admin.Token, "2.0.0-dev", map[string][]byte{"linux_file": linux}, nil)
	if code != http.StatusCreated {
		t.Fatalf("черновик %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/releases/2.0.0-dev", nil)
	if code != http.StatusOK {
		t.Fatalf("удаление dev %d %s", code, raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "2.0.0-dev")); !os.IsNotExist(err) {
		t.Fatal("каталог dev остался")
	}
	code, raw = api(t, srv, "", http.MethodGet, "/api/releases/2.0.0-dev/download/linux", nil)
	if code != http.StatusNotFound {
		t.Fatalf("скачивание dev %d %s", code, raw)
	}
}

func shaHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func uploadReleaseFiles(t *testing.T, srv *httptest.Server, token, version string, files map[string][]byte, fields map[string]string) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("version", version); err != nil {
		t.Fatal(err)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range files {
		part, err := writer.CreateFormFile(name, "bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/releases", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, raw
}

func TestDraftDownloadAndNotes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELEASES_DIR", dir)
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	user := registerUser(t, srv, "user@example.com", "secret-pass", nil)
	linux := []byte("draft-linux")
	code, raw := uploadReleaseFiles(t, srv, admin.Token, "1.2.0", map[string][]byte{"linux_file": linux}, nil)
	if code != http.StatusCreated {
		t.Fatalf("черновик %d %s", code, raw)
	}
	status, _, _ := downloadRelease(t, srv, "/api/releases/1.2.0/download/linux")
	if status != http.StatusNotFound {
		t.Fatalf("публичное скачивание черновика %d", status)
	}
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/admin/releases/1.2.0/download/linux", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+user.Token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("пользователь качает черновик %d", res.StatusCode)
	}
	req, err = http.NewRequest(http.MethodGet, srv.URL+"/api/admin/releases/1.2.0/download/linux", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	res, err = srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, linux) || res.Header.Get("X-Content-SHA256") != shaHex(linux) {
		t.Fatalf("черновик админу %d %s", res.StatusCode, body)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPatch, "/api/admin/releases/1.2.0", map[string]any{"notes": "исправили шрифт"})
	if code != http.StatusOK {
		t.Fatalf("заметки %d %s", code, raw)
	}
	var patched struct {
		Release clientRelease `json:"release"`
	}
	mustJSON(t, raw, &patched)
	if patched.Release.Notes != "исправили шрифт" || patched.Release.Status != "dev" {
		t.Fatalf("после правки %+v", patched.Release)
	}
	code, raw = api(t, srv, user.Token, http.MethodPatch, "/api/admin/releases/1.2.0", map[string]any{"notes": "нет"})
	if code != http.StatusForbidden {
		t.Fatalf("чужие заметки %d %s", code, raw)
	}
}

func downloadRelease(t *testing.T, srv *httptest.Server, path string) (int, []byte, string) {
	t.Helper()
	res, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, raw, res.Header.Get("X-Content-SHA256")
}
