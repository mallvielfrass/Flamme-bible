package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestPlanItemText(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	pushAt(t, srv, owner, book.ID, 0, []testSong{{"grace", "Благодать", "Куплет о благодати", false}})

	var song songText
	code, raw := api(t, srv, owner.Token, http.MethodGet, "/api/songs/text?songbook_id="+book.ID+"&fbid=grace", nil)
	if code != http.StatusOK {
		t.Fatalf("песня %d %s", code, raw)
	}
	mustJSON(t, raw, &song)
	if song.Title != "Благодать" || song.SongbookTitle != "Домашний" || song.Text != "Куплет о благодати" {
		t.Fatalf("%+v", song)
	}
	if code, raw = api(t, srv, other.Token, http.MethodGet, "/api/songs/text?songbook_id="+book.ID+"&fbid=grace", nil); code != http.StatusNotFound {
		t.Fatalf("чужой черновик %d %s", code, raw)
	}
	if code, _ = api(t, srv, owner.Token, http.MethodGet, "/api/songs/text?songbook_id=bad&fbid=grace", nil); code != http.StatusBadRequest {
		t.Fatalf("плохой id %d", code)
	}

	ctx := context.Background()
	moduleID := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO modules (id, owner_user_id, short_name, title, status) VALUES ($1, $2, 'EX', 'Пример', 'approved')`, moduleID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO module_verses (module_id, book, book_name, chapter, verse, body) VALUES ($1, 43, 'Иоанн', 3, 16, 'Ибо так возлюбил Бог мир')`, moduleID); err != nil {
		t.Fatal(err)
	}
	var verse verseText
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/bible/verse?module=EX&book=43&chapter=3&verse=16", nil)
	if code != http.StatusOK {
		t.Fatalf("стих %d %s", code, raw)
	}
	mustJSON(t, raw, &verse)
	if verse.ModuleTitle != "Пример" || verse.BookName != "Иоанн" || verse.Text != "Ибо так возлюбил Бог мир" {
		t.Fatalf("%+v", verse)
	}
	if code, _ = api(t, srv, other.Token, http.MethodGet, "/api/bible/verse?module=EX&book=43&chapter=3&verse=17", nil); code != http.StatusNotFound {
		t.Fatalf("нет стиха %d", code)
	}
}
