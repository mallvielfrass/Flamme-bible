package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flammebible/internal/ai/provider"
)

func aiRun(t *testing.T, s *Server, userID, name string, args any) aiToolOutcome {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return s.runAITool(context.Background(), userID, provider.ToolCall{ID: "c1", Type: "function", Function: provider.FunctionCall{Name: name, Arguments: string(raw)}})
}

func aiPlanOut(t *testing.T, out aiToolOutcome) map[string]any {
	t.Helper()
	if !out.OK {
		t.Fatalf("инструмент не сработал %+v", out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out.Content), &got); err != nil {
		t.Fatalf("ответ %s: %v", out.Content, err)
	}
	return got
}

func aiItemTitles(got map[string]any) string {
	items, _ := got["items"].([]any)
	names := []string{}
	for _, raw := range items {
		item := raw.(map[string]any)
		name, _ := item["title"].(string)
		if name == "" {
			name, _ = item["caption"].(string)
		}
		names = append(names, name)
	}
	return strings.Join(names, ",")
}

// Личный план: создать, добавить песню и слайд, переставить, убрать, переименовать.
// Чужой пользователь тот же план не видит и не меняет.
func TestAIToolPlanWrite(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	book := createBook(t, srv, owner.Token, owner.DimensionID, map[string]any{"fbid": "book", "title": "Домашний"})
	pushSong(t, srv, owner, book.ID, "grace", "Благодать", "дивная благодать спасла меня")
	s := &Server{db: testPool}

	out := aiRun(t, s, owner.UserID, "create_plan", map[string]any{"title": "Воскресенье 29.11"})
	got := aiPlanOut(t, out)
	planID, _ := got["id"].(string)
	if got["status"] != "создан" || got["title"] != "Воскресенье 29.11" || out.Summary != "План создан: «Воскресенье 29.11»" {
		t.Fatalf("создание %+v", out)
	}
	if len(out.Cards) != 1 || out.Cards[0].Kind != "plan" || out.Cards[0].ID != planID || out.Cards[0].Note != "План создан" {
		t.Fatalf("карточка %+v", out.Cards)
	}

	got = aiPlanOut(t, aiRun(t, s, owner.UserID, "add_to_plan", map[string]any{"plan_id": planID, "song_ref": book.ID + ":grace"}))
	if aiItemTitles(got) != "Благодать" {
		t.Fatalf("песня %+v", got)
	}
	got = aiPlanOut(t, aiRun(t, s, owner.UserID, "add_to_plan", map[string]any{"plan_id": planID, "caption": "Объявления", "text": "Чай после служения", "position": 1}))
	if aiItemTitles(got) != "Объявления,Благодать" {
		t.Fatalf("слайд в начало %+v", got)
	}
	got = aiPlanOut(t, aiRun(t, s, owner.UserID, "add_to_plan", map[string]any{"plan_id": planID, "caption": "Молитва"}))
	if aiItemTitles(got) != "Объявления,Благодать,Молитва" {
		t.Fatalf("слайд в конец %+v", got)
	}
	got = aiPlanOut(t, aiRun(t, s, owner.UserID, "move_plan_item", map[string]any{"plan_id": planID, "from": 3, "to": 1}))
	if aiItemTitles(got) != "Молитва,Объявления,Благодать" {
		t.Fatalf("перестановка %+v", got)
	}
	got = aiPlanOut(t, aiRun(t, s, owner.UserID, "remove_from_plan", map[string]any{"plan_id": planID, "items": []int{2}}))
	if aiItemTitles(got) != "Молитва,Благодать" {
		t.Fatalf("удаление пункта %+v", got)
	}
	out = aiRun(t, s, owner.UserID, "rename_plan", map[string]any{"plan_id": planID, "title": "Утреннее служение"})
	got = aiPlanOut(t, out)
	if got["title"] != "Утреннее служение" || out.Summary != "План изменён: «Утреннее служение»" || out.Cards[0].Note != "План изменён" {
		t.Fatalf("переименование %+v", out)
	}

	// Обычный API видит то же самое.
	code, raw := api(t, srv, owner.Token, http.MethodGet, "/api/plans/"+planID, nil)
	if code != http.StatusOK {
		t.Fatalf("план %d %s", code, raw)
	}
	var plan planView
	mustJSON(t, raw, &plan)
	if plan.Title != "Утреннее служение" || len(plan.Items) != 2 || plan.Items[1].Kind != "song" || plan.Items[1].SongID != "grace" || plan.Items[1].SongbookID != book.ID {
		t.Fatalf("план после ИИ %+v", plan)
	}

	// Неверные аргументы ничего не меняют.
	for name, args := range map[string]map[string]any{
		"add_to_plan":      {"plan_id": planID, "song_ref": book.ID + ":grace", "caption": "лишнее"},
		"remove_from_plan": {"plan_id": planID, "items": []int{5}},
		"move_plan_item":   {"plan_id": planID, "from": 1, "to": 9},
		"rename_plan":      {"plan_id": planID, "title": " "},
	} {
		if out := aiRun(t, s, owner.UserID, name, args); out.OK || !strings.Contains(out.Content, "неверные аргументы") {
			t.Fatalf("%s: %+v", name, out)
		}
	}
	if out := aiRun(t, s, owner.UserID, "remove_from_plan", map[string]any{"plan_id": planID, "items": []int{1, 2, 1, 2}}); out.OK || !strings.Contains(out.Content, "не больше 3") {
		t.Fatalf("много пунктов %+v", out)
	}
	if out := aiRun(t, s, owner.UserID, "add_to_plan", map[string]any{"plan_id": planID, "passage_ref": "RST:43:3:16"}); out.OK || !strings.Contains(out.Content, "не найдено") {
		t.Fatalf("несуществующее место %+v", out)
	}

	// Чужой пользователь: план не найден, чужую песню не добавить.
	for name, args := range map[string]map[string]any{
		"add_to_plan":      {"plan_id": planID, "caption": "чужое"},
		"remove_from_plan": {"plan_id": planID, "items": []int{1}},
		"move_plan_item":   {"plan_id": planID, "from": 1, "to": 2},
		"rename_plan":      {"plan_id": planID, "title": "чужое"},
	} {
		if out := aiRun(t, s, other.UserID, name, args); out.OK || !strings.Contains(out.Content, "не найдено") {
			t.Fatalf("чужой %s: %+v", name, out)
		}
	}
	otherPlan := aiPlanOut(t, aiRun(t, s, other.UserID, "create_plan", map[string]any{"title": "Свой"}))
	if out := aiRun(t, s, other.UserID, "add_to_plan", map[string]any{"plan_id": otherPlan["id"], "song_ref": book.ID + ":grace"}); out.OK || !strings.Contains(out.Content, "не найдено") {
		t.Fatalf("чужая песня %+v", out)
	}
	if code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/plans/"+planID, nil); code != http.StatusOK {
		t.Fatalf("план %d %s", code, raw)
	}
	mustJSON(t, raw, &plan)
	if plan.Title != "Утреннее служение" || len(plan.Items) != 2 {
		t.Fatalf("план изменён чужим %+v", plan)
	}
}

// План общины: участник создаёт и меняет, посторонний и исключённый — нет.
func TestAIToolPlanWriteCommunity(t *testing.T) {
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	member := registerUser(t, srv, "member@example.com", "secret-pass", nil)
	other := registerUser(t, srv, "other@example.com", "secret-pass", nil)
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/communities", map[string]any{"name": "Община"})
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
	if code, raw = api(t, srv, member.Token, http.MethodPost, "/api/invites/"+invite.Token+"/accept", nil); code != http.StatusOK {
		t.Fatalf("вступление %d %s", code, raw)
	}
	s := &Server{db: testPool}

	out := aiRun(t, s, member.UserID, "create_plan", map[string]any{"title": "Общий", "dimension_id": community.ID})
	got := aiPlanOut(t, out)
	planID, _ := got["id"].(string)
	if got["dimension_id"] != community.ID || out.Cards[0].DimensionID != community.ID {
		t.Fatalf("план общины %+v", out)
	}
	if out := aiRun(t, s, other.UserID, "create_plan", map[string]any{"title": "Чужой", "dimension_id": community.ID}); out.OK {
		t.Fatalf("посторонний создал план общины %+v", out)
	}
	if out := aiRun(t, s, other.UserID, "add_to_plan", map[string]any{"plan_id": planID, "caption": "чужое"}); out.OK || !strings.Contains(out.Content, "не найдено") {
		t.Fatalf("посторонний изменил план общины %+v", out)
	}
	aiPlanOut(t, aiRun(t, s, owner.UserID, "add_to_plan", map[string]any{"plan_id": planID, "caption": "Приветствие"}))

	if _, err := testPool.Exec(context.Background(), `UPDATE memberships SET excluded = true WHERE dimension_id = $1 AND user_id = $2`, community.ID, member.UserID); err != nil {
		t.Fatal(err)
	}
	if out := aiRun(t, s, member.UserID, "rename_plan", map[string]any{"plan_id": planID, "title": "Новое"}); out.OK || !strings.Contains(out.Content, "нет доступа") {
		t.Fatalf("исключённый изменил план %+v", out)
	}
}
