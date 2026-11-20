package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestPullPages(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "a@example.com", "secret-pass", nil)
	book := createBook(t, srv, user.Token, user.DimensionID, map[string]any{"fbid": "book"})
	path := "/dimensions/" + user.DimensionID + "/songbooks/" + book.ID + "/push"
	titles := []string{"один", "два", "три"}
	var base int64
	for i, title := range titles {
		fbid := "song-1"
		if i == 1 {
			fbid = "song-2"
		}
		code, raw := api(t, srv, user.Token, http.MethodPost, path, pushBody(base, songOp(uuid.NewString(), fbid, title, nil)))
		if code != http.StatusOK {
			t.Fatalf("push %d %d %s", i, code, raw)
		}
		var resp pushResp
		mustJSON(t, raw, &resp)
		if resp.Results[0].Status != "applied" {
			t.Fatalf("push %d %+v", i, resp.Results[0])
		}
		base = resp.Results[0].Revision
	}
	code, raw := api(t, srv, user.Token, http.MethodGet, "/dimensions/"+user.DimensionID+"/songbooks/"+book.ID+"/snapshot", nil)
	if code != http.StatusOK {
		t.Fatalf("снимок %d %s", code, raw)
	}
	var snap struct {
		Cursor int64 `json:"cursor"`
		Songs  []struct {
			FBID  string `json:"fbid"`
			Title string `json:"title"`
		} `json:"songs"`
	}
	mustJSON(t, raw, &snap)
	if snap.Cursor != 3 || len(snap.Songs) != 2 {
		t.Fatalf("снимок cursor=%d songs=%d", snap.Cursor, len(snap.Songs))
	}
	var current string
	for _, song := range snap.Songs {
		if song.FBID == "song-1" {
			current = song.Title
		}
	}
	if current != "три" {
		t.Fatalf("снимок держит старый текст %s", current)
	}
	changes := "/dimensions/" + user.DimensionID + "/changes"
	read := func(cursor string) (int, []struct {
		Seq  int64 `json:"seq"`
		Body struct {
			Song struct {
				Title string `json:"title"`
			} `json:"song"`
		} `json:"body"`
	}) {
		t.Helper()
		code, raw := api(t, srv, user.Token, http.MethodGet, changes+"?cursor="+cursor+"&limit=1", nil)
		if code != http.StatusOK {
			t.Fatalf("pull %s %d %s", cursor, code, raw)
		}
		var page struct {
			Events []struct {
				Seq  int64 `json:"seq"`
				Body struct {
					Song struct {
						Title string `json:"title"`
					} `json:"song"`
				} `json:"body"`
			} `json:"events"`
		}
		mustJSON(t, raw, &page)
		return code, page.Events
	}
	_, first := read("0")
	_, again := read("0")
	if len(first) != 1 || first[0].Seq != 1 || first[0].Body.Song.Title != "один" {
		t.Fatalf("первая страница %+v", first)
	}
	if len(again) != 1 || again[0].Seq != 1 {
		t.Fatalf("повтор первой страницы прыгнул в журнал %+v", again)
	}
	_, second := read("1")
	if len(second) != 1 || second[0].Seq != 2 || second[0].Body.Song.Title != "два" {
		t.Fatalf("вторая страница %+v", second)
	}
	_, third := read("2")
	if len(third) != 1 || third[0].Seq != 3 || third[0].Body.Song.Title != "три" {
		t.Fatalf("событие ревизии %+v", third)
	}
	_, tail := read("3")
	if len(tail) != 0 {
		t.Fatalf("после снимка есть события %+v", tail)
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, changes+"?cursor=99", nil)
	if code != http.StatusConflict {
		t.Fatalf("будущий курсор %d %s", code, raw)
	}
	if _, err := testPool.Exec(context.Background(), `DELETE FROM change_log WHERE dimension_id = $1 AND seq = 1`, user.DimensionID); err != nil {
		t.Fatal(err)
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, changes+"?cursor=1", nil)
	if code != http.StatusConflict {
		t.Fatalf("старый курсор %d %s", code, raw)
	}
	var errBody map[string]string
	mustJSON(t, raw, &errBody)
	if errBody["error"] != "reload" {
		t.Fatalf("ошибка курсора %s", errBody["error"])
	}
}
