import { describe, expect, it } from "vitest";
import {
  copyTitle,
  createDimension,
  folderHint,
  folderMeta,
  kindLabel,
  makeBible,
  planCounts,
  planInFolder,
  visiblePlans,
  type Plan,
} from "./model";

function plan(id: string, title: string, dimension: string | null): Plan {
  return { id, title, dimension_id: dimension, version: 1, updated_at: "", items: [] };
}

describe("plans list", () => {
  const plans = [
    plan("a", "Личный", null),
    plan("b", "Воскресенье", "community-1"),
    plan("c", "Репетиция", "community-2"),
  ];

  it("filters personal and one community", () => {
    expect(visiblePlans(plans, "all").map((item) => item.id)).toEqual(["a", "b", "c"]);
    expect(visiblePlans(plans, "personal").map((item) => item.id)).toEqual(["a"]);
    expect(visiblePlans(plans, "community").map((item) => item.id)).toEqual(["b", "c"]);
    expect(visiblePlans(plans, "community-1").map((item) => item.id)).toEqual(["b"]);
  });

  it("counts spaces", () => {
    expect(planCounts(plans)).toMatchObject({ all: 3, personal: 1, community: 2, "community-1": 1 });
  });

  it("creates a community plan only when that community is selected", () => {
    expect(createDimension("community-1", ["community-1"])).toBe("community-1");
    expect(createDimension("all", ["community-1"])).toBe("");
    expect(createDimension("community", ["community-1"])).toBe("");
  });
});

describe("folder presence", () => {
  const plan = { id: "server-1", title: "Воскресенье" };

  it("matches a binding and a single unbound file with the same title", () => {
    expect(planInFolder(plan, [{ title: "Другой", server_id: "server-1" }])).toBe(true);
    expect(planInFolder(plan, [{ title: "Воскресенье", server_id: "" }])).toBe(true);
    expect(planInFolder(plan, [{ title: "Воскресенье", server_id: "other" }])).toBe(false);
    expect(
      planInFolder(plan, [
        { title: "Воскресенье", server_id: "" },
        { title: "Воскресенье", server_id: "" },
      ]),
    ).toBe(false);
  });

  it("uses the mockup wording", () => {
    expect(folderMeta(true)).toBe("есть в VisioBible");
    expect(folderMeta(false)).toBe("только на сервере");
    expect(folderHint(false)).toContain("после синхронизации");
  });
});

describe("plan items", () => {
  it("labels kinds the way the plans mockup does", () => {
    expect(kindLabel("song")).toBe("Песня");
    expect(kindLabel("bible")).toBe("Стих");
    expect(kindLabel("text")).toBe("Текст");
    expect(copyTitle("Воскресенье")).toBe("Копия — Воскресенье");
  });

  it("rejects a bible item without a module name", () => {
    expect(() => makeBible({ kind: "bible", reference: "Ин. 3:16" })).toThrow(/перевод/);
  });
});
