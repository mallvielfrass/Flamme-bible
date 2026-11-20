package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestChunkVerses(t *testing.T) {
	verses := func(chapter, n int) []moduleVerse {
		out := make([]moduleVerse, n)
		for i := range out {
			out[i] = moduleVerse{Book: 1, BookName: "Бытие", Chapter: chapter, Verse: i + 1, Body: "слово"}
		}
		return out
	}
	sizes := func(n int) []int {
		var got []int
		for _, passage := range chunkVerses(verses(1, n)) {
			got = append(got, passage.VerseEnd-passage.VerseStart+1)
		}
		return got
	}
	if !reflect.DeepEqual(sizes(1), []int{1}) || !reflect.DeepEqual(sizes(2), []int{2}) {
		t.Fatalf("короткая глава %v %v", sizes(1), sizes(2))
	}
	if !reflect.DeepEqual(sizes(5), []int{5}) || !reflect.DeepEqual(sizes(6), []int{3, 3}) {
		t.Fatalf("5 и 6: %v %v", sizes(5), sizes(6))
	}
	if !reflect.DeepEqual(sizes(7), []int{4, 3}) || !reflect.DeepEqual(sizes(10), []int{4, 3, 3}) {
		t.Fatalf("7 и 10: %v %v", sizes(7), sizes(10))
	}
	joined := append(verses(1, 2), verses(2, 2)...)
	passages := chunkVerses(joined)
	if len(passages) != 2 || passages[0].Chapter != 1 || passages[1].Chapter != 2 {
		t.Fatalf("главы не смешиваются %+v", passages)
	}
	if passages[0].Reference != "Бытие 1:1-2" {
		t.Fatalf("ссылка %s", passages[0].Reference)
	}
}

func TestReadBibleQuote(t *testing.T) {
	dir := t.TempDir()
	ini := "BibleName = Пример\nBibleShortName = EX\nChapterSign = <h4>\nVerseSign = <p>\n" +
		"PathName = grief.htm\nFullName = Истории\nChapterQty = 2\n" +
		"PathName = ../secret.txt\nFullName = Чужая\n"
	if err := os.WriteFile(filepath.Join(dir, "bibleqt.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	html := "<h4>1</h4><p>1 Человек сидит в печали.</p><p>2 Друг молчит рядом.</p><h4>2</h4><p>1 Царь послал за Вирсавией.</p>"
	encoded, err := charmap.Windows1251.NewEncoder().Bytes([]byte(html))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grief.htm"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readBibleQuote(dir); err == nil {
		t.Fatal("путь вне каталога принят")
	}

	ini = "BibleShortName = EX\nPathName = grief.htm\nFullName = Истории\nChapterQty = 2\n"
	if err := os.WriteFile(filepath.Join(dir, "bibleqt.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	verses, err := readBibleQuote(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(verses) != 3 || verses[0].Book != 1 || verses[0].BookName != "Истории" || verses[0].Chapter != 1 || verses[2].Chapter != 2 {
		t.Fatalf("%+v", verses)
	}
	if verses[2].Body != "Царь послал за Вирсавией." {
		t.Fatalf("cp1251: %+v", verses[2])
	}
}

func TestBibleSemantic(t *testing.T) {
	emb := &countingEmbedder{}
	srv, svc := newSemanticServer(t, emb)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/modules", map[string]any{
		"short_name": "EX", "title": "Пример",
	})
	if code != http.StatusOK {
		t.Fatalf("модуль %d %s", code, raw)
	}
	var module moduleView
	mustJSON(t, raw, &module)
	dir := writeQuoteModule(t)
	if _, err := svc.ImportBibleModule(context.Background(), module.ID, dir); err != nil {
		t.Fatal(err)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/bible/semantic?q="+urlQuery("как утешить человека в горе"), nil)
	if code != http.StatusOK || len(bibleHits(t, raw)) != 0 {
		t.Fatalf("черновик модуля %d %s", code, raw)
	}

	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules/"+module.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("отправка %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/modules/"+module.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("одобрение %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/bible/semantic?q="+urlQuery("как утешить человека в горе")+"&module=EX", nil)
	if code != http.StatusOK {
		t.Fatalf("поиск %d %s", code, raw)
	}
	hits := bibleHits(t, raw)
	if len(hits) != 3 || hits[0].Chapter != 1 || hits[0].ModuleShortName != "EX" || hits[0].VerseEnd-hits[0].VerseStart+1 != 3 {
		t.Fatalf("горе %+v", hits)
	}
	for i, hit := range hits {
		if hit.Chapter == 2 && i == 0 {
			t.Fatalf("царь выше горя %+v", hits)
		}
	}
	var king bool
	for _, hit := range hits {
		if hit.Chapter == 2 {
			king = hit.Reference == "Истории 2:1-2"
		}
	}
	if !king {
		t.Fatalf("нет отрывка о царе %+v", hits)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/bible/semantic?q="+urlQuery("Давид и Вирсавия")+"&module=EX", nil)
	hits = bibleHits(t, raw)
	if code != http.StatusOK || len(hits) == 0 || hits[0].Chapter != 2 {
		t.Fatalf("вирсавия %d %+v", code, hits)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules", map[string]any{
		"short_name": "HIDE", "title": "Черновик",
	})
	if code != http.StatusOK {
		t.Fatalf("второй модуль %d %s", code, raw)
	}
	var hidden moduleView
	mustJSON(t, raw, &hidden)
	if _, err := svc.ImportBibleModule(context.Background(), hidden.ID, dir); err != nil {
		t.Fatal(err)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/bible/semantic?q="+urlQuery("Вирсавия"), nil)
	hits = bibleHits(t, raw)
	if code != http.StatusOK || len(hits) != 3 {
		t.Fatalf("черновик попал в выдачу %d %+v", code, hits)
	}
	for _, hit := range hits {
		if hit.ModuleID != module.ID {
			t.Fatalf("чужой модуль %+v", hit)
		}
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/bible/semantic?q=мир&module=!!!", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("модуль %d %s", code, raw)
	}

	before := emb.Calls()
	if _, err := svc.ReindexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if emb.Calls() != before {
		t.Fatalf("повтор пересчитал отрывки: %d -> %d", before, emb.Calls())
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/modules/"+module.ID+"/revoke", map[string]any{"note": "снято"})
	if code != http.StatusOK {
		t.Fatalf("отзыв %d %s", code, raw)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/bible/semantic?q="+urlQuery("Вирсавия"), nil)
	if code != http.StatusOK || len(bibleHits(t, raw)) != 0 {
		t.Fatalf("после отзыва %d %s", code, raw)
	}
	var verses int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM module_verses WHERE module_id = $1`, module.ID).Scan(&verses); err != nil {
		t.Fatal(err)
	}
	if verses == 0 {
		t.Fatal("отзыв стёр стихи")
	}
}

func writeQuoteModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ini := "BibleShortName = EX\nChapterSign = <h4>\nVerseSign = <p>\nPathName = book.htm\nFullName = Истории\nChapterQty = 2\n"
	if err := os.WriteFile(filepath.Join(dir, "bibleqt.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	html := "<h4>1</h4>" +
		"<p>1 Человек сидит в печали и не ест.</p>" +
		"<p>2 Друг молчит рядом с ним.</p>" +
		"<p>3 Печаль не проходит за один день.</p>" +
		"<p>4 Он говорит: мне тяжело.</p>" +
		"<p>5 Друг остаётся до утра.</p>" +
		"<p>6 Утром печаль чуть тише.</p>" +
		"<h4>2</h4>" +
		"<p>1 Царь с крыши увидел Вирсавию.</p>" +
		"<p>2 Пророк обличил царя, и тот признал вину.</p>"
	if err := os.WriteFile(filepath.Join(dir, "book.htm"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func bibleHits(t *testing.T, raw []byte) []bibleHit {
	t.Helper()
	var body struct {
		Passages []bibleHit `json:"passages"`
	}
	mustJSON(t, raw, &body)
	return body.Passages
}
