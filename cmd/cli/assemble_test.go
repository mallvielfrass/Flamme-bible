package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"flammebible/internal/vbm"
)

func dtoSong(title, text, fbid string, deleted bool) songDTO {
	song, err := songFromVBM(songWith(title, text, fbid, 0), deleted)
	if err != nil {
		panic(err)
	}
	return song
}

func titlesOf(songs []vbm.Song) []string {
	out := make([]string, len(songs))
	for i, song := range songs {
		out[i] = song.Title
	}
	return out
}

func TestAssembleAddedSongAndOrder(t *testing.T) {
	parent := Layer{
		ID:    "10000000-0000-4000-8000-000000000001",
		Songs: []songDTO{dtoSong("П1", "а", fbidA, false), dtoSong("П2", "б", fbidB, false)},
		Order: []string{fbidA, fbidB},
	}
	added := "dddddddd-eeee-4fff-8aaa-bbbbbbbbbbbb"
	patch := Layer{
		ID:       "10000000-0000-4000-8000-000000000002",
		ParentID: parent.ID,
		Songs:    []songDTO{dtoSong("Новая", "г", added, false)},
	}
	songs, _, err := Assemble([]Layer{parent, patch})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(titlesOf(songs), ",") != "П1,П2,Новая" {
		t.Fatalf("порядок %v", titlesOf(songs))
	}
	for i, song := range songs {
		if !strings.Contains(song.Properties, "$ID$="+itoa(i+1)) {
			t.Fatalf("позиция %d: %s", i, song.Properties)
		}
	}
	parent.Songs = append(parent.Songs, dtoSong("П3", "в", fbidC, false))
	parent.Order = []string{fbidA, fbidB, fbidC}
	ordered := Layer{
		ID:       patch.ID,
		ParentID: parent.ID,
		Order:    []string{fbidB, fbidA},
	}
	songs, report, err := Assemble([]Layer{parent, ordered})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(titlesOf(songs), ",") != "П2,П1,П3" {
		t.Fatalf("свой порядок %v", titlesOf(songs))
	}
	if !strings.Contains(report, "в конец "+fbidC) {
		t.Fatalf("отчёт %s", report)
	}
}

func itoa(n int) string {
	return string([]byte{byte('0' + n)})
}

func TestAssembleDeleteWithoutEdit(t *testing.T) {
	parent := Layer{
		ID: "10000000-0000-4000-8000-000000000001",
		Songs: []songDTO{
			dtoSong("П1", "а", fbidA, true),
			dtoSong("П2", "б", fbidB, false),
		},
		Order: []string{fbidA, fbidB},
	}
	patch := Layer{ID: "10000000-0000-4000-8000-000000000002", ParentID: parent.ID}
	songs, report, err := Assemble([]Layer{parent, patch})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(report, "вопрос") || strings.Join(titlesOf(songs), ",") != "П2" {
		t.Fatalf("songs=%v report=%s", titlesOf(songs), report)
	}
}

func TestAssembleDeleteWithEdit(t *testing.T) {
	parent := Layer{
		ID:    "10000000-0000-4000-8000-000000000001",
		Songs: []songDTO{dtoSong("П1", "старый", fbidA, true)},
		Order: []string{fbidA},
	}
	edited := dtoSong("Правка", "новый", fbidA, false)
	patch := Layer{
		ID:       "10000000-0000-4000-8000-000000000002",
		ParentID: parent.ID,
		Songs:    []songDTO{edited},
	}
	_, report, err := Assemble([]Layer{parent, patch})
	if !errors.Is(err, errChoose) || !strings.Contains(report, "вопрос "+fbidA) {
		t.Fatalf("err=%v report=%s", err, report)
	}
	keep := patch
	keep.Decisions = map[string]string{fbidA: "retain"}
	songs, report, err := Assemble([]Layer{parent, keep})
	if err != nil {
		t.Fatal(err, report)
	}
	if len(songs) != 1 || songs[0].Title != "Правка" || !strings.Contains(songs[0].Properties, "новый") && !partIs(songs[0], "новый") {
		t.Fatalf("retain %+v", songs)
	}
	drop := patch
	drop.Decisions = map[string]string{fbidA: "drop"}
	songs, report, err = Assemble([]Layer{parent, drop})
	if err != nil {
		t.Fatal(err, report)
	}
	if len(songs) != 0 || strings.Contains(report, "вопрос") {
		t.Fatalf("drop songs=%d report=%s", len(songs), report)
	}
}

func partIs(song vbm.Song, text string) bool {
	for _, part := range song.Parts {
		if strings.Contains(part.Text, text) {
			return true
		}
	}
	return false
}

func TestAssembleSuppressStaysOnOnePatch(t *testing.T) {
	parent := Layer{
		ID:    "10000000-0000-4000-8000-000000000001",
		Songs: []songDTO{dtoSong("П1", "а", fbidA, false), dtoSong("П2", "б", fbidB, false)},
		Order: []string{fbidA, fbidB},
	}
	hidden := Layer{
		ID:        "10000000-0000-4000-8000-000000000002",
		ParentID:  parent.ID,
		Decisions: map[string]string{fbidA: "suppress"},
	}
	other := Layer{ID: "10000000-0000-4000-8000-000000000003", ParentID: parent.ID}
	songs, _, err := Assemble([]Layer{parent, hidden})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(titlesOf(songs), ",") != "П2" {
		t.Fatalf("suppress %v", titlesOf(songs))
	}
	songs, _, err = Assemble([]Layer{parent, other})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(titlesOf(songs), ",") != "П1,П2" {
		t.Fatalf("сосед %v", titlesOf(songs))
	}
}

func TestCycleDoesNotReplaceFile(t *testing.T) {
	dir := t.TempDir()
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{songWith("А", "текст", fbidA, 0)})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	layers := []Layer{
		{ID: fbidC, ParentID: fbidB},
		{ID: fbidB, ParentID: fbidC},
	}
	_, err = buildLocal(path, layers, true)
	if !errors.Is(err, errCycle) {
		t.Fatalf("err=%v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("файл заменён при цикле")
	}
}

func TestQuestionDoesNotReplaceFile(t *testing.T) {
	dir := t.TempDir()
	path := writeSongs(t, dir, "book.vbm", []vbm.Song{songWith("А", "текст", fbidA, 0)})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parent := Layer{
		ID:    "10000000-0000-4000-8000-000000000001",
		Songs: []songDTO{dtoSong("П1", "старый", fbidA, true)},
	}
	patch := Layer{
		ID:       "10000000-0000-4000-8000-000000000002",
		ParentID: parent.ID,
		Songs:    []songDTO{dtoSong("Правка", "новый", fbidA, false)},
	}
	_, err = buildLocal(path, []Layer{parent, patch}, true)
	if !errors.Is(err, errChoose) {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("файл заменён до ответа")
	}
	patch.Decisions = map[string]string{fbidA: "retain"}
	if _, err := buildLocal(path, []Layer{parent, patch}, true); err != nil {
		t.Fatal(err)
	}
	book, err := vbm.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Songs) != 1 || book.Songs[0].Title != "Правка" || !partIs(book.Songs[0], "новый") {
		t.Fatalf("%+v", book.Songs)
	}
	if !strings.Contains(book.Songs[0].Properties, "$ID$=1") {
		t.Fatalf("id %s", book.Songs[0].Properties)
	}
}
