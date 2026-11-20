package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flammebible/internal/vbm"
)

func TestInstallTwoCatalogsRequiresChoice(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "one", "Modules")
	second := filepath.Join(root, "two", "Modules")
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "one", "VisioBible.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "two", "VisioBible.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	left := writeSongs(t, first, "a.vbm", []vbm.Song{songWith("А", "текст", "", 0)})
	right := writeSongs(t, second, "b.vbm", []vbm.Song{songWith("Б", "текст", "", 0)})
	beforeLeft, err := os.ReadFile(left)
	if err != nil {
		t.Fatal(err)
	}
	beforeRight, err := os.ReadFile(right)
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "local.db")

	err = run([]string{"install", "--root", root, "--db", db, "--apply"})
	if err == nil || !strings.Contains(err.Error(), "нужен выбор") {
		t.Fatalf("ожидался выбор: %v", err)
	}
	afterLeft, err := os.ReadFile(left)
	if err != nil {
		t.Fatal(err)
	}
	afterRight, err := os.ReadFile(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterLeft) != string(beforeLeft) || string(afterRight) != string(beforeRight) {
		t.Fatal("без выбора файлы изменились")
	}

	err = run([]string{"install", "--root", root, "--dir", first, "--db", db, "--apply"})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := os.ReadFile(left)
	if err != nil {
		t.Fatal(err)
	}
	untouched, err := os.ReadFile(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(changed) == string(beforeLeft) {
		t.Fatal("выбранный файл не записан")
	}
	if string(untouched) != string(beforeRight) {
		t.Fatal("второй каталог записан")
	}
}

func TestInstallWithoutINI(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, "portable", "Modules")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "portable", "VisioBible.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSongs(t, modules, "book.vbm", []vbm.Song{songWith("Песня", "текст", fbidA, 0)})
	err := run([]string{"install", "--root", root, "--db", filepath.Join(root, "local.db")})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInstallProgramDataAndINI(t *testing.T) {
	root := t.TempDir()
	portable := filepath.Join(root, "portable", "Modules")
	shared := filepath.Join(root, "ProgramData", "VisioBible V2.5", "Modules")
	extra := filepath.Join(root, "extra")
	if err := os.MkdirAll(portable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "portable", "VisioBible.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	ini := "[BiblePath]\r\n0=\"" + strings.ReplaceAll(extra, `\`, `\\`) + "\"|1\r\n"
	if err := os.WriteFile(filepath.Join(root, "portable", "VisioBible.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := findCatalogs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{filepath.Clean(portable): true, filepath.Clean(shared): true, filepath.Clean(extra): true}
	if len(found) != len(want) {
		t.Fatalf("каталоги: %v", found)
	}
	for _, path := range found {
		if !want[path] {
			t.Fatalf("лишний каталог %s в %v", path, found)
		}
	}
}
