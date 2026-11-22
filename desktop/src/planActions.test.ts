import { describe, expect, it } from "vitest";
import type { PlanItem } from "./commands";
import {
  clonePlanItems,
  deletePrompt,
  deleteScope,
  duplicateTitle,
  exportExtension,
  exportFileName,
  fileLabel,
  importFormatFromName,
  pointsLabel,
} from "./planActions";

function item(extra: Partial<PlanItem> = {}): PlanItem {
  return {
    kind: "song",
    title: "Песня",
    song_id: "11111111-1111-4111-8111-111111111111",
    songbook_id: "",
    caption: "",
    quote: "",
    reference: "",
    class: "-1",
    module_short_name: "",
    module_short_name_second: "",
    book: 0,
    chapter: 0,
    verse: 0,
    id: "22222222-2222-4222-8222-222222222222",
    position: 3,
    ...extra,
  };
}

describe("plan file actions", () => {
  it("picks the format the site uses for the same file names", () => {
    expect(importFormatFromName("C:\\Plans\\Sunday.vplt")).toBe("vplt");
    expect(importFormatFromName("/Plans/History.ini")).toBe("history");
    expect(importFormatFromName("Week.plandef.ini")).toBe("plandef");
    expect(importFormatFromName("plan.json")).toBe("json");
    expect(importFormatFromName("notes.txt")).toBe("");
    expect(exportFileName("history")).toBe("History.ini");
    expect(exportFileName("plandef")).toBe("PlanDef.ini");
    expect(exportFileName("vplt")).toBe("plan.vplt");
    expect(exportFileName("json")).toBe("plan.json");
    expect(exportExtension("json")).toBe("json");
    expect(exportExtension("history")).toBe("ini");
    expect(fileLabel("/Plans/Sunday.vplt")).toBe("Sunday.vplt");
  });

  it("copies a plan under a new local name and drops server item ids", () => {
    expect(duplicateTitle("Служение", [])).toBe("Копия — Служение");
    expect(duplicateTitle("  ", ["Копия — План"])).toBe("Копия — План (2)");
    expect(duplicateTitle("Служение", ["Копия — Служение", "Копия — Служение (2)"])).toBe("Копия — Служение (3)");
    const source = item();
    const copy = clonePlanItems([source]);
    expect(copy[0].title).toBe("Песня");
    expect(copy[0].id).toBeUndefined();
    expect(copy[0].position).toBeUndefined();
    expect(source.id).toBe("22222222-2222-4222-8222-222222222222");
    expect(source.position).toBe(3);
  });

  it("confirms a local delete, a server delete, or both", () => {
    expect(pointsLabel(1)).toBe("1 пункт");
    expect(pointsLabel(2)).toBe("2 пункта");
    expect(pointsLabel(11)).toBe("11 пунктов");
    expect(deleteScope("/Plans/a.vplt", "server-1", true)).toBe("both");
    expect(deleteScope("", "server-1", true)).toBe("server");
    expect(deleteScope("/Plans/a.vplt", "server-1", false)).toBe("local");
    const local = deletePrompt({ name: "Служение", count: 1, scope: "local" });
    expect(local.title).toBe("Удалить план?");
    expect(local.text).toContain("с этого компьютера");
    expect(local.text).toContain("1 пункт");
    const kept = deletePrompt({ name: "Служение", count: 1, scope: "local", keptOnServer: true });
    expect(kept.text).toContain("Копия на сервере останется");
    const server = deletePrompt({ name: "Служение", count: 2, scope: "server", place: "Община" });
    expect(server.text).toContain("из общины «Община»");
    expect(server.text).toContain("после следующей синхронизации");
    const both = deletePrompt({ name: "", count: 5, scope: "both" });
    expect(both.text).toContain("«План»");
    expect(both.text).toContain("на сервере и с этого компьютера");
  });
});
