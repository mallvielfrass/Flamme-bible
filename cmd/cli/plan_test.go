package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanKeepsUUIDWithoutTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.vplt")
	history := filepath.Join(dir, "History.ini")
	if err := os.WriteFile(history, []byte("history\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePlan(t, path, planItem{Caption: "а", Reference: "Ин 3:16", Quote: "первый", Class: "-1"}, planItem{Caption: "б", Reference: "", Quote: "второй", Class: "-1"})
	first, err := syncPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if !validUUID(first.ID) || len(first.Items) != 2 {
		t.Fatalf("план %+v", first)
	}
	xmlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(xmlBytes), first.ID) || strings.Contains(string(xmlBytes), first.Items[0].ID) {
		t.Fatal("идентификатор записан в XML")
	}
	writePlan(t, path, planItem{Caption: "а", Reference: "Ин 3:16", Quote: "первый", Class: "-1"}, planItem{Caption: "б", Reference: "", Quote: "второй", Class: "-1"})
	second, err := syncPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Items[0].ID != first.Items[0].ID || second.Items[1].ID != first.Items[1].ID {
		t.Fatalf("хеш не сохранил пункты %+v", second)
	}
	writePlan(t, path, planItem{Caption: "б", Reference: "", Quote: "второй", Class: "-1"}, planItem{Caption: "а", Reference: "Ин 3:16", Quote: "первый", Class: "-1"})
	swapped, err := syncPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if swapped.Items[0].ID != first.Items[1].ID || swapped.Items[1].ID != first.Items[0].ID {
		t.Fatalf("порядок не узнан по хешу %+v", swapped.Items)
	}
	other := filepath.Join(dir, "copy.vplt")
	writePlan(t, other, planItem{Caption: "а", Reference: "Ин 3:16", Quote: "первый", Class: "-1"}, planItem{Caption: "б", Reference: "", Quote: "второй", Class: "-1"})
	copied, err := syncPlan(other)
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID == first.ID {
		t.Fatal("файл без sync взял чужой план")
	}
	kept := historyBytes(t, history)
	if kept != "history\r\n" {
		t.Fatal("History.ini изменён")
	}
}

func TestPlanAmbiguousReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.vplt")
	writePlan(t, path, planItem{Caption: "а", Reference: "", Quote: "один", Class: "-1"})
	first, err := syncPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	writePlan(t, path,
		planItem{Caption: "б", Reference: "", Quote: "два", Class: "-1"},
		planItem{Caption: "в", Reference: "", Quote: "три", Class: "-1"},
	)
	next, err := syncPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 3 || !next.Items[2].Deleted || next.Items[2].ID != first.Items[0].ID {
		t.Fatalf("старый пункт %+v", next.Items)
	}
	if next.Items[0].ID == first.Items[0].ID || next.Items[1].ID == first.Items[0].ID || next.Items[0].ID == next.Items[1].ID {
		t.Fatalf("новым пунктам не выданы новые UUID %+v", next.Items)
	}
	xmlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(xmlBytes), next.Items[0].ID) {
		t.Fatal("новый UUID записан в XML")
	}
}

func writePlan(t *testing.T, path string, items ...planItem) {
	t.Helper()
	var buf strings.Builder
	buf.WriteString("<VisioBibleJournal>\n  <Journal Name=\"Text\">\n")
	for _, item := range items {
		fmtItem(&buf, item)
	}
	buf.WriteString("  </Journal>\n</VisioBibleJournal>\n")
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fmtItem(buf *strings.Builder, item planItem) {
	buf.WriteString(`    <Item Caption="`)
	buf.WriteString(item.Caption)
	buf.WriteString(`" Reference="`)
	buf.WriteString(item.Reference)
	buf.WriteString(`" Quote="`)
	buf.WriteString(item.Quote)
	buf.WriteString(`" Class="`)
	buf.WriteString(item.Class)
	buf.WriteString("\"/>\n")
}

func historyBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
