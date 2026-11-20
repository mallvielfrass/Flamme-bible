package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flammebible/internal/server"
	"flammebible/internal/vbm"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	poolVal     *pgxpool.Pool
	poolCleanup func()
	poolErr     error
)

func TestMain(m *testing.M) {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		poolVal, poolCleanup, poolErr = server.OpenTestPool(context.Background(), url)
	}
	code := m.Run()
	if poolCleanup != nil {
		poolCleanup()
	}
	os.Exit(code)
}

func dbPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if poolErr != nil {
		t.Fatal(poolErr)
	}
	if poolVal == nil {
		t.Fatal("DATABASE_URL не задан")
	}
	return poolVal
}

func resetDB(t *testing.T) {
	t.Helper()
	_, err := dbPool(t).Exec(context.Background(), `
		TRUNCATE plan_items, plans, modules, client_releases, app_settings,
		         song_hides, conflicts, operation_results, change_log, song_order, songs, songbooks,
		         invites, session_tokens, session_families, devices, memberships, sync_counters,
		         dimensions, users
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func startTLS(t *testing.T) *httptest.Server {
	t.Helper()
	resetDB(t)
	srv := httptest.NewTLSServer(server.New(dbPool(t)).Handler())
	t.Cleanup(srv.Close)
	raw := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAMMEBIBLE_CA", path)
	return srv
}

func capture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	callErr := fn()
	_ = w.Close()
	os.Stdout = old
	<-done
	return buf.String(), callErr
}

func apiJSON(t *testing.T, srv *httptest.Server, token, method, path string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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

func registerAccount(t *testing.T, srv *httptest.Server, email string) {
	t.Helper()
	code, raw := apiJSON(t, srv, "", http.MethodPost, "/register", map[string]string{
		"email": email, "password": "secret-pass",
	})
	if code != http.StatusOK {
		t.Fatalf("register %d %s", code, raw)
	}
}

func loginCLI(t *testing.T, srv *httptest.Server, cfg, email string) Session {
	t.Helper()
	t.Setenv("FLAMMEBIBLE_CONFIG", cfg)
	out, err := capture(t, func() error {
		return run([]string{"login", "--server", srv.URL, "--email", email, "--password", "secret-pass"})
	})
	if err != nil {
		t.Fatalf("login: %v %s", err, out)
	}
	sess, err := loadSession()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(cfg, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("режим %o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(filepath.Join(cfg, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-pass") {
		t.Fatal("пароль записан в файл")
	}
	return sess
}

func useSpace(t *testing.T, cfg, id string) {
	t.Helper()
	t.Setenv("FLAMMEBIBLE_CONFIG", cfg)
	out, err := capture(t, func() error {
		return run([]string{"space", id})
	})
	if err != nil {
		t.Fatalf("space: %v %s", err, out)
	}
}

func syncRemote(t *testing.T, cfg, db, path string) (string, error) {
	t.Helper()
	t.Setenv("FLAMMEBIBLE_CONFIG", cfg)
	return capture(t, func() error {
		return run([]string{"sync", "--db", db, "--remote", "--apply", path})
	})
}

func TestSettingsPath(t *testing.T) {
	got, err := settingsPath("windows", "", `C:\Users\me\AppData\Roaming`)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(`C:\Users\me\AppData\Roaming`, "flammebible")
	if got != want {
		t.Fatalf("%s", got)
	}
	dir := t.TempDir()
	got, err = settingsPath("windows", dir, "")
	if err != nil || got != dir {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := settingsPath("windows", "", ""); err == nil {
		t.Fatal("пустой AppData")
	}
}

func songTitles(t *testing.T, path string) []string {
	t.Helper()
	book, err := vbm.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return titlesOf(book.Songs)
}

func hasTitle(titles []string, title string) bool {
	for _, item := range titles {
		if item == title {
			return true
		}
	}
	return false
}

func TestTwoCLIProcessesExchangeSong(t *testing.T) {
	srv := startTLS(t)
	registerAccount(t, srv, "a@example.com")
	cfgA := t.TempDir()
	cfgB := t.TempDir()
	sess := loginCLI(t, srv, cfgA, "a@example.com")
	personal := sess.DimensionID
	loginCLI(t, srv, cfgB, "a@example.com")
	code, raw := apiJSON(t, srv, sess.Token, http.MethodPost, "/dimensions", map[string]string{"name": "Община"})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var community struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &community); err != nil {
		t.Fatal(err)
	}
	useSpace(t, cfgA, community.ID)
	useSpace(t, cfgB, community.ID)

	dirA := t.TempDir()
	dirB := t.TempDir()
	pathA := writeSongs(t, dirA, "book.vbm", []vbm.Song{songWith("Общая", "база", fbidA, 0)})
	rawA, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	pathB := filepath.Join(dirB, "book.vbm")
	if err := os.WriteFile(pathB, rawA, 0o644); err != nil {
		t.Fatal(err)
	}
	dbA := filepath.Join(dirA, "a.db")
	dbB := filepath.Join(dirB, "b.db")

	if out, err := syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("первая синхронизация: %v %s", err, out)
	}
	if out, err := syncRemote(t, cfgB, dbB, pathB); err != nil {
		t.Fatalf("вторая копия: %v %s", err, out)
	}
	pathA = writeSongs(t, dirA, "book.vbm", []vbm.Song{
		songWith("Общая", "база", fbidA, 0),
		songWith("Песня А", "текст а", fbidB, 0),
	})
	if out, err := syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("отправка А: %v %s", err, out)
	}
	if out, err := syncRemote(t, cfgB, dbB, pathB); err != nil {
		t.Fatalf("получение Б: %v %s", err, out)
	}
	if !hasTitle(songTitles(t, pathB), "Песня А") {
		t.Fatalf("Б не получил песню: %v", songTitles(t, pathB))
	}
	pathB = writeSongs(t, dirB, "book.vbm", []vbm.Song{
		songWith("Общая", "база", fbidA, 0),
		songWith("Песня А", "текст а", fbidB, 0),
		songWith("Песня Б", "текст б", fbidC, 0),
	})
	if out, err := syncRemote(t, cfgB, dbB, pathB); err != nil {
		t.Fatalf("отправка Б: %v %s", err, out)
	}
	if out, err := syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("получение А: %v %s", err, out)
	}
	gotA := songTitles(t, pathA)
	if !hasTitle(gotA, "Песня Б") || !hasTitle(gotA, "Песня А") {
		t.Fatalf("А не получил песню: %v", gotA)
	}
	code, raw = apiJSON(t, srv, sess.Token, http.MethodGet, "/dimensions/"+community.ID+"/songbooks", nil)
	if code != http.StatusOK || !strings.Contains(string(raw), "id") {
		t.Fatalf("сборник общины %d %s", code, raw)
	}
	code, raw = apiJSON(t, srv, sess.Token, http.MethodGet, "/dimensions/"+personal+"/songbooks", nil)
	if code != http.StatusOK || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("личное пространство %d %s", code, raw)
	}
}

func partText(t *testing.T, path, fbid string) string {
	t.Helper()
	book, err := vbm.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, song := range book.Songs {
		id, ok, err := songFBID(song.Properties)
		if err != nil {
			t.Fatal(err)
		}
		if ok && id == fbid {
			if len(song.Parts) == 0 {
				return ""
			}
			return song.Parts[0].Text
		}
	}
	return ""
}

func prepareConflict(t *testing.T) (srv *httptest.Server, cfgA, cfgB, dbA, dbB, pathA, pathB string) {
	t.Helper()
	srv = startTLS(t)
	registerAccount(t, srv, "a@example.com")
	cfgA = t.TempDir()
	cfgB = t.TempDir()
	sess := loginCLI(t, srv, cfgA, "a@example.com")
	loginCLI(t, srv, cfgB, "a@example.com")
	useSpace(t, cfgA, sess.DimensionID)
	useSpace(t, cfgB, sess.DimensionID)
	dirA := t.TempDir()
	dirB := t.TempDir()
	pathA = writeSongs(t, dirA, "book.vbm", []vbm.Song{songWith("Общая", "база", fbidA, 0)})
	raw, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	pathB = filepath.Join(dirB, "book.vbm")
	if err := os.WriteFile(pathB, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	dbA = filepath.Join(dirA, "a.db")
	dbB = filepath.Join(dirB, "b.db")
	if out, err := syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("А: %v %s", err, out)
	}
	if out, err := syncRemote(t, cfgB, dbB, pathB); err != nil {
		t.Fatalf("Б: %v %s", err, out)
	}
	return srv, cfgA, cfgB, dbA, dbB, pathA, pathB
}

func TestConflictBothOutcomes(t *testing.T) {
	_, cfgA, cfgB, dbA, dbB, pathA, pathB := prepareConflict(t)
	dirA := filepath.Dir(pathA)
	dirB := filepath.Dir(pathB)
	pathA = writeSongs(t, dirA, "book.vbm", []vbm.Song{songWith("Общая", "правка-а", fbidA, 0)})
	pathB = writeSongs(t, dirB, "book.vbm", []vbm.Song{songWith("Общая", "правка-б", fbidA, 0)})
	if out, err := syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("правка А: %v %s", err, out)
	}
	before, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	out, err := syncRemote(t, cfgB, dbB, pathB)
	if !errors.Is(err, errUnresolved) || !strings.Contains(out, "правка-а") || !strings.Contains(out, "правка-б") {
		t.Fatalf("конфликт err=%v out=%s", err, out)
	}
	after, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("локальный текст затёрт до выбора")
	}

	t.Setenv("FLAMMEBIBLE_CONFIG", cfgB)
	out, err = capture(t, func() error {
		return run([]string{"resolve", "--db", dbB, "--fbid", fbidA, "--take", "server", "--apply", pathB})
	})
	if err != nil {
		t.Fatalf("сервер: %v %s", err, out)
	}
	if partText(t, pathB, fbidA) != "правка-а" {
		t.Fatalf("после сервера %s", partText(t, pathB, fbidA))
	}
	pathB = writeSongs(t, dirB, "book.vbm", []vbm.Song{songWith("Общая", "поверх", fbidA, 0)})
	if out, err = syncRemote(t, cfgB, dbB, pathB); err != nil {
		t.Fatalf("поверх: %v %s", err, out)
	}
	if out, err = syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("А принял поверх: %v %s", err, out)
	}
	if partText(t, pathA, fbidA) != "поверх" {
		t.Fatalf("новая ревизия %s", partText(t, pathA, fbidA))
	}

	_, cfgA, cfgB, dbA, dbB, pathA, pathB = prepareConflict(t)
	dirA = filepath.Dir(pathA)
	dirB = filepath.Dir(pathB)
	pathA = writeSongs(t, dirA, "book.vbm", []vbm.Song{songWith("Общая", "серверная", fbidA, 0)})
	pathB = writeSongs(t, dirB, "book.vbm", []vbm.Song{songWith("Общая", "локальная", fbidA, 0)})
	if out, err = syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("серверная: %v %s", err, out)
	}
	out, err = syncRemote(t, cfgB, dbB, pathB)
	if !errors.Is(err, errUnresolved) {
		t.Fatalf("конфликт 2 err=%v %s", err, out)
	}
	before, err = os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if partText(t, pathB, fbidA) != "локальная" || !bytes.Equal(before, mustRead(t, pathB)) {
		t.Fatal("текст изменился")
	}
	t.Setenv("FLAMMEBIBLE_CONFIG", cfgB)
	out, err = capture(t, func() error {
		return run([]string{"resolve", "--db", dbB, "--fbid", fbidA, "--take", "local", "--apply", pathB})
	})
	if err != nil {
		t.Fatalf("local: %v %s", err, out)
	}
	book, err := vbm.ParseFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	var sawServer, sawLocal bool
	var localID string
	for _, song := range book.Songs {
		id, _, _ := songFBID(song.Properties)
		text := ""
		if len(song.Parts) > 0 {
			text = song.Parts[0].Text
		}
		if id == fbidA && text == "серверная" {
			sawServer = true
		}
		if id != fbidA && text == "локальная" {
			sawLocal = true
			localID = id
		}
	}
	if !sawServer || !sawLocal || localID == "" {
		t.Fatalf("исход local: %+v", book.Songs)
	}
	if out, err = syncRemote(t, cfgB, dbB, pathB); err != nil {
		t.Fatalf("push новой: %v %s", err, out)
	}
	if out, err = syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("А видит новую: %v %s", err, out)
	}
	if partText(t, pathA, fbidA) != "серверная" || partText(t, pathA, localID) != "локальная" {
		t.Fatalf("А: %v", songTitles(t, pathA))
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDeleteVersusEditDoesNotChoose(t *testing.T) {
	_, cfgA, cfgB, dbA, dbB, pathA, pathB := prepareConflict(t)
	dirA := filepath.Dir(pathA)
	dirB := filepath.Dir(pathB)
	pathA = writeSongs(t, dirA, "book.vbm", []vbm.Song{songWith("Общая", "сервер-правка", fbidA, 0)})
	if out, err := syncRemote(t, cfgA, dbA, pathA); err != nil {
		t.Fatalf("правка: %v %s", err, out)
	}
	pathB = writeSongs(t, dirB, "book.vbm", nil)
	before := mustRead(t, pathB)
	out, err := syncRemote(t, cfgB, dbB, pathB)
	if !errors.Is(err, errUnresolved) || !strings.Contains(out, "сервер-правка") || !strings.Contains(out, "удаление") {
		t.Fatalf("err=%v out=%s", err, out)
	}
	if !bytes.Equal(before, mustRead(t, pathB)) {
		t.Fatal("удаление или правка выбраны сами")
	}
	out, err = syncRemote(t, cfgB, dbB, pathB)
	if !errors.Is(err, errUnresolved) || !bytes.Equal(before, mustRead(t, pathB)) {
		t.Fatalf("повтор выбрал победителя err=%v out=%s", err, out)
	}
}

func headerJSON() map[string]any {
	return map[string]any{
		"format_version": "0x102",
		"flags":          0,
		"charset":        "0x4b0",
		"header_short":   "SB",
		"header_title":   "Songbook",
	}
}

func createServerBook(t *testing.T, srv *httptest.Server, token, dim, fbid, parent string) string {
	t.Helper()
	body := map[string]any{"fbid": fbid, "title": "Книга", "header": headerJSON()}
	if parent != "" {
		body["parent_id"] = parent
	}
	code, raw := apiJSON(t, srv, token, http.MethodPost, "/dimensions/"+dim+"/songbooks", body)
	if code != http.StatusOK {
		t.Fatalf("сборник %d %s", code, raw)
	}
	var view struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	return view.ID
}

func pushSong(t *testing.T, srv *httptest.Server, token, dim, book string, base int64, song songDTO, order []string) int64 {
	t.Helper()
	id, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	code, raw := apiJSON(t, srv, token, http.MethodPost, "/dimensions/"+dim+"/songbooks/"+book+"/push", map[string]any{
		"base_revision": base,
		"operations": []any{map[string]any{
			"operation_id": id,
			"song":         song,
			"order":        order,
		}},
	})
	if code != http.StatusOK {
		t.Fatalf("push %d %s", code, raw)
	}
	var resp struct {
		Results []opResult `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Status != "applied" {
		t.Fatalf("push %+v", resp.Results)
	}
	return resp.Results[0].Revision
}

func TestBuildPatchOnServer(t *testing.T) {
	srv := startTLS(t)
	registerAccount(t, srv, "a@example.com")
	cfg := t.TempDir()
	sess := loginCLI(t, srv, cfg, "a@example.com")
	useSpace(t, cfg, sess.DimensionID)
	parentID := createServerBook(t, srv, sess.Token, sess.DimensionID, "parent-book", "")
	var base int64
	base = pushSong(t, srv, sess.Token, sess.DimensionID, parentID, base, dtoSong("Первая", "текст а", fbidA, false), []string{fbidA})
	base = pushSong(t, srv, sess.Token, sess.DimensionID, parentID, base, dtoSong("Вторая", "текст б", fbidB, false), []string{fbidA, fbidB})
	patchID := createServerBook(t, srv, sess.Token, sess.DimensionID, "patch-book", parentID)
	base = pushSong(t, srv, sess.Token, sess.DimensionID, patchID, base, dtoSong("Добавленная", "текст в", fbidC, false), nil)
	dir := t.TempDir()
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{songWith("Черновик", "x", fbidA, 0)})
	db := filepath.Join(dir, "local.db")
	out, err := capture(t, func() error {
		return run([]string{"build", "--db", db, "--book", patchID, "--apply", path})
	})
	if err != nil {
		t.Fatalf("сборка: %v %s", err, out)
	}
	if strings.Join(songTitles(t, path), ",") != "Первая,Вторая,Добавленная" {
		t.Fatalf("песни %v", songTitles(t, path))
	}
	book, err := vbm.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, song := range book.Songs {
		if !strings.Contains(song.Properties, "$ID$="+itoa(i+1)) {
			t.Fatalf("id %s", song.Properties)
		}
	}
	base = pushSong(t, srv, sess.Token, sess.DimensionID, parentID, base, dtoSong("Первая", "текст а", fbidA, true), []string{fbidB})
	out, err = capture(t, func() error {
		return run([]string{"build", "--db", db, "--book", patchID, "--apply", path})
	})
	if err != nil {
		t.Fatalf("удаление без правки: %v %s", err, out)
	}
	if strings.Join(songTitles(t, path), ",") != "Вторая,Добавленная" {
		t.Fatalf("после удаления %v отчёт %s", songTitles(t, path), out)
	}
	kept := mustRead(t, path)
	base = pushSong(t, srv, sess.Token, sess.DimensionID, patchID, base, dtoSong("Правка", "новый", fbidB, false), nil)
	base = pushSong(t, srv, sess.Token, sess.DimensionID, parentID, base, dtoSong("Вторая", "текст б", fbidB, true), []string{fbidB})
	out, err = capture(t, func() error {
		return run([]string{"build", "--db", db, "--book", patchID, "--apply", path})
	})
	if !errors.Is(err, errChoose) || !strings.Contains(out, "вопрос") {
		t.Fatalf("вопрос err=%v out=%s", err, out)
	}
	if !bytes.Equal(kept, mustRead(t, path)) {
		t.Fatal("файл записан до ответа")
	}
	out, err = capture(t, func() error {
		return run([]string{"build", "--db", db, "--book", patchID, "--retain", fbidB, "--apply", path})
	})
	if err != nil {
		t.Fatalf("retain: %v %s", err, out)
	}
	if !hasTitle(songTitles(t, path), "Правка") || hasTitle(songTitles(t, path), "Первая") {
		t.Fatalf("retain songs %v", songTitles(t, path))
	}
	if partText(t, path, fbidB) != "новый" {
		t.Fatalf("текст %s", partText(t, path, fbidB))
	}
	out, err = capture(t, func() error {
		return run([]string{"build", "--db", db, "--book", patchID, "--apply", path})
	})
	if err != nil || strings.Contains(out, "вопрос") {
		t.Fatalf("повтор err=%v out=%s", err, out)
	}
	store, err := OpenStore(db)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := store.loadDecisions(patchID)
	store.Close()
	if err != nil || dec[fbidB] != "retain" {
		t.Fatalf("решение %v %v", dec, err)
	}
	stable := mustRead(t, path)
	_, err = dbPool(t).Exec(context.Background(), `UPDATE songbooks SET parent_id = $1 WHERE id = $2`, patchID, parentID)
	if err != nil {
		t.Fatal(err)
	}
	out, err = capture(t, func() error {
		return run([]string{"build", "--db", db, "--book", patchID, "--apply", path})
	})
	if !errors.Is(err, errCycle) {
		t.Fatalf("цикл err=%v out=%s", err, out)
	}
	if !bytes.Equal(stable, mustRead(t, path)) {
		t.Fatal("цикл заменил файл")
	}
	_ = base
}
