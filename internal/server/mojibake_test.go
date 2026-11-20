package server

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestRepairMojibake(t *testing.T) {
	plain := "Привет, мир"
	as1252 := decodeCharmap(charmap.Windows1252, []byte(plain))
	as1251 := decodeCharmap(charmap.Windows1251, []byte(plain))
	cp, ok := encodeCharmap(charmap.Windows1251, plain)
	if !ok {
		t.Fatal("привет не кодируется в windows-1251")
	}
	asLatin := decodeCharmap(charmap.ISO8859_1, cp)

	fixed, repaired := repairMojibake(as1252)
	if !repaired || fixed != plain {
		t.Fatalf("latin-1/1252: repaired=%v %q из %q", repaired, fixed, as1252)
	}
	if !strings.Contains(as1252, "Ð") {
		t.Fatalf("ожидались пары Ð: %q", as1252)
	}
	fixed, repaired = repairMojibake(as1251)
	if !repaired || fixed != plain {
		t.Fatalf("cp1251: repaired=%v %q из %q", repaired, fixed, as1251)
	}
	fixed, repaired = repairMojibake(asLatin)
	if !repaired || fixed != plain {
		t.Fatalf("1251 как latin-1: repaired=%v %q из %q", repaired, fixed, asLatin)
	}
	if _, again := repairMojibake(fixed); again {
		t.Fatal("повторная починка")
	}

	xml := `<VisioBibleJournal><Journal Name="Text"><Item Caption="Привет" Quote="мир" Class="-1"/></Journal></VisioBibleJournal>`
	broken := decodeCharmap(charmap.Windows1252, []byte(xml))
	fixed, repaired = repairMojibake(broken)
	if !repaired || fixed != xml {
		t.Fatalf("xml: repaired=%v %q", repaired, fixed)
	}
	cpXML, ok := encodeCharmap(charmap.Windows1251, xml)
	if !ok {
		t.Fatal("xml")
	}
	fixed, repaired = repairMojibake(decodeCharmap(charmap.ISO8859_1, cpXML))
	if !repaired || fixed != xml {
		t.Fatalf("xml 1251: repaired=%v %q", repaired, fixed)
	}
}

func TestRepairMojibakeLeavesRealText(t *testing.T) {
	samples := []string{
		"",
		"Привет, мир",
		"Ёлка у реки. Ин. 3:16",
		"Ибо так возлюбил Бог мир, что отдал Сына Своего Единородного.",
		"Grüße aus Köln",
		"Straße, schön und groß",
		"für München",
		"Café naïve à l'été",
		"Ça va très bien, où est l'hôtel?",
		"L'élève est à Paris",
		"Привет, naïve café und Grüße",
		"Hello, Ин. 3:16 и Straße",
		"äöüß",
		"àèéêëîôû",
	}
	for _, sample := range samples {
		if fixed, ok := repairMojibake(sample); ok {
			t.Fatalf("ложное срабатывание %q → %q", sample, fixed)
		}
	}
	mixed := `<Item Caption="` + decodeCharmap(charmap.Windows1252, []byte("Привет")) + `" Quote="ёлка"/>`
	if fixed, ok := repairMojibake(mixed); ok {
		t.Fatalf("смешанный xml %q → %q", mixed, fixed)
	}
}

func TestEncodeWindows1251RejectsUnmappable(t *testing.T) {
	raw, err := encodeWindows1251("Привет")
	if err != nil {
		t.Fatal(err)
	}
	if decodeCharmap(charmap.Windows1251, raw) != "Привет" {
		t.Fatalf("roundtrip %q", raw)
	}
	if _, err := encodeWindows1251("café"); err == nil {
		t.Fatal("é записан в windows-1251")
	}
}
