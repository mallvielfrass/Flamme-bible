package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flammebible/internal/vbm"
)

func TestSyncApplyReplacesCopyAndDryRunPrints(t *testing.T) {
	dir := t.TempDir()
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{songWith("Новая", "текст песни", "", 0)})
	vbi := []byte("vbi-index")
	if err := os.WriteFile(siblingVBI(path), vbi, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "local.db")
	store := openTempStore(t, dir)
	changes, err := store.Diff(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != "added" {
		t.Fatalf("ожидалась одна новая песня: %+v", changes)
	}

	oldOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := run([]string{"sync", "--db", dbPath, path})
	w.Close()
	os.Stdout = oldOut
	printed, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(string(printed), "добавлена "+changes[0].FBID) {
		t.Fatalf("сухой запуск не напечатал изменение: %s", printed)
	}
	afterDry, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterDry, before) {
		t.Fatal("запуск без применения изменил VBM")
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatal("сухой запуск создал резервную копию")
	}

	if _, err := store.Sync(path, true); err != nil {
		t.Fatal(err)
	}
	parsed, err := vbm.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.Songs[0].Properties, changes[0].FBID) {
		t.Fatalf("после замены нет FBID %s", changes[0].FBID)
	}
	gotVBI, err := os.ReadFile(siblingVBI(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotVBI, vbi) {
		t.Fatal("замена изменила VBI")
	}
	stage, err := store.Stage((func() string {
		b, err := store.Bind(path)
		if err != nil {
			t.Fatal(err)
		}
		return b.ID
	})())
	if err != nil {
		t.Fatal(err)
	}
	if stage != "done" {
		t.Fatalf("стадия %s", stage)
	}
}

func TestBrokenWriteRestoresVBMAndVBI(t *testing.T) {
	dir := t.TempDir()
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{songWith("Песня", "текст", fbidA, 0)})
	vbi := []byte("vbi-bytes")
	if err := os.WriteFile(siblingVBI(path), vbi, 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := backupModule(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("damaged-vbm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(siblingVBI(path), []byte("damaged-vbi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreModule(path); err != nil {
		t.Fatal(err)
	}
	if err := replacePayload(path, []byte("not-a-vbm")); err == nil {
		t.Fatal("битая запись принята")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, orig) {
		t.Fatal("битая запись не вернула прежние байты VBM")
	}
	gotVBI, err := os.ReadFile(siblingVBI(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotVBI, vbi) {
		t.Fatal("битая запись не вернула прежние байты VBI")
	}

	grouped := writeGrouped(t, dir)
	groupedVBI := []byte("grouped-vbi")
	if err := os.WriteFile(siblingVBI(grouped), groupedVBI, 0o644); err != nil {
		t.Fatal(err)
	}
	beforeGroup, err := os.ReadFile(grouped)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "group-db"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := openTempStore(t, filepath.Join(dir, "group-db"))
	if _, err := store.Sync(grouped, true); err == nil {
		t.Fatal("файл с группами заменён")
	}
	afterGroup, err := os.ReadFile(grouped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterGroup, beforeGroup) {
		t.Fatal("отказ записи с группами изменил VBM")
	}
	afterVBI, err := os.ReadFile(siblingVBI(grouped))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterVBI, groupedVBI) {
		t.Fatal("отказ записи с группами изменил VBI")
	}
}

func TestBindingFollowsLowercaseRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "SongBook.vbm")
	writeSynthBook(t, src, []vbm.Song{songWith("Тестовая песня", "тестовый куплет", fbidA, 0)})
	store := openTempStore(t, dir)
	first, err := store.Bind(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "songbook.vbm")
	if err := os.Rename(src, dst); err != nil {
		t.Fatal(err)
	}
	again, err := store.Bind(dst)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatalf("после переименования id сменился: %s -> %s", first.ID, again.ID)
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		t.Fatal(err)
	}
	if again.Path != abs {
		t.Fatalf("путь привязки %s, файл %s", again.Path, abs)
	}
	count, err := store.bindingCount(dst)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("привязок %d", count)
	}
}

func writeGrouped(t *testing.T, dir string) string {
	t.Helper()
	book, err := vbm.Parse(synthTemplate())
	if err != nil {
		t.Fatal(err)
	}
	book.Songs = []vbm.Song{songWith("Песня", "текст", fbidA, 0)}
	book.Groups = []vbm.Group{{Name: "группа", Songs: []uint32{0}}}
	raw, err := packVBM(book)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "grouped.vbm")
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}
