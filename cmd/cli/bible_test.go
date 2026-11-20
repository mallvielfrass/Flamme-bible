package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBibleIndexDoesNotSendVerses(t *testing.T) {
	dir := t.TempDir()
	ini := "" +
		"BibleName = Синодальный\n" +
		"BibleShortName = RST+\n" +
		"BibleId=rfwfwqd3dwea\n" +
		"PathName = Book1.html\n" +
		"FullName = Бытие\n" +
		"ChapterQty = 50\n" +
		"BookHash=grfsrsrew3rq2q2eq\n" +
		"PathName = Book2.html\n" +
		"FullName = Исход\n" +
		"ChapterQty = 40\n"
	if err := os.WriteFile(filepath.Join(dir, "bibleqt.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	verse := "СТИХ-СЕКРЕТ В начале сотворил Бог небо и землю"
	htmlPath := filepath.Join(dir, "Book1.html")
	if err := os.WriteFile(htmlPath, []byte(verse), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	index, err := readBible(dir)
	if err != nil {
		t.Fatal(err)
	}
	if index.BibleID != verifiedBibleID || len(index.Books) != 2 {
		t.Fatalf("индекс %+v", index)
	}
	if index.Books[0].Hash != "grfsrsrew3rq2q2eq" || index.Books[1].Hash != "" {
		t.Fatalf("хеши %+v", index.Books)
	}
	printed := formatBible(index)
	if strings.Contains(printed, "СТИХ-СЕКРЕТ") || strings.Contains(printed, "сотворил") {
		t.Fatal("текст стиха ушёл в результат")
	}
	after, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("html изменён")
	}
	other := t.TempDir()
	otherINI := "BibleShortName = OTHER\nPathName = Book1.html\nFullName = Книга\nChapterQty = 1\n"
	if err := os.WriteFile(filepath.Join(other, "bibleqt.ini"), []byte(otherINI), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeVerifiedBibleID(other); err == nil {
		t.Fatal("BibleId записан в непроверенный модуль")
	}
	kept, err := os.ReadFile(filepath.Join(other, "bibleqt.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != otherINI || strings.Contains(string(kept), "BookHash") {
		t.Fatal("непроверенный модуль изменён")
	}
	copyDir := t.TempDir()
	copyINI := "BibleName = Синодальный\nBibleShortName = RST+\nPathName = Book1.html\nFullName = Бытие\nChapterQty = 50\nPathName = Book2.html\nFullName = Исход\nChapterQty = 40\n"
	if err := os.WriteFile(filepath.Join(copyDir, "bibleqt.ini"), []byte(copyINI), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeVerifiedBibleID(copyDir); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(copyDir, "bibleqt.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "BibleId="+verifiedBibleID) || strings.Contains(string(written), "BookHash") {
		t.Fatalf("шапка копии: %s", written)
	}
}
