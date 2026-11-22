package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	sampleINI   = "BibleName=Пример\nBibleShortName=EX\nChapterSign=<h4>\nVerseSign=<p>\nPathName=grief.htm\nFullName=Истории\nShortName=Ист\nChapterQty=1\n"
	sampleVerse = "verse-1"
	sampleHash  = "c004ac1417fa0542735fca37e8e3e1517583e5c4e505ad0f5d19c7f0cb3770a0"
)

func TestBibleIdentityStable(t *testing.T) {
	dir := t.TempDir()
	writeSampleModule(t, dir, false)
	ident, err := bibleIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ident.ContentHash != sampleHash || ident.ShortName != "EX" || ident.Title != "Пример" {
		t.Fatalf("%+v", ident)
	}
	if len(ident.Files) != 2 || ident.Files[0] != "bibleqt.ini" || ident.Files[1] != "grief.htm" {
		t.Fatalf("файлы %+v", ident.Files)
	}
	noisy := t.TempDir()
	writeSampleModule(t, noisy, true)
	again, err := bibleIdentity(noisy)
	if err != nil {
		t.Fatal(err)
	}
	if again.ContentHash != ident.ContentHash {
		t.Fatalf("BibleId изменил хеш %s", again.ContentHash)
	}
	changed := t.TempDir()
	writeSampleModule(t, changed, false)
	if err := os.WriteFile(filepath.Join(changed, "grief.htm"), []byte("verse-2"), 0o644); err != nil {
		t.Fatal(err)
	}
	other, err := bibleIdentity(changed)
	if err != nil {
		t.Fatal(err)
	}
	if other.ContentHash == ident.ContentHash {
		t.Fatal("другой текст книги дал тот же хеш")
	}
	if _, err := safeModuleRel("../secret.txt"); err == nil {
		t.Fatal("путь наружу принят")
	}
}

func TestBibleModuleUploadOnce(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "bible-modules")
	if err := os.Mkdir(store, 0o750); err != nil {
		t.Fatal(err)
	}
	srv := newBibleServer(t, store, 0)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)

	code, raw := api(t, srv, "", http.MethodPost, "/api/modules/exists", map[string]any{
		"content_hash": sampleHash, "short_name": "EX", "title": "Пример",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("без входа %d %s", code, raw)
	}
	code, raw = uploadSample(t, srv, "", sampleHash)
	if code != http.StatusUnauthorized {
		t.Fatalf("заливка без входа %d %s", code, raw)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules/exists", map[string]any{
		"content_hash": sampleHash, "short_name": "EX", "title": "Пример",
	})
	if code != http.StatusOK {
		t.Fatalf("exists %d %s", code, raw)
	}
	var missing moduleSyncView
	mustJSON(t, raw, &missing)
	if missing.Exists || missing.ID != "" {
		t.Fatalf("пустое существование %+v", missing)
	}

	code, raw = uploadSample(t, srv, owner.Token, sampleHash)
	if code != http.StatusOK {
		t.Fatalf("заливка %d %s", code, raw)
	}
	var uploaded moduleSyncView
	mustJSON(t, raw, &uploaded)
	if !uploaded.Exists || !uploaded.Uploaded || uploaded.ID == "" || uploaded.ContentHash != sampleHash {
		t.Fatalf("ответ %+v", uploaded)
	}
	stored := filepath.Join(store, uploaded.ID, "grief.htm")
	got, err := os.ReadFile(stored)
	if err != nil || string(got) != sampleVerse {
		t.Fatalf("файл модуля %v %q", err, got)
	}
	outside := filepath.Join(root, "secret.txt")
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("файл оказался вне каталога модулей")
	}

	code, raw = api(t, srv, other.Token, http.MethodPost, "/api/modules/exists", map[string]any{
		"content_hash": sampleHash, "short_name": "EX", "title": "Пример",
	})
	if code != http.StatusOK {
		t.Fatalf("exists второго %d %s", code, raw)
	}
	var found moduleSyncView
	mustJSON(t, raw, &found)
	if !found.Exists || found.Uploaded || found.ID != uploaded.ID {
		t.Fatalf("второй поиск %+v", found)
	}

	code, raw = uploadSample(t, srv, other.Token, sampleHash)
	if code != http.StatusOK {
		t.Fatalf("повтор %d %s", code, raw)
	}
	var again moduleSyncView
	mustJSON(t, raw, &again)
	if !again.Exists || again.Uploaded || again.ID != uploaded.ID {
		t.Fatalf("повторная заливка %+v", again)
	}
	var modules int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM modules WHERE content_hash = $1`, sampleHash).Scan(&modules); err != nil {
		t.Fatal(err)
	}
	if modules != 1 {
		t.Fatalf("строк модуля %d", modules)
	}
	entries, err := os.ReadDir(store)
	if err != nil {
		t.Fatal(err)
	}
	kept := 0
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == uploaded.ID {
			kept++
		}
		if entry.IsDir() && len(entry.Name()) > 9 && entry.Name()[:9] == "incoming-" {
			t.Fatalf("временный каталог остался %s", entry.Name())
		}
	}
	if kept != 1 {
		t.Fatalf("каталогов %d", kept)
	}

	code, raw = api(t, srv, other.Token, http.MethodPost, "/api/modules/exists", map[string]any{
		"content_hash": sampleHash, "short_name": "OTHER", "title": "Пример",
	})
	if code != http.StatusOK {
		t.Fatalf("чужое имя %d %s", code, raw)
	}
	var wrong moduleSyncView
	mustJSON(t, raw, &wrong)
	if wrong.Exists {
		t.Fatalf("чужое имя принято %+v", wrong)
	}
}

func TestBibleModuleUploadRejects(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "bible-modules")
	if err := os.Mkdir(store, 0o750); err != nil {
		t.Fatal(err)
	}
	srv := newBibleServer(t, store, 0)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)

	files := sampleFiles()
	files["visiobible.idx"] = []byte("idx")
	code, raw := uploadFiles(t, srv, owner.Token, sampleHash, "EX", "Пример", files)
	if code != http.StatusBadRequest {
		t.Fatalf("индекс %d %s", code, raw)
	}
	code, raw = uploadFiles(t, srv, owner.Token, sampleHash, "EX", "Пример", map[string][]byte{
		"bibleqt.ini":   []byte(sampleINI),
		"../secret.txt": []byte("no"),
		"grief.htm":     []byte(sampleVerse),
	})
	if code != http.StatusBadRequest {
		t.Fatalf("путь %d %s", code, raw)
	}
	if _, err := os.Stat(filepath.Join(root, "secret.txt")); !os.IsNotExist(err) {
		t.Fatal("секрет записан")
	}
	code, raw = uploadSample(t, srv, owner.Token, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if code != http.StatusBadRequest {
		t.Fatalf("чужой хеш %d %s", code, raw)
	}
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM modules`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("после отказа осталась строка %d", n)
	}

	closed := newBibleServer(t, "", 0)
	closedUser := registerUser(t, closed, "owner@example.com", "secret-pass", nil)
	code, raw = uploadSample(t, closed, closedUser.Token, sampleHash)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("без каталога %d %s", code, raw)
	}

	limited := newBibleServer(t, store, 80)
	limitedUser := registerUser(t, limited, "owner@example.com", "secret-pass", nil)
	code, raw = uploadSample(t, limited, limitedUser.Token, sampleHash)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("размер %d %s", code, raw)
	}
}

// Сайт шлёт файлы папки без хеша: имя и хеш считает сервер, лишнее не хранится.
func TestBibleModuleUploadFromSite(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "bible-modules")
	if err := os.Mkdir(store, 0o750); err != nil {
		t.Fatal(err)
	}
	srv := newBibleServer(t, store, 0)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)

	files := sampleFiles()
	files["notes.txt"] = []byte("лишнее")
	code, raw := uploadFiles(t, srv, owner.Token, "", "EX", "", files)
	if code != http.StatusBadRequest {
		t.Fatalf("неполные поля %d %s", code, raw)
	}
	code, raw = uploadFiles(t, srv, owner.Token, "", "", "", files)
	if code != http.StatusOK {
		t.Fatalf("с сайта %d %s", code, raw)
	}
	var view moduleSyncView
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	if !view.Uploaded || view.ContentHash != sampleHash || view.ShortName != "EX" || view.Title != "Пример" {
		t.Fatalf("%+v", view)
	}
	if _, err := os.Stat(filepath.Join(store, view.ID, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("лишний файл сохранён")
	}
	if _, err := os.Stat(filepath.Join(store, view.ID, "grief.htm")); err != nil {
		t.Fatal(err)
	}
}

func TestBibleModuleDeleteRemovesFiles(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	store := t.TempDir()
	srv := newBibleServer(t, store, 0)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	code, raw := uploadSample(t, srv, admin.Token, sampleHash)
	if code != http.StatusOK {
		t.Fatalf("заливка %d %s", code, raw)
	}
	var uploaded moduleSyncView
	mustJSON(t, raw, &uploaded)
	code, raw = api(t, srv, admin.Token, http.MethodDelete, "/api/admin/modules/"+uploaded.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление %d %s", code, raw)
	}
	if _, err := os.Stat(filepath.Join(store, uploaded.ID)); !os.IsNotExist(err) {
		t.Fatal("каталог модуля остался")
	}
}

func newBibleServer(t *testing.T, dir string, limit int64) *httptest.Server {
	t.Helper()
	reset(t)
	api := New(testPool)
	api.SetModuleDir(dir)
	if limit > 0 {
		api.SetModuleBodyLimit(limit)
	}
	srv := httptest.NewServer(api.Handler())
	srv.Client().Timeout = 20 * time.Second
	t.Cleanup(srv.Close)
	return srv
}

func writeSampleModule(t *testing.T, dir string, noisy bool) {
	t.Helper()
	ini := sampleINI
	if noisy {
		ini = "BibleName=Пример\nBibleShortName=EX\nBibleId=999\nChapterSign=<h4>\nVerseSign=<p>\nPathName=grief.htm\nFullName=Истории\nBookHash=deadbeef\nShortName=Ист\nChapterQty=1\n"
		if err := os.WriteFile(filepath.Join(dir, "visiobible.idx"), []byte("idx"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "bibleqt.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grief.htm"), []byte(sampleVerse), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sampleFiles() map[string][]byte {
	return map[string][]byte{
		"bibleqt.ini": []byte(sampleINI),
		"grief.htm":   []byte(sampleVerse),
	}
}

func uploadSample(t *testing.T, srv *httptest.Server, token, hash string) (int, []byte) {
	t.Helper()
	return uploadFiles(t, srv, token, hash, "EX", "Пример", sampleFiles())
}

func uploadFiles(t *testing.T, srv *httptest.Server, token, hash, shortName, title string, files map[string][]byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("content_hash", hash); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("short_name", shortName); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("title", title); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		part, err := writer.CreateFormFile("file", name)
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
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/modules/upload", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
