package server

import (
	"bytes"
	"net/http"
	"testing"
)

func TestSongbookSectionsAndSearch(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	pushTwoSongs(t, srv, owner, book.ID)

	code, raw := api(t, srv, other.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/sections", map[string]any{"title": "Чужой"})
	if code != http.StatusNotFound {
		t.Fatalf("чужой раздел %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/sections", map[string]any{"title": "Служение"})
	if code != http.StatusOK {
		t.Fatalf("раздел %d %s", code, raw)
	}
	var section sectionView
	mustJSON(t, raw, &section)
	if section.Title != "Служение" || section.Count != 0 {
		t.Fatalf("создан %+v", section)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/songbooks/"+book.ID+"/sections/"+section.ID, map[string]any{"title": "Воскресенье"})
	if code != http.StatusOK {
		t.Fatalf("имя %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/sections/"+section.ID+"/songs", map[string]any{
		"fbids": []string{"alpha"},
	})
	if code != http.StatusOK {
		t.Fatalf("песни %d %s", code, raw)
	}
	mustJSON(t, raw, &section)
	if section.Count != 1 || len(section.Songs) != 1 || section.Songs[0] != "alpha" {
		t.Fatalf("состав %+v", section)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/sections/"+section.ID+"/songs", map[string]any{
		"fbids": []string{"missing"},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("нет песни %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=Альфа&section_id="+section.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("поиск %d %s", code, raw)
	}
	hits := songHits(t, raw)
	if len(hits) != 1 || hits[0].FBID != "alpha" || hits[0].SectionTitle != "Воскресенье" {
		t.Fatalf("хит %+v", hits)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=уникальнаястрока&section_id="+section.ID, nil)
	if code != http.StatusOK || len(songHits(t, raw)) != 0 {
		t.Fatalf("чужая секция %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=Альфа&songbook_id="+other.DimensionID+"&section_id="+section.ID, nil)
	if code != http.StatusBadRequest && code != http.StatusNotFound {
		t.Fatalf("чужой сборник %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songbooks/tree", nil)
	if code != http.StatusOK {
		t.Fatalf("дерево %d %s", code, raw)
	}
	var tree []treeGroup
	mustJSON(t, raw, &tree)
	if len(tree) != 1 || tree[0].Group != "personal" || len(tree[0].Songbooks) != 1 || tree[0].Songbooks[0].Sections[0].Count != 1 {
		t.Fatalf("группы %+v", tree)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/communities", map[string]any{"name": "Преображение"})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var community dimensionView
	mustJSON(t, raw, &community)
	shared := createBook(t, srv, owner.Token, community.ID, map[string]any{"fbid": "shared", "title": "Общий"})
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songbooks/tree", nil)
	if code != http.StatusOK {
		t.Fatalf("дерево общины %d %s", code, raw)
	}
	mustJSON(t, raw, &tree)
	if len(tree) != 2 || tree[0].Group != "community" || tree[0].Name != "Преображение" || tree[0].Songbooks[0].ID != shared.ID {
		t.Fatalf("общинная группа %+v", tree)
	}

	code, raw = api(t, srv, owner.Token, http.MethodDelete, "/api/songbooks/"+book.ID+"/sections/"+section.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("удаление %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/songs/search?q=Альфа&section_id="+section.ID, nil)
	if code != http.StatusNotFound {
		t.Fatalf("удалённый раздел %d %s", code, raw)
	}
}

func TestSectionTitleIsUnique(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})

	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/sections", map[string]any{"title": "Служение"})
	if code != http.StatusOK {
		t.Fatalf("раздел %d %s", code, raw)
	}
	var first sectionView
	mustJSON(t, raw, &first)
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/sections", map[string]any{"title": " Служение "})
	if code != http.StatusConflict || !bytes.Contains(raw, []byte(`"раздел"`)) {
		t.Fatalf("повтор названия %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/sections", map[string]any{"title": "Дети"})
	if code != http.StatusOK {
		t.Fatalf("второй %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/songbooks/"+book.ID+"/sections/"+first.ID, map[string]any{"title": "Дети"})
	if code != http.StatusConflict || !bytes.Contains(raw, []byte(`"раздел"`)) {
		t.Fatalf("переименование в занятое %d %s", code, raw)
	}
}
