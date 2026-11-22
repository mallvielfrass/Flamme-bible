package server

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"flammebible/internal/ai/provider"

	"github.com/jackc/pgx/v5"
)

// Инструменты чтения для чата (SPEC, раздел 4.1). Каждый идёт через те же функции
// и проверки доступа, что и обычный API, поэтому своих правил доступа здесь нет.

const (
	aiToolResultRunes = 12000
	aiSongTextRunes   = 1500
	aiPlansLimit      = 30
)

var aiTools = []provider.Tool{
	aiTool("search_songs", "Найти песни в сборниках пользователя по словам и по смыслу.", `{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Слова из названия или текста либо тема песни"},
			"mode": {"type": "string", "enum": ["hybrid", "words", "meaning"], "description": "По умолчанию hybrid"},
			"songbook_id": {"type": "string", "description": "uuid сборника из list_songbooks"},
			"section_id": {"type": "string", "description": "uuid раздела сборника"},
			"dimension_id": {"type": "string", "description": "uuid пространства (общины)"},
			"limit": {"type": "integer", "minimum": 1, "maximum": 10}
		},
		"required": ["query"]
	}`),
	aiTool("get_song", "Полный текст песни по song_ref из search_songs.", `{
		"type": "object",
		"properties": {
			"song_ref": {"type": "string", "description": "songbook_id:fbid"}
		},
		"required": ["song_ref"]
	}`),
	aiTool("list_songbooks", "Сборники песен, доступные пользователю, и их разделы.", `{
		"type": "object",
		"properties": {}
	}`),
	aiTool("search_bible", "Найти места Писания по смыслу.", `{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Тема или слова"},
			"module": {"type": "string", "description": "Краткое имя модуля Библии (например RST) или его uuid"},
			"limit": {"type": "integer", "minimum": 1, "maximum": 8}
		},
		"required": ["query"]
	}`),
	aiTool("list_plans", "Планы служений пользователя.", `{
		"type": "object",
		"properties": {
			"scope": {"type": "string", "enum": ["all", "personal", "community"]},
			"dimension_id": {"type": "string", "description": "uuid пространства (общины)"},
			"query": {"type": "string", "description": "Фильтр по названию"}
		}
	}`),
	aiTool("get_plan", "План служения с пунктами.", `{
		"type": "object",
		"properties": {
			"plan_id": {"type": "string", "description": "uuid плана"}
		},
		"required": ["plan_id"]
	}`),
}

func aiTool(name, description, schema string) provider.Tool {
	return provider.Tool{
		Type: "function",
		Function: provider.ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  json.RawMessage(schema),
		},
	}
}

// aiToolOutcome — ответ инструмента для модели и короткая строка для интерфейса.
type aiToolOutcome struct {
	Content string
	Summary string
	OK      bool
}

// runAITool выполняет вызов от имени userID. Ошибка не прерывает ответ:
// модель получает {"error": …} и может переспросить или ответить сама.
func (s *Server) runAITool(ctx context.Context, userID string, call provider.ToolCall) aiToolOutcome {
	args := []byte(strings.TrimSpace(call.Function.Arguments))
	if len(args) == 0 {
		args = []byte("{}")
	}
	result, summary, err := s.callAITool(ctx, userID, call.Function.Name, args)
	if err != nil {
		text := aiToolError(err)
		raw, _ := json.Marshal(map[string]string{"error": text})
		return aiToolOutcome{Content: string(raw), Summary: text}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return aiToolOutcome{Content: `{"error":"ответ инструмента"}`, Summary: "ошибка"}
	}
	return aiToolOutcome{Content: clipRunes(string(raw), aiToolResultRunes), Summary: summary, OK: true}
}

func aiToolError(err error) string {
	switch {
	case errors.Is(err, errNotFound):
		return "не найдено"
	case errors.Is(err, errRequest):
		return "неверные аргументы"
	case errors.Is(err, errSemanticOff):
		return "смысловой поиск недоступен"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "остановлено"
	default:
		return "не получилось выполнить"
	}
}

func (s *Server) callAITool(ctx context.Context, userID, name string, args []byte) (any, string, error) {
	switch name {
	case "search_songs":
		return s.aiSearchSongs(ctx, userID, args)
	case "get_song":
		return s.aiGetSong(ctx, userID, args)
	case "list_songbooks":
		return s.aiListSongbooks(ctx, userID)
	case "search_bible":
		return s.aiSearchBible(ctx, args)
	case "list_plans":
		return s.aiListPlans(ctx, userID, args)
	case "get_plan":
		return s.aiGetPlan(ctx, userID, args)
	default:
		return nil, "", errors.New("нет такого инструмента")
	}
}

// aiToolLabel — подпись для карточки «Ищу в песнях: «благодать»».
func aiToolLabel(call provider.ToolCall) string {
	var args struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	q := clipRunes(strings.TrimSpace(args.Query), 60)
	switch call.Function.Name {
	case "search_songs":
		return "Ищу в песнях: «" + q + "»"
	case "get_song":
		return "Читаю песню"
	case "list_songbooks":
		return "Смотрю сборники"
	case "search_bible":
		return "Ищу в Писании: «" + q + "»"
	case "list_plans":
		return "Смотрю планы"
	case "get_plan":
		return "Читаю план"
	default:
		return call.Function.Name
	}
}

type aiSongOut struct {
	SongRef  string `json:"song_ref"`
	Title    string `json:"title"`
	Songbook string `json:"songbook"`
	Section  string `json:"section,omitempty"`
	Fragment string `json:"fragment,omitempty"`
}

func (s *Server) aiSearchSongs(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		Query       string `json:"query"`
		Mode        string `json:"mode"`
		SongbookID  string `json:"songbook_id"`
		SectionID   string `json:"section_id"`
		DimensionID string `json:"dimension_id"`
		Limit       int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	q := strings.TrimSpace(in.Query)
	if q == "" || utf8.RuneCountInString(q) > 200 {
		return nil, "", errRequest
	}
	if in.Limit < 1 || in.Limit > 10 {
		in.Limit = 8
	}
	book, err := optionalUUID(in.SongbookID)
	if err != nil {
		return nil, "", err
	}
	section, err := optionalUUID(in.SectionID)
	if err != nil {
		return nil, "", err
	}
	dim, err := optionalUUID(in.DimensionID)
	if err != nil {
		return nil, "", err
	}
	out := []aiSongOut{}
	seen := map[string]bool{}
	addSong := func(hit songHit, fragment string) {
		ref := hit.SongbookID + ":" + hit.FBID
		if seen[ref] || len(out) >= in.Limit {
			return
		}
		seen[ref] = true
		out = append(out, aiSongOut{SongRef: ref, Title: hit.Title, Songbook: hit.SongbookTitle, Section: hit.SectionTitle, Fragment: fragment})
	}
	if in.Mode != "meaning" {
		hits, err := s.findSongs(ctx, userID, q, book, dim, section, in.Limit, 0)
		if err != nil {
			return nil, "", err
		}
		for _, hit := range hits {
			addSong(hit, "")
		}
	}
	semantic := "off"
	if in.Mode != "words" && s.embedder != nil && !s.blockActiveEmbed(ctx) {
		hits, err := s.findSongsSemantic(ctx, userID, q, book, dim, section, in.Limit)
		switch {
		case err == nil:
			semantic = "on"
			for _, hit := range hits {
				addSong(hit.songHit, hit.Fragment)
			}
		case !errors.Is(err, errSemanticOff):
			return nil, "", err
		}
	}
	return map[string]any{"songs": out, "semantic": semantic}, countSummary(len(out)), nil
}

func (s *Server) aiGetSong(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		SongRef string `json:"song_ref"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	bookID, fbid, ok := strings.Cut(strings.TrimSpace(in.SongRef), ":")
	if !ok || !validUUID(bookID) || fbid == "" {
		return nil, "", errNotFound
	}
	song, err := s.readSong(ctx, userID, bookID, fbid)
	if err != nil {
		return nil, "", err
	}
	var text strings.Builder
	for _, part := range song.Parts {
		if text.Len() > 0 {
			text.WriteString("\n\n")
		}
		if part.Kind != "" {
			text.WriteString(part.Kind + ":\n")
		}
		text.WriteString(part.Text)
	}
	return map[string]any{
		"song_ref": in.SongRef,
		"title":    song.Title,
		"author":   song.Author,
		"songbook": song.Songbook,
		"text":     clipRunes(text.String(), aiSongTextRunes),
	}, song.Title, nil
}

type aiSong struct {
	Title    string
	Author   string
	Songbook string
	Parts    []partDTO
}

// readSong — одна песня с тем же правилом доступа, что у поиска (songAccessWhere).
func (s *Server) readSong(ctx context.Context, userID, bookID, fbid string) (aiSong, error) {
	var song aiSong
	var parts []byte
	err := s.db.QueryRow(ctx, `
		SELECT s.title, s.author, b.title, s.parts
		FROM songs s
		JOIN songbooks b ON b.id = s.songbook_id
		WHERE `+songAccessWhere("$1", "NULL", "$2")+`
		  AND s.fbid = $3
	`, bookID, userID, fbid).Scan(&song.Title, &song.Author, &song.Songbook, &parts)
	if errors.Is(err, pgx.ErrNoRows) {
		return aiSong{}, errNotFound
	}
	if err != nil {
		return aiSong{}, err
	}
	// Повреждённые части не мешают отдать название.
	_ = json.Unmarshal(parts, &song.Parts)
	return song, nil
}

func (s *Server) aiListSongbooks(ctx context.Context, userID string) (any, string, error) {
	tree, err := s.loadTree(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	type sectionOut struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	type bookOut struct {
		ID       string       `json:"id"`
		Title    string       `json:"title"`
		Sections []sectionOut `json:"sections,omitempty"`
	}
	type groupOut struct {
		Group       string    `json:"group"`
		Name        string    `json:"name"`
		DimensionID string    `json:"dimension_id,omitempty"`
		Songbooks   []bookOut `json:"songbooks"`
	}
	out := []groupOut{}
	books := 0
	for _, group := range tree {
		g := groupOut{Group: group.Group, Name: group.Name, DimensionID: group.DimensionID, Songbooks: []bookOut{}}
		for _, book := range group.Songbooks {
			b := bookOut{ID: book.ID, Title: book.Title}
			for _, sec := range book.Sections {
				b.Sections = append(b.Sections, sectionOut{ID: sec.ID, Title: sec.Title})
			}
			g.Songbooks = append(g.Songbooks, b)
			books++
		}
		out = append(out, g)
	}
	return map[string]any{"groups": out}, countSummary(books), nil
}

func (s *Server) aiSearchBible(ctx context.Context, raw []byte) (any, string, error) {
	var in struct {
		Query  string `json:"query"`
		Module string `json:"module"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	q := strings.TrimSpace(in.Query)
	if q == "" || utf8.RuneCountInString(q) > 200 {
		return nil, "", errRequest
	}
	if in.Limit < 1 || in.Limit > 8 {
		in.Limit = 5
	}
	moduleID, shortName, err := bibleModuleFilter(in.Module)
	if err != nil {
		return nil, "", err
	}
	hits, err := s.findPassages(ctx, q, moduleID, shortName, in.Limit)
	if err != nil {
		return nil, "", err
	}
	type passageOut struct {
		Reference string `json:"reference"`
		Module    string `json:"module"`
		Text      string `json:"text"`
	}
	out := make([]passageOut, 0, len(hits))
	for _, hit := range hits {
		out = append(out, passageOut{Reference: hit.Reference, Module: hit.ModuleShortName, Text: hit.Text})
	}
	return map[string]any{"passages": out}, countSummary(len(out)), nil
}

func (s *Server) aiListPlans(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		Scope       string `json:"scope"`
		DimensionID string `json:"dimension_id"`
		Query       string `json:"query"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	ids, err := s.planIDs(ctx, userID, in.Scope, strings.TrimSpace(in.DimensionID))
	if err != nil {
		return nil, "", err
	}
	type planOut struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Version    int64  `json:"version"`
		ItemsCount int    `json:"items_count"`
		UpdatedAt  string `json:"updated_at"`
		Community  bool   `json:"community"`
	}
	filter := strings.ToLower(strings.TrimSpace(in.Query))
	out := []planOut{}
	for _, id := range ids {
		if len(out) >= aiPlansLimit {
			break
		}
		plan, err := s.readPlan(ctx, userID, id)
		if err != nil {
			return nil, "", err
		}
		if filter != "" && !strings.Contains(strings.ToLower(plan.Title), filter) {
			continue
		}
		out = append(out, planOut{
			ID: plan.ID, Title: plan.Title, Version: plan.Version, ItemsCount: len(plan.Items),
			UpdatedAt: plan.UpdatedAt.Format("2006-01-02 15:04"), Community: plan.DimensionID != nil,
		})
	}
	return map[string]any{"plans": out}, countSummary(len(out)), nil
}

func (s *Server) aiGetPlan(ctx context.Context, userID string, raw []byte) (any, string, error) {
	var in struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, "", errRequest
	}
	plan, err := s.readPlan(ctx, userID, strings.TrimSpace(in.PlanID))
	if err != nil {
		return nil, "", err
	}
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
	return map[string]any{"id": plan.ID, "title": plan.Title, "version": plan.Version, "items": items}, plan.Title, nil
}

func optionalUUID(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !validUUID(raw) {
		return nil, errRequest
	}
	return raw, nil
}

func countSummary(n int) string {
	if n == 0 {
		return "ничего не найдено"
	}
	return "найдено: " + strconv.Itoa(n)
}
