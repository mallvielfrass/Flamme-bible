package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPlansPersonalCommunityAndSync(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	member := registerUser(t, srv, "member@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	songbook := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Сборник"})

	items := []any{
		map[string]any{"kind": "song", "song_id": "song-1", "songbook_id": songbook.ID, "title": "Тихая"},
		map[string]any{
			"kind": "bible", "module_short_name": "RST", "module_short_name_second": "NRT",
			"book": 43, "chapter": 3, "verse": 16, "reference": "Ин 3:16", "quote": "мир",
		},
		map[string]any{"kind": "text", "caption": "Слайд", "reference": "хор", "quote": "Аминь", "class": "-1"},
	}
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/plans", map[string]any{
		"title": "Воскресенье", "items": items,
	})
	if code != http.StatusOK {
		t.Fatalf("план %d %s", code, raw)
	}
	var plan planView
	mustJSON(t, raw, &plan)
	if plan.Version != 1 || plan.OwnerUserID == nil || *plan.OwnerUserID != owner.UserID || plan.DimensionID != nil {
		t.Fatalf("личный план %+v", plan)
	}
	if len(plan.Items) != 3 || plan.Items[0].Kind != "song" || plan.Items[1].ModuleShortNameSecond != "NRT" || plan.Items[1].Book != 43 || plan.Items[2].Class != "-1" {
		t.Fatalf("пункты %+v", plan.Items)
	}

	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans?scope=personal", nil)
	if code != http.StatusOK {
		t.Fatalf("список %d %s", code, raw)
	}
	var listed []planView
	mustJSON(t, raw, &listed)
	if len(listed) != 1 || listed[0].ID != plan.ID {
		t.Fatalf("личные %+v", listed)
	}

	stale := plan.UpdatedAt.Add(time.Hour)
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/plans/"+plan.ID+"/push", map[string]any{
		"version": plan.Version, "updated_at": stale, "title": "чужое", "items": items,
	})
	if code != http.StatusConflict {
		t.Fatalf("устаревшее время %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/plans/"+plan.ID, map[string]any{
		"version": plan.Version, "updated_at": plan.UpdatedAt, "title": "Утро", "items": items,
	})
	if code != http.StatusOK {
		t.Fatalf("правка %d %s", code, raw)
	}
	mustJSON(t, raw, &plan)
	if plan.Version != 2 || plan.Title != "Утро" {
		t.Fatalf("после правки %+v", plan)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPut, "/api/plans/"+plan.ID, map[string]any{
		"version": 1, "title": "назад", "items": items,
	})
	if code != http.StatusConflict {
		t.Fatalf("старая версия %d %s", code, raw)
	}

	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans/"+plan.ID+"/pull?version=2&updated_at="+plan.UpdatedAt.Format(time.RFC3339Nano), nil)
	if code != http.StatusOK {
		t.Fatalf("pull %d %s", code, raw)
	}
	var same struct {
		Changed bool `json:"changed"`
	}
	mustJSON(t, raw, &same)
	if same.Changed {
		t.Fatalf("та же версия изменилась %s", raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans/"+plan.ID+"/pull?version=1", nil)
	if code != http.StatusOK {
		t.Fatalf("pull старый %d %s", code, raw)
	}
	var diff struct {
		Changed bool     `json:"changed"`
		Plan    planView `json:"plan"`
	}
	mustJSON(t, raw, &diff)
	if !diff.Changed || diff.Plan.Version != 2 {
		t.Fatalf("pull не отдал план %s", raw)
	}

	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/communities", map[string]any{"name": "Община"})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var community dimensionView
	mustJSON(t, raw, &community)
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/communities/"+community.ID+"/invites", map[string]any{"ttl_seconds": 3600})
	if code != http.StatusOK {
		t.Fatalf("приглашение %d %s", code, raw)
	}
	var invite struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &invite)
	code, raw = api(t, srv, member.Token, http.MethodPost, "/api/invites/"+invite.Token+"/accept", nil)
	if code != http.StatusOK {
		t.Fatalf("вступление %d %s", code, raw)
	}
	code, raw = api(t, srv, member.Token, http.MethodPost, "/api/plans", map[string]any{
		"title": "Общий", "dimension_id": community.ID, "items": items,
	})
	if code != http.StatusOK {
		t.Fatalf("план общины %d %s", code, raw)
	}
	var shared planView
	mustJSON(t, raw, &shared)
	if shared.DimensionID == nil || *shared.DimensionID != community.ID || shared.OwnerUserID != nil {
		t.Fatalf("область %+v", shared)
	}
	code, raw = api(t, srv, other.Token, http.MethodGet, "/api/plans/"+shared.ID, nil)
	if code != http.StatusNotFound {
		t.Fatalf("чужой план %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans?scope=community", nil)
	if code != http.StatusOK {
		t.Fatalf("общие %d %s", code, raw)
	}
	mustJSON(t, raw, &listed)
	if len(listed) != 1 || listed[0].ID != shared.ID {
		t.Fatalf("планы общины %+v", listed)
	}

	code, raw = api(t, srv, owner.Token, http.MethodDelete, "/api/plans/"+plan.ID+"?version=2", nil)
	if code != http.StatusOK {
		t.Fatalf("удаление %d %s", code, raw)
	}
	mustJSON(t, raw, &plan)
	if !plan.Deleted || plan.Version != 3 {
		t.Fatalf("tombstone %+v", plan)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans?scope=personal", nil)
	if code != http.StatusOK {
		t.Fatalf("после удаления %d %s", code, raw)
	}
	mustJSON(t, raw, &listed)
	if len(listed) != 0 {
		t.Fatalf("удалённый в списке %+v", listed)
	}
}

func TestPlanImportExport(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	payload := `<VisioBibleJournal>
  <Journal Name="Bible">
    <Item ModuleShortName="RST" ModuleShortNameSecond="NRT" Book="43" Chapter="3" Verse="16" Reference="Ин 3:16" Quote="мир" Class="-1"/>
  </Journal>
  <Journal Name="Songs">
    <Item Title="Тихая" SongID="song-1" Class="-1"/>
  </Journal>
  <Journal Name="Text">
    <Item Caption="Слайд" Reference="хор" Quote="Аминь" Class="-1"/>
  </Journal>
</VisioBibleJournal>`
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/plans/import", map[string]any{
		"format": "history", "title": "Импорт", "payload": payload,
	})
	if code != http.StatusOK {
		t.Fatalf("импорт %d %s", code, raw)
	}
	var plan planView
	mustJSON(t, raw, &plan)
	if len(plan.Items) != 3 || plan.Items[0].ModuleShortName != "RST" || plan.Items[0].ModuleShortNameSecond != "NRT" || plan.Items[0].Verse != 16 {
		t.Fatalf("библия %+v", plan.Items)
	}
	if plan.Items[1].Kind != "song" || plan.Items[1].SongID != "song-1" || plan.Items[1].Title != "Тихая" {
		t.Fatalf("песня %+v", plan.Items[1])
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans/"+plan.ID+"/export?format=vplt", nil)
	if code != http.StatusOK {
		t.Fatalf("vplt %d %s", code, raw)
	}
	text := string(raw)
	if strings.Contains(text, "ModuleShortName") || !strings.Contains(text, `Caption="RST"`) || !strings.Contains(text, `Caption="Слайд"`) {
		t.Fatalf("vplt %s", text)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans/"+plan.ID+"/export?format=history", nil)
	if code != http.StatusOK || !strings.Contains(string(raw), `ModuleShortName="RST"`) || !strings.Contains(string(raw), `Name="Bible"`) {
		t.Fatalf("history %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans/"+plan.ID+"/export?format=plandef", nil)
	if code != http.StatusOK || !strings.Contains(string(raw), "module_short_name=RST") || !strings.Contains(string(raw), "module_short_name_second=NRT") {
		t.Fatalf("plandef %d %s", code, raw)
	}
}
