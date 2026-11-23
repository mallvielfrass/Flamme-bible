package server

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestBibleRead(t *testing.T) {
	srv := newTestServer(t)
	user := registerUser(t, srv, "reader@example.com", "secret-pass", nil)
	ctx := context.Background()

	addModule := func(short, title, status string, verses [][3]any) {
		t.Helper()
		id := uuid.NewString()
		if _, err := testPool.Exec(ctx, `INSERT INTO modules (id, owner_user_id, short_name, title, status) VALUES ($1, $2, $3, $4, $5)`, id, user.UserID, short, title, status); err != nil {
			t.Fatal(err)
		}
		for _, v := range verses {
			if _, err := testPool.Exec(ctx, `INSERT INTO module_verses (module_id, book, book_name, chapter, verse, body) VALUES ($1, $2, 'Иоанн', $3, $4, $5)`, id, 43, v[0], v[1], v[2]); err != nil {
				t.Fatal(err)
			}
		}
	}
	addModule("BRA", "Перевод А", "approved", [][3]any{
		{3, 16, "Ибо так возлюбил Бог мир"},
		{3, 17, "Ибо не послал Бог Сына Своего"},
		{1, 1, "В начале было Слово 50%"},
	})
	addModule("BRB", "Перевод Б", "approved", [][3]any{{3, 16, "For God so loved the world"}})
	addModule("BRD", "Черновик", "draft", [][3]any{{3, 16, "черновик"}})
	addModule("BRE", "Без стихов", "approved", nil)

	var tr struct{ Translations []bibleTranslation }
	code, raw := api(t, srv, user.Token, http.MethodGet, "/api/bible/translations", nil)
	if code != http.StatusOK {
		t.Fatalf("переводы %d %s", code, raw)
	}
	mustJSON(t, raw, &tr)
	names := map[string]bool{}
	for _, item := range tr.Translations {
		names[item.ShortName] = true
	}
	if !names["BRA"] || !names["BRB"] || names["BRD"] || names["BRE"] {
		t.Fatalf("%+v", tr.Translations)
	}

	var books struct{ Books []bibleBook }
	code, raw = api(t, srv, user.Token, http.MethodGet, "/api/bible/books?module=BRA", nil)
	if code != http.StatusOK {
		t.Fatalf("книги %d %s", code, raw)
	}
	mustJSON(t, raw, &books)
	if len(books.Books) != 1 || books.Books[0].Book != 43 || books.Books[0].BookName != "Иоанн" || books.Books[0].Chapters != 3 {
		t.Fatalf("%+v", books.Books)
	}

	var chapter bibleChapter
	code, raw = api(t, srv, user.Token, http.MethodGet, "/api/bible/chapter?module=BRA&book=43&chapter=3", nil)
	if code != http.StatusOK {
		t.Fatalf("глава %d %s", code, raw)
	}
	mustJSON(t, raw, &chapter)
	if chapter.ModuleTitle != "Перевод А" || chapter.BookName != "Иоанн" || len(chapter.Verses) != 2 || chapter.Verses[0].Verse != 16 || chapter.Verses[1].Verse != 17 {
		t.Fatalf("%+v", chapter)
	}

	var found struct {
		Verses []bibleVerseHit
		More   bool
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, "/api/bible/search?module=BRA&q="+url.QueryEscape("бог"), nil)
	if code != http.StatusOK {
		t.Fatalf("поиск %d %s", code, raw)
	}
	mustJSON(t, raw, &found)
	if len(found.Verses) != 2 || found.Verses[0].Verse != 16 || found.Verses[0].ModuleShortName != "BRA" || found.More {
		t.Fatalf("%+v", found)
	}
	code, raw = api(t, srv, user.Token, http.MethodGet, "/api/bible/search?module=BRA&q="+url.QueryEscape("0%"), nil)
	mustJSON(t, raw, &found)
	if code != http.StatusOK || len(found.Verses) != 1 || found.Verses[0].Chapter != 1 {
		t.Fatalf("процент как буква %d %s", code, raw)
	}

	for path, want := range map[string]int{
		"/api/bible/books?module=BRD":                                   http.StatusNotFound,
		"/api/bible/books?module=BRE":                                   http.StatusNotFound,
		"/api/bible/books?module=bad%20name":                            http.StatusBadRequest,
		"/api/bible/chapter?module=BRA&book=43&chapter=9":               http.StatusNotFound,
		"/api/bible/chapter?module=BRA&book=x&chapter=3":                http.StatusBadRequest,
		"/api/bible/search?module=BRA&q=a":                              http.StatusBadRequest,
		"/api/bible/search?module=BRD&q=" + url.QueryEscape("черновик"): http.StatusNotFound,
	} {
		if code, raw := api(t, srv, user.Token, http.MethodGet, path, nil); code != want {
			t.Fatalf("%s: %d %s", path, code, raw)
		}
	}
	if code, _ := api(t, srv, "", http.MethodGet, "/api/bible/translations", nil); code != http.StatusUnauthorized {
		t.Fatalf("без входа %d", code)
	}
}
