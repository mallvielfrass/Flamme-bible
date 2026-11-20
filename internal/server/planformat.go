package server

import (
	"encoding/json"
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/encoding/charmap"
)

// В plan.vplt VisioBible сохраняет только Caption, Reference, Quote и Class.
// Незнакомые атрибуты программа стирает при следующем сохранении, поэтому
// книга, глава и стих едут в History.ini и PlanDef.ini, а не в vplt.

var moduleNameRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_+-]{0,31}$`)

func validModuleName(name string) bool {
	return moduleNameRe.MatchString(name)
}

type vpltDoc struct {
	XMLName  xml.Name      `xml:"VisioBibleJournal"`
	Journals []vpltJournal `xml:"Journal"`
}

type vpltJournal struct {
	Name  string     `xml:"Name,attr"`
	Items []vpltItem `xml:"Item"`
}

type vpltItem struct {
	Caption   string `xml:"Caption,attr"`
	Reference string `xml:"Reference,attr"`
	Quote     string `xml:"Quote,attr"`
	Class     string `xml:"Class,attr"`
}

type historyDoc struct {
	XMLName  xml.Name         `xml:"VisioBibleJournal"`
	Journals []historyJournal `xml:"Journal"`
}

type historyJournal struct {
	Name  string        `xml:"Name,attr"`
	Items []historyItem `xml:"Item"`
}

type historyItem struct {
	Caption               string `xml:"Caption,attr,omitempty"`
	Reference             string `xml:"Reference,attr,omitempty"`
	Quote                 string `xml:"Quote,attr,omitempty"`
	Class                 string `xml:"Class,attr,omitempty"`
	ModuleShortName       string `xml:"ModuleShortName,attr,omitempty"`
	ModuleShortNameSecond string `xml:"ModuleShortNameSecond,attr,omitempty"`
	Book                  string `xml:"Book,attr,omitempty"`
	Chapter               string `xml:"Chapter,attr,omitempty"`
	Verse                 string `xml:"Verse,attr,omitempty"`
	SongID                string `xml:"SongID,attr,omitempty"`
	SongbookID            string `xml:"SongbookID,attr,omitempty"`
	Title                 string `xml:"Title,attr,omitempty"`
	Module                string `xml:"Module,attr,omitempty"`
	ModuleSecond          string `xml:"ModuleSecond,attr,omitempty"`
}

func parsePlanPayload(format string, data []byte) (string, []planItem, error) {
	text, err := decodePlanText(data)
	if err != nil {
		return "", nil, err
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return parsePlanJSON(text)
	case "vplt":
		items, err := parseVPLT(text)
		return "", items, err
	case "history":
		items, err := parseHistory(text)
		return "", items, err
	case "plandef", "plan_def", "ini":
		return parsePlanDef(text)
	default:
		return "", nil, errRequest
	}
}

func formatPlan(format string, view planView) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "vplt":
		raw, err := formatVPLT(view.Items)
		return raw, "application/xml; charset=utf-8", err
	case "history":
		raw, err := formatHistory(view.Items)
		return raw, "application/xml; charset=utf-8", err
	case "plandef", "plan_def", "ini":
		return formatPlanDef(view.Title, view.Items), "text/plain; charset=utf-8", nil
	default:
		return "", "", errRequest
	}
}

func decodePlanText(data []byte) (string, error) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	decoded, err := charmap.Windows1251.NewDecoder().Bytes(data)
	if err != nil {
		return "", errRequest
	}
	return string(decoded), nil
}

func parsePlanJSON(text string) (string, []planItem, error) {
	var body struct {
		Title string     `json:"title"`
		Items []planItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return "", nil, errRequest
	}
	if body.Items == nil {
		body.Items = []planItem{}
	}
	return body.Title, body.Items, nil
}

func parseVPLT(text string) ([]planItem, error) {
	var doc vpltDoc
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, errRequest
	}
	return itemsFromVPLT(doc)
}

func itemsFromVPLT(doc vpltDoc) ([]planItem, error) {
	var items []planItem
	for _, journal := range doc.Journals {
		kind, err := kindFromJournal(journal.Name)
		if err != nil {
			continue
		}
		for _, raw := range journal.Items {
			item, err := itemFromFlat(kind, raw.Caption, raw.Reference, raw.Quote, raw.Class, historyItem{})
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	if items == nil {
		items = []planItem{}
	}
	return items, nil
}

func parseHistory(text string) ([]planItem, error) {
	var doc historyDoc
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, errRequest
	}
	var items []planItem
	for _, journal := range doc.Journals {
		kind, err := kindFromJournal(journal.Name)
		if err != nil {
			continue
		}
		for _, raw := range journal.Items {
			item, err := itemFromFlat(kind, raw.Caption, raw.Reference, raw.Quote, raw.Class, raw)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	if items == nil {
		items = []planItem{}
	}
	return items, nil
}

func itemFromFlat(kind, caption, reference, quote, class string, extra historyItem) (planItem, error) {
	item := planItem{
		Kind:                  kind,
		Caption:               caption,
		Reference:             reference,
		Quote:                 quote,
		Class:                 class,
		Title:                 extra.Title,
		SongID:                extra.SongID,
		SongbookID:            extra.SongbookID,
		ModuleShortName:       firstNonEmpty(extra.ModuleShortName, extra.Module),
		ModuleShortNameSecond: firstNonEmpty(extra.ModuleShortNameSecond, extra.ModuleSecond),
	}
	var err error
	if item.Book, err = mustCount(extra.Book); err != nil {
		return item, err
	}
	if item.Chapter, err = mustCount(extra.Chapter); err != nil {
		return item, err
	}
	if item.Verse, err = mustCount(extra.Verse); err != nil {
		return item, err
	}
	switch kind {
	case "song":
		if item.SongID == "" {
			item.SongID = quote
		}
		if item.Title == "" {
			item.Title = caption
		}
		if item.SongbookID == "" && validUUID(reference) {
			item.SongbookID = reference
		}
	case "bible":
		if item.ModuleShortName == "" && validModuleName(caption) {
			item.ModuleShortName = caption
		}
		if item.Class == "" {
			item.Class = "-1"
		}
	case "text":
		if item.Class == "" {
			item.Class = "-1"
		}
	}
	return item, nil
}

func mustCount(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, errRequest
	}
	return n, nil
}

func kindFromJournal(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "text":
		return "text", nil
	case "bible":
		return "bible", nil
	case "songs", "song":
		return "song", nil
	default:
		return "", errRequest
	}
}

func journalName(kind string) string {
	switch kind {
	case "bible":
		return "Bible"
	case "song":
		return "Songs"
	default:
		return "Text"
	}
}

func formatVPLT(items []planItem) (string, error) {
	doc := vpltDoc{}
	for _, item := range items {
		doc.Journals = append(doc.Journals, vpltJournal{
			Name: journalName(item.Kind),
			Items: []vpltItem{{
				Caption:   vpltCaption(item),
				Reference: vpltReference(item),
				Quote:     vpltQuote(item),
				Class:     vpltClass(item),
			}},
		})
	}
	return marshalXML(doc)
}

func vpltCaption(item planItem) string {
	if item.Kind == "song" {
		return item.Title
	}
	if item.Kind == "bible" && item.Caption == "" {
		return item.ModuleShortName
	}
	return item.Caption
}

func vpltReference(item planItem) string {
	if item.Kind == "song" {
		return item.SongbookID
	}
	if item.Reference != "" {
		return item.Reference
	}
	if item.Kind == "bible" && item.Book > 0 {
		return strconv.Itoa(item.Book) + " " + strconv.Itoa(item.Chapter) + ":" + strconv.Itoa(item.Verse)
	}
	return ""
}

func vpltQuote(item planItem) string {
	if item.Kind == "song" {
		return item.SongID
	}
	return item.Quote
}

func vpltClass(item planItem) string {
	if item.Class == "" {
		return "-1"
	}
	return item.Class
}

func formatHistory(items []planItem) (string, error) {
	doc := historyDoc{}
	for _, item := range items {
		raw := historyItem{
			Caption:               item.Caption,
			Reference:             item.Reference,
			Quote:                 item.Quote,
			Class:                 vpltClass(item),
			ModuleShortName:       item.ModuleShortName,
			ModuleShortNameSecond: item.ModuleShortNameSecond,
			SongID:                item.SongID,
			SongbookID:            item.SongbookID,
			Title:                 item.Title,
		}
		if item.Book > 0 {
			raw.Book = strconv.Itoa(item.Book)
		}
		if item.Chapter > 0 {
			raw.Chapter = strconv.Itoa(item.Chapter)
		}
		if item.Verse > 0 {
			raw.Verse = strconv.Itoa(item.Verse)
		}
		doc.Journals = append(doc.Journals, historyJournal{
			Name:  journalName(item.Kind),
			Items: []historyItem{raw},
		})
	}
	return marshalXML(doc)
}

func marshalXML(doc any) (string, error) {
	raw, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return xml.Header + string(raw) + "\n", nil
}

func parsePlanDef(text string) (string, []planItem, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var title string
	var section string
	var keys map[string]string
	var items []planItem
	flush := func() error {
		if section == "" || section == "plan" {
			section = ""
			keys = nil
			return nil
		}
		base, _, _ := strings.Cut(section, ".")
		kind, err := kindFromJournal(base)
		if err != nil {
			return err
		}
		item, err := itemFromINI(kind, keys)
		if err != nil {
			return err
		}
		items = append(items, item)
		section = ""
		keys = nil
		return nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if err := flush(); err != nil {
				return "", nil, err
			}
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			keys = map[string]string{}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || section == "" {
			return "", nil, errRequest
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = iniUnescape(val)
		if section == "plan" && key == "title" {
			title = val
			continue
		}
		if keys == nil {
			return "", nil, errRequest
		}
		keys[key] = val
	}
	if err := flush(); err != nil {
		return "", nil, err
	}
	if items == nil {
		items = []planItem{}
	}
	return title, items, nil
}

func itemFromINI(kind string, keys map[string]string) (planItem, error) {
	item := planItem{
		Kind:                  kind,
		Caption:               keys["caption"],
		Reference:             keys["reference"],
		Quote:                 keys["quote"],
		Class:                 keys["class"],
		Title:                 keys["title"],
		SongID:                keys["song_id"],
		SongbookID:            keys["songbook_id"],
		ModuleShortName:       keys["module_short_name"],
		ModuleShortNameSecond: keys["module_short_name_second"],
	}
	var err error
	if item.Book, err = mustCount(keys["book"]); err != nil {
		return planItem{}, err
	}
	if item.Chapter, err = mustCount(keys["chapter"]); err != nil {
		return planItem{}, err
	}
	if item.Verse, err = mustCount(keys["verse"]); err != nil {
		return planItem{}, err
	}
	if kind == "text" && item.Class == "" {
		item.Class = "-1"
	}
	if kind == "bible" && item.Class == "" {
		item.Class = "-1"
	}
	return item, nil
}

func formatPlanDef(title string, items []planItem) string {
	var buf strings.Builder
	buf.WriteString("[Plan]\n")
	buf.WriteString("Title=" + iniEscape(title) + "\n")
	for i, item := range items {
		buf.WriteByte('\n')
		buf.WriteString("[" + journalName(item.Kind) + "." + strconv.Itoa(i+1) + "]\n")
		writeINI(&buf, "caption", item.Caption)
		writeINI(&buf, "reference", item.Reference)
		writeINI(&buf, "quote", item.Quote)
		writeINI(&buf, "class", vpltClass(item))
		switch item.Kind {
		case "song":
			writeINI(&buf, "song_id", item.SongID)
			writeINI(&buf, "songbook_id", item.SongbookID)
			writeINI(&buf, "title", item.Title)
		case "bible":
			writeINI(&buf, "module_short_name", item.ModuleShortName)
			writeINI(&buf, "module_short_name_second", item.ModuleShortNameSecond)
			writeINI(&buf, "book", strconv.Itoa(item.Book))
			writeINI(&buf, "chapter", strconv.Itoa(item.Chapter))
			writeINI(&buf, "verse", strconv.Itoa(item.Verse))
		}
	}
	return buf.String()
}

func writeINI(buf *strings.Builder, key, value string) {
	buf.WriteString(key)
	buf.WriteByte('=')
	buf.WriteString(iniEscape(value))
	buf.WriteByte('\n')
}

func iniEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\r\n", `\n`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\n`)
	return s
}

func iniUnescape(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				buf.WriteByte('\n')
				i++
				continue
			case '\\':
				buf.WriteByte('\\')
				i++
				continue
			}
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}
