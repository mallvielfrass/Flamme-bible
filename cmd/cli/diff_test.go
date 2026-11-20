package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flammebible/internal/vbm"
)

const (
	fbidA = "fba56871-f5d6-4b32-8b42-4bca03278ec1"
	fbidB = "11111111-2222-4333-8444-555555555555"
	fbidC = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

func songWith(title, text, fbid string, align uint32) vbm.Song {
	props := "$TUNE$=\r\n$ID$=1\r\n"
	if fbid != "" {
		props += "$FBID$=" + fbid + "\r\n"
	}
	return vbm.Song{
		Title:      title,
		Properties: props,
		Parts:      []vbm.Part{{Kind: "Куплет", Text: text, Align: align}},
	}
}

func writeSongs(t *testing.T, dir, name string, songs []vbm.Song) string {
	t.Helper()
	raw := synthTemplate()
	book, err := vbm.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	book.Songs = songs
	out, err := vbm.Write(book, raw)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func openTempStore(t *testing.T, dir string) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(dir, "local.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestDiffAddEditDelete(t *testing.T) {
	dir := t.TempDir()
	edited := songWith("Песня Б", "новый текст", fbidB, 0)
	added := songWith("Песня В", "добавленная", fbidC, 0)
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{edited, added})
	store := openTempStore(t, dir)
	binding, err := store.Bind(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSnapshot(binding.ID, []vbm.Song{
		songWith("Песня А", "текст а", fbidA, 0),
		songWith("Песня Б", "старый текст", fbidB, 0),
	}); err != nil {
		t.Fatal(err)
	}
	changes, err := store.Diff(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].Kind != "changed" || changes[0].FBID != fbidB || changes[1].Kind != "added" || changes[1].FBID != fbidC || changes[2].Kind != "deleted" || changes[2].FBID != fbidA {
		t.Fatalf("изменения: %+v", changes)
	}
	ops, err := store.Operations(binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 3 {
		t.Fatalf("в очереди %d операций", len(ops))
	}
	again, err := store.Diff(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Fatalf("повторный diff размножил изменения: %d", len(again))
	}
	ops, err = store.Operations(binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 3 {
		t.Fatalf("повторный diff размножил очередь: %d", len(ops))
	}
}

func TestDiffAssignsFBIDOnce(t *testing.T) {
	dir := t.TempDir()
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{songWith("Без номера", "текст", "", 0)})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := openTempStore(t, dir)
	first, err := store.Diff(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Diff(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Kind != "added" || !validUUID(first[0].FBID) {
		t.Fatalf("первое назначение: %+v", first)
	}
	if second[0].FBID != first[0].FBID {
		t.Fatalf("повторный запуск сменил UUID %s на %s", first[0].FBID, second[0].FBID)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("сравнение записало UUID в файл")
	}
	parsed, err := vbm.Parse(after)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(parsed.Songs[0].Properties, first[0].FBID) {
		t.Fatal("FBID оказался в properties")
	}
}

func TestDiffIgnoresLineEndingsButNotSpacesOrAlign(t *testing.T) {
	dir := t.TempDir()
	base := songWith("Песня", "строка\nещё", fbidA, 0)
	path := writeSongs(t, dir, "endings.vbm", []vbm.Song{songWith("Песня", "строка\r\nещё\r\n", fbidA, 0)})
	store := openTempStore(t, dir)
	binding, err := store.Bind(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSnapshot(binding.ID, []vbm.Song{base}); err != nil {
		t.Fatal(err)
	}
	changes, err := store.Diff(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("LF/CRLF сочтён правкой: %+v", changes)
	}

	spaced := writeSongs(t, dir, "spaces.vbm", []vbm.Song{songWith("Песня", "строка  ещё", fbidA, 0)})
	if err := os.MkdirAll(dir+"-spaces", 0o755); err != nil {
		t.Fatal(err)
	}
	store2 := openTempStore(t, dir+"-spaces")
	binding, err = store2.Bind(spaced)
	if err != nil {
		t.Fatal(err)
	}
	if err := store2.SaveSnapshot(binding.ID, []vbm.Song{songWith("Песня", "строка ещё", fbidA, 0)}); err != nil {
		t.Fatal(err)
	}
	changes, err = store2.Diff(spaced)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != "changed" {
		t.Fatalf("пробел внутри текста не сочтён правкой: %+v", changes)
	}

	aligned := writeSongs(t, dir, "align.vbm", []vbm.Song{songWith("Песня", "строка\nещё", fbidA, 2)})
	if err := os.MkdirAll(dir+"-align", 0o755); err != nil {
		t.Fatal(err)
	}
	store3 := openTempStore(t, dir+"-align")
	binding, err = store3.Bind(aligned)
	if err != nil {
		t.Fatal(err)
	}
	if err := store3.SaveSnapshot(binding.ID, []vbm.Song{base}); err != nil {
		t.Fatal(err)
	}
	changes, err = store3.Diff(aligned)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != "changed" {
		t.Fatalf("align не сочтён правкой: %+v", changes)
	}
}

func TestDiffRejectsBadAndDuplicateFBID(t *testing.T) {
	dir := t.TempDir()
	bad := writeSongs(t, dir, "bad.vbm", []vbm.Song{songWith("Песня", "текст", "не-uuid", 0)})
	dup := writeSongs(t, dir, "dup.vbm", []vbm.Song{
		songWith("Одна", "текст", fbidA, 0),
		songWith("Другая", "текст", fbidA, 0),
	})
	store := openTempStore(t, dir)
	anchor := writeSongs(t, dir, "anchor.vbm", []vbm.Song{songWith("Опора", "текст", fbidB, 0)})
	binding, err := store.Bind(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(binding.ID, "added", fbidB, `{"title":"Опора"}`); err != nil {
		t.Fatal(err)
	}
	before, err := store.Operations(binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Diff(bad); !errors.Is(err, errBadFBID) {
		t.Fatalf("битый FBID: %v", err)
	}
	if _, err := store.Diff(dup); !errors.Is(err, errDuplicateFBID) {
		t.Fatalf("повторный FBID: %v", err)
	}
	// Diff битого файла привязывает его отдельно и не должен пополнять его очередь.
	for _, path := range []string{bad, dup} {
		bound, err := store.Bind(path)
		if err != nil {
			t.Fatal(err)
		}
		ops, err := store.Operations(bound.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(ops) != 0 {
			t.Fatalf("очередь %s пополнена: %+v", path, ops)
		}
	}
	after, err := store.Operations(binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("существующая очередь изменилась: %+v", after)
	}
}

func TestDiffMissingFileIsNotDeletion(t *testing.T) {
	dir := t.TempDir()
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{
		songWith("Песня А", "текст", fbidA, 0),
		songWith("Песня Б", "текст", fbidB, 0),
	})
	store := openTempStore(t, dir)
	binding, err := store.Bind(path)
	if err != nil {
		t.Fatal(err)
	}
	songs := []vbm.Song{
		songWith("Песня А", "текст", fbidA, 0),
		songWith("Песня Б", "текст", fbidB, 0),
	}
	if err := store.SaveSnapshot(binding.ID, songs); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Diff(filepath.Join(dir, "missing.vbm")); !errors.Is(err, errFileNotFound) {
		t.Fatalf("ошибка: %v", err)
	}
	got, err := store.Snapshot(binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("снимок стёрт: %d", len(got))
	}
	ops, err := store.Operations(binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("отсутствие файла записало удаления: %+v", ops)
	}
}
