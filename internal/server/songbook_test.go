package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSongbookChain(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	var parent string
	var root string
	for i := 1; i <= 8; i++ {
		body := map[string]any{"fbid": "book", "title": "уровень"}
		if parent != "" {
			body["parent_id"] = parent
		}
		view := createBook(t, srv, user.Token, user.DimensionID, body)
		if i == 1 {
			root = view.ID
		}
		parent = view.ID
	}
	code, raw := api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks", map[string]any{
		"fbid": "book", "title": "девятый", "parent_id": parent, "header": headerBody(),
	})
	if code != http.StatusConflict {
		t.Fatalf("девятый уровень %d %s", code, raw)
	}
	var errBody map[string]string
	mustJSON(t, raw, &errBody)
	if errBody["error"] != "глубина" {
		t.Fatalf("ошибка глубины %s", errBody["error"])
	}
	self := uuid.NewString()
	code, raw = api(t, srv, user.Token, http.MethodPost, "/dimensions/"+user.DimensionID+"/songbooks", map[string]any{
		"id": self, "fbid": "book", "parent_id": self, "header": headerBody(),
	})
	if code != http.StatusConflict {
		t.Fatalf("ссылка на себя %d %s", code, raw)
	}
	mustJSON(t, raw, &errBody)
	if errBody["error"] != "цикл" {
		t.Fatalf("ошибка цикла %s", errBody["error"])
	}
	child := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "patch", "parent_id": root})
	code, raw = api(t, srv, user.Token, http.MethodPost, "/songbooks/"+root+"/parent", map[string]any{"parent_id": child.ID})
	if code != http.StatusConflict {
		t.Fatalf("ссылка на потомка %d %s", code, raw)
	}
	mustJSON(t, raw, &errBody)
	if errBody["error"] != "цикл" {
		t.Fatalf("ошибка потомка %s", errBody["error"])
	}
}

func TestSongbookHideKeepsRows(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	root := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "root", "title": "корень"})
	patch := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "patch", "title": "патч", "parent_id": root.ID})
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO songs (songbook_id, fbid, parts, revision) VALUES ($1, 'song-1', '[]', 0)
	`, root.ID); err != nil {
		t.Fatal(err)
	}
	code, raw := api(t, srv, user.Token, http.MethodPost, "/songbooks/"+root.ID+"/hide", nil)
	if code != http.StatusOK {
		t.Fatalf("скрытие %d %s", code, raw)
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, "/dimensions/"+user.DimensionID+"/songbooks", nil)
	if code != http.StatusOK {
		t.Fatalf("список %d %s", code, raw)
	}
	var list []songbookView
	mustJSON(t, raw, &list)
	if len(list) != 2 {
		t.Fatalf("после скрытия сборников %d", len(list))
	}
	var hidden bool
	var parent *string
	for _, book := range list {
		if book.ID == root.ID {
			hidden = book.Hidden
		}
		if book.ID == patch.ID {
			parent = book.ParentID
		}
	}
	if !hidden || parent == nil || *parent != root.ID {
		t.Fatalf("скрытие удалило связь hidden=%v parent=%v", hidden, parent)
	}
	var songs int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM songs WHERE songbook_id = $1`, root.ID).Scan(&songs); err != nil {
		t.Fatal(err)
	}
	if songs != 1 {
		t.Fatalf("скрытие удалило песни: %d", songs)
	}
}
