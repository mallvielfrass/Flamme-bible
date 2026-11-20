package main

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"flammebible/internal/vbm"
)

func TestStoreKeepsRowsAndBinding(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "book.vbm")
	writeSynthBook(t, modulePath, []vbm.Song{songWith("Тестовая песня", "тестовый куплет", fbidA, 0)})
	dbPath := filepath.Join(dir, "local.db")

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Bind(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Bind(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatalf("повторная привязка создала другой id: %s и %s", first.ID, again.ID)
	}
	parsed, err := vbm.ParseFile(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSnapshot(first.ID, parsed.Songs); err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(first.ID, "added", "song-1", `{"title":"песня"}`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStage(first.ID, "idle"); err != nil {
		t.Fatal(err)
	}
	wantSongs, err := store.Snapshot(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantOps, err := store.Operations(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantStage, err := store.Stage(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	bound, err := reopened.Bind(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if bound.ID != first.ID {
		t.Fatalf("после открытия id изменился: %s", bound.ID)
	}
	count, err := reopened.bindingCount(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("привязок по пути: %d", count)
	}
	gotSongs, err := reopened.Snapshot(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotSongs, wantSongs) {
		t.Fatal("снимок после открытия не совпал")
	}
	gotOps, err := reopened.Operations(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotOps, wantOps) {
		t.Fatal("очередь после открытия не совпала")
	}
	gotStage, err := reopened.Stage(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotStage != wantStage {
		t.Fatalf("стадия %q, ожидалась %q", gotStage, wantStage)
	}

	writeSynthBookIdentified(t, modulePath, "Другой синтетический сборник", []vbm.Song{songWith("Другая тестовая", "другой текст", fbidB, 0)})
	replaced, err := reopened.Bind(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.ID == first.ID {
		t.Fatal("смена файла переиспользовала чужой id")
	}
	var oldPath sql.NullString
	if err := reopened.db.QueryRow(`SELECT path FROM books WHERE id = ?`, first.ID).Scan(&oldPath); err != nil {
		t.Fatal(err)
	}
	if oldPath.Valid {
		t.Fatalf("старый id всё ещё привязан к пути %s", oldPath.String)
	}
	count, err = reopened.bindingCount(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("после смены файла привязок: %d", count)
	}
}
