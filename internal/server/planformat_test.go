package server

import (
	"strings"
	"testing"

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
	vplt, _, err := formatPlan("vplt", planView{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(vplt, "ModuleShortName") {
		t.Fatalf("vplt содержит служебный атрибут: %s", vplt)
	}
}
