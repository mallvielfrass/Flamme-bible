package server

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

func TestPlanPayloadRoundTrip(t *testing.T) {
	items := []planItem{{
		Kind: "text", Caption: "Первая\nстрока", Reference: "хор", Quote: `цитата "с" кавычками`, Class: "-1",
	}, {
		Kind: "bible", ModuleShortName: "RST", ModuleShortNameSecond: "NRT",
		Book: 43, Chapter: 3, Verse: 16, Reference: "Ин 3:16", Quote: "мир", Class: "-1",
	}}
	raw := formatPlanDef("Служение", items)
	title, parsed, err := parsePlanDef(raw)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Служение" || len(parsed) != 2 || parsed[0].Quote != items[0].Quote || parsed[0].Caption != items[0].Caption {
		t.Fatalf("ini %+v %q", parsed, title)
	}
	if parsed[1].ModuleShortNameSecond != "NRT" || parsed[1].Book != 43 || parsed[1].Verse != 16 {
		t.Fatalf("библия %+v", parsed[1])
	}

	xmlText := `<VisioBibleJournal><Journal Name="Text"><Item Caption="ПЕСНЯ" Reference="" Quote="текст" Class="-1"/></Journal></VisioBibleJournal>`
	encoded, err := charmap.Windows1251.NewEncoder().Bytes([]byte(xmlText))
	if err != nil {
		t.Fatal(err)
	}
	_, parsed, err = parsePlanPayload("vplt", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || parsed[0].Caption != "ПЕСНЯ" || parsed[0].Kind != "text" {
		t.Fatalf("1251 %+v", parsed)
	}
	_, _, err = parsePlanDef("[Bible.1]\nmodule_short_name=RST\nbook=нет\nreference=Ин 1:1\n")
	if err == nil {
		t.Fatal("плохой номер книги принят")
	}
	vpltBytes, _, err := formatPlan("vplt", "", planView{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	vplt := string(vpltBytes)
	if strings.Contains(vplt, "ModuleShortName") {
		t.Fatalf("vplt содержит служебный атрибут: %s", vplt)
	}
	encoded, ctype, err := formatPlan("vplt", "windows-1251", planView{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if ctype != "application/xml; charset=windows-1251" || !strings.Contains(string(encoded), `encoding="windows-1251"`) {
		t.Fatalf("заголовок %s %q", ctype, encoded)
	}
	if utf8.Valid(encoded) {
		t.Fatal("windows-1251 записан как utf-8")
	}
	decoded := decodeCharmap(charmap.Windows1251, encoded)
	if !strings.Contains(decoded, `encoding="windows-1251"`) || strings.Contains(decoded, `encoding="UTF-8"`) || !strings.Contains(decoded, "Первая") {
		t.Fatalf("байты %s", decoded)
	}
	if _, _, err := formatPlan("plandef", "windows-1251", planView{Title: "Служение", Items: []planItem{{Kind: "text", Caption: "café", Quote: "текст"}}}); err == nil {
		t.Fatal("é записан молча")
	}
}
