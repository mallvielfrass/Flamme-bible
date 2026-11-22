package server

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Инструменты записи планов (SPEC 4.2). Запись идёт через insertPlan и savePlan,
// поэтому доступ проверяется ровно так же, как в обычном API: личный план —
// только владелец, план общины — неисключённый участник.

const aiRemoveLimit = 3

// errAIRemoveLimit — больше трёх пунктов за раз ИИ не убирает (SPEC 4.2).
var errAIRemoveLimit = errors.New("за раз можно убрать не больше 3 пунктов")

// aiPlanSummary — компактный вид плана для модели: номера пунктов n начинаются с 1.
func aiPlanSummary(plan planView) map[string]any {
	type itemOut struct {
		N         int    `json:"n"`
		Kind      string `json:"kind"`
		Title     string `json:"title,omitempty"`
		Reference string `json:"reference,omitempty"`
		Caption   string `json:"caption,omitempty"`
	}
	items := make([]itemOut, 0, len(plan.Items))
	for i, item := range plan.Items {
		items = append(items, itemOut{N: i + 1, Kind: item.Kind, Title: item.Title, Reference: item.Reference, Caption: item.Caption})
	}
	out := map[string]any{"id": plan.ID, "title": plan.Title, "version": plan.Version, "items": items}
	if plan.DimensionID != nil {
		out["dimension_id"] = *plan.DimensionID
	}
	return out
}

func aiPlanChanged(plan planView, status string) (any, string, error) {
	out := aiPlanSummary(plan)
	out["status"] = status
	label := "План изменён"
	if status == "создан" {
		label = "План создан"
	}
	return out, label + ": «" + clipRunes(plan.Title, 60) + "»", nil
}

// planCards — карточка «План изменён» для интерфейса (ai_messages.cards).
func planCards(result any) []aiCard {
	data, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	id, _ := data["id"].(string)
	if !validUUID(id) {
		return nil
	}
	title, _ := data["title"].(string)
	status, _ := data["status"].(string)
	dim, _ := data["dimension_id"].(string)
	note := "План изменён"
	if status == "создан" {
		note = "План создан"
	}
	return []aiCard{{Kind: "plan", ID: id, Title: title, Note: note, DimensionID: dim}}
}

func (s *Server) aiCreatePlan(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		Title       string `json:"title"`
		DimensionID string `json:"dimension_id"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	plan, err := s.insertPlan(ctx, userID, in.Title, strings.TrimSpace(in.DimensionID), nil)
	if err != nil {
		return nil, "", err
	}
	return aiPlanChanged(plan, "создан")
}

func (s *Server) aiAddToPlan(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		PlanID     string `json:"plan_id"`
		SongRef    string `json:"song_ref"`
		PassageRef string `json:"passage_ref"`
		Text       string `json:"text"`
		Caption    string `json:"caption"`
		Position   int    `json:"position"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	item, err := s.aiPlanItem(ctx, userID, strings.TrimSpace(in.SongRef), strings.TrimSpace(in.PassageRef), in.Text, in.Caption)
	if err != nil {
		return nil, "", err
	}
	plan, err := s.aiEditPlan(ctx, userID, in.PlanID, func(body *planWrite) error {
		pos := in.Position - 1
		if pos < 0 || pos > len(body.Items) {
			pos = len(body.Items)
		}
		items := make([]planItem, 0, len(body.Items)+1)
		items = append(items, body.Items[:pos]...)
		items = append(items, item)
		body.Items = append(items, body.Items[pos:]...)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return aiPlanChanged(plan, "изменён")
}

// aiPlanItem собирает ровно один пункт. Песня и место Писания читаются с теми же
// правилами доступа, что у поиска: чужую песню в план не добавить.
func (s *Server) aiPlanItem(ctx context.Context, userID, songRef, passageRef, text, caption string) (planItem, error) {
	text, caption = strings.TrimSpace(text), strings.TrimSpace(caption)
	kinds := 0
	for _, set := range []bool{songRef != "", passageRef != "", text != "" || caption != ""} {
		if set {
			kinds++
		}
	}
	if kinds != 1 {
		return planItem{}, errRequest
	}
	switch {
	case songRef != "":
		bookID, fbid, ok := strings.Cut(songRef, ":")
		if !ok || !validUUID(bookID) || fbid == "" {
			return planItem{}, errNotFound
		}
		song, err := s.readSong(ctx, userID, bookID, fbid)
		if err != nil {
			return planItem{}, err
		}
		return planItem{Kind: "song", SongID: fbid, SongbookID: bookID, Title: song.Title}, nil
	case passageRef != "":
		return s.passageItem(ctx, passageRef)
	default:
		return planItem{Kind: "text", Caption: caption, Quote: text}, nil
	}
}

// passageRef — короткий идентификатор отрывка для модели: «RST:43:3:16».
func passageRef(hit bibleHit) string {
	return hit.ModuleShortName + ":" + strconv.Itoa(hit.Book) + ":" + strconv.Itoa(hit.Chapter) + ":" + strconv.Itoa(hit.VerseStart)
}

func (s *Server) passageItem(ctx context.Context, ref string) (planItem, error) {
	parts := strings.Split(ref, ":")
	if len(parts) != 4 || !validModuleName(parts[0]) {
		return planItem{}, errNotFound
	}
	nums := make([]int, 3)
	for i, raw := range parts[1:] {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			return planItem{}, errNotFound
		}
		nums[i] = n
	}
	var reference, body string
	err := s.db.QueryRow(ctx, `
		SELECT p.reference, p.body
		FROM bible_passages p
		JOIN modules m ON m.id = p.module_id
		WHERE m.status = 'approved' AND m.short_name = $1
		  AND p.book = $2 AND p.chapter = $3 AND p.verse_start = $4
		ORDER BY p.verse_end
		LIMIT 1
	`, parts[0], nums[0], nums[1], nums[2]).Scan(&reference, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return planItem{}, errNotFound
	}
	if err != nil {
		return planItem{}, err
	}
	return planItem{
		Kind: "bible", ModuleShortName: parts[0], Book: nums[0], Chapter: nums[1], Verse: nums[2],
		Reference: clipRunes(reference, 300), Quote: clipRunes(body, 4000),
	}, nil
}

func (s *Server) aiRemoveFromPlan(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		PlanID string `json:"plan_id"`
		Items  []int  `json:"items"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	if len(in.Items) == 0 {
		return nil, "", errRequest
	}
	if len(in.Items) > aiRemoveLimit {
		return nil, "", errAIRemoveLimit
	}
	plan, err := s.aiEditPlan(ctx, userID, in.PlanID, func(body *planWrite) error {
		drop := map[int]bool{}
		for _, n := range in.Items {
			if n < 1 || n > len(body.Items) {
				return errRequest
			}
			drop[n-1] = true
		}
		kept := make([]planItem, 0, len(body.Items))
		for i, item := range body.Items {
			if !drop[i] {
				kept = append(kept, item)
			}
		}
		body.Items = kept
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return aiPlanChanged(plan, "изменён")
}

func (s *Server) aiMovePlanItem(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		PlanID string `json:"plan_id"`
		From   int    `json:"from"`
		To     int    `json:"to"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	plan, err := s.aiEditPlan(ctx, userID, in.PlanID, func(body *planWrite) error {
		n := len(body.Items)
		if in.From < 1 || in.From > n || in.To < 1 || in.To > n {
			return errRequest
		}
		item := body.Items[in.From-1]
		rest := append(append([]planItem{}, body.Items[:in.From-1]...), body.Items[in.From:]...)
		items := make([]planItem, 0, n)
		items = append(items, rest[:in.To-1]...)
		items = append(items, item)
		body.Items = append(items, rest[in.To-1:]...)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return aiPlanChanged(plan, "изменён")
}

func (s *Server) aiRenamePlan(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		PlanID string `json:"plan_id"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, "", errRequest
	}
	plan, err := s.aiEditPlan(ctx, userID, in.PlanID, func(body *planWrite) error {
		body.Title = title
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return aiPlanChanged(plan, "изменён")
}

// aiEditPlan читает план, применяет правку и сохраняет её через savePlan с версией.
// Если план успели изменить, перечитывает и пробует ещё раз; второй конфликт —
// ошибка «перечитай план».
func (s *Server) aiEditPlan(ctx context.Context, userID, planID string, edit func(*planWrite) error) (planView, error) {
	planID = strings.TrimSpace(planID)
	for attempt := 0; ; attempt++ {
		plan, err := s.readPlan(ctx, userID, planID)
		if err != nil {
			return planView{}, err
		}
		if plan.Deleted {
			return planView{}, errNotFound
		}
		body := planWrite{Title: plan.Title, Version: plan.Version, Items: append([]planItem{}, plan.Items...)}
		if err := edit(&body); err != nil {
			return planView{}, err
		}
		view, err := s.savePlan(ctx, userID, plan.ID, body, false)
		if errors.Is(err, errConflict) && attempt == 0 {
			continue
		}
		return view, err
	}
}
