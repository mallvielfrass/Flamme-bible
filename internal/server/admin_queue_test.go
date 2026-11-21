package server

import (
	"net/http"
	"testing"
)

func TestSubmissionQueueAndTransitions(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	admin := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "К публикации", "short_name": "KP"})
	pushSong(t, srv, owner, book.ID, "song-a", "Тихая", "в тишине мы ждём")
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/songbooks/"+book.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("submit %d %s", code, raw)
	}
	mustJSON(t, raw, &book)
	if book.Status != "pending" {
		t.Fatalf("статус %s", book.Status)
	}

	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/submissions?sort=submitted", nil)
	if code != http.StatusOK {
		t.Fatalf("очередь %d %s", code, raw)
	}
	var queue struct {
		Submissions []submission `json:"submissions"`
		Total       int          `json:"total"`
		Counts      struct {
			Status map[string]int `json:"status"`
		} `json:"counts"`
	}
	mustJSON(t, raw, &queue)
	if queue.Total != 1 || queue.Submissions[0].AuthorEmail != "owner@example.com" || queue.Submissions[0].SongsCount != 1 {
		t.Fatalf("обогащение %+v", queue.Submissions)
	}
	if queue.Submissions[0].SubmittedAt == nil || queue.Submissions[0].CommunityName != "Личное" {
		t.Fatalf("дата или община %+v", queue.Submissions[0])
	}
	if queue.Counts.Status["pending"] != 1 {
		t.Fatalf("счётчики %+v", queue.Counts.Status)
	}
	if queue.Submissions[0].ShortName != "KP" {
		t.Fatalf("короткое имя %+v", queue.Submissions[0])
	}

	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/summary", nil)
	if code != http.StatusOK {
		t.Fatalf("сводка %d %s", code, raw)
	}
	var summary struct {
		PendingSongbooks int `json:"pending_songbooks"`
	}
	mustJSON(t, raw, &summary)
	if summary.PendingSongbooks != 1 {
		t.Fatalf("сводка %+v", summary)
	}

	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/songbooks/"+book.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("карточка %d %s", code, raw)
	}
	var card struct {
		Songs []songPreview `json:"songs"`
	}
	mustJSON(t, raw, &card)
	if len(card.Songs) != 1 || card.Songs[0].Title != "Тихая" {
		t.Fatalf("песни %+v", card.Songs)
	}

	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/revoke", map[string]any{})
	if code != http.StatusConflict {
		t.Fatalf("revoke из очереди %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/reject", map[string]any{"note": "дубль"})
	if code != http.StatusOK {
		t.Fatalf("reject %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/approve", nil)
	if code != http.StatusOK {
		t.Fatalf("approve из отказа %d %s", code, raw)
	}
	mustJSON(t, raw, &book)
	if book.Status != "approved" {
		t.Fatalf("после отказа %+v", book)
	}
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/songbooks/"+book.ID+"/reject", map[string]any{"note": "поздно"})
	if code != http.StatusConflict {
		t.Fatalf("reject одобренного %d %s", code, raw)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules", map[string]any{"short_name": "RST", "title": "Синодальный"})
	if code != http.StatusOK {
		t.Fatalf("модуль %d %s", code, raw)
	}
	var module moduleView
	mustJSON(t, raw, &module)
	code, raw = api(t, srv, admin.Token, http.MethodPost, "/api/admin/modules/"+module.ID+"/approve", nil)
	if code != http.StatusConflict {
		t.Fatalf("approve черновика %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/modules/"+module.ID+"/submit", nil)
	if code != http.StatusOK {
		t.Fatalf("submit модуля %d %s", code, raw)
	}
	code, raw = api(t, srv, admin.Token, http.MethodGet, "/api/admin/modules/"+module.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("карточка модуля %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/admin/songbooks/"+book.ID, nil)
	if code != http.StatusForbidden {
		t.Fatalf("владелец смотрит карточку %d %s", code, raw)
	}
}
