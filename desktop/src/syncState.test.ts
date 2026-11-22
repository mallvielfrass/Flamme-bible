import { describe, expect, it } from "vitest";
import { actionLabel, canStart, conflictRows, countPhrase, decisionsFrom, foundLine, groupKey, groupRows, planKey, songKey } from "./syncState";

describe("sync screen state", () => {
  it("groups uploads, downloads and conflicts without showing raw codes", () => {
    const rows = [
      { action: "push_add" },
      { action: "push_delete" },
      { action: "pull_update" },
      { action: "unchanged" },
      { action: "conflict" },
    ];
    const groups = groupRows(rows);
    expect(groups.upload).toHaveLength(2);
    expect(groups.download).toHaveLength(1);
    expect(groups.unchanged).toHaveLength(1);
    expect(groups.conflicts).toHaveLength(1);
    expect(actionLabel("push_update")).toBe("Изменено здесь");
    expect(actionLabel("conflict")).not.toMatch(/cursor|payload|operation/);
  });

  it("blocks start until every conflict has a clear choice", () => {
    const rows = [{ action: "conflict", key: songKey("book.vbm", "id-1") }];
    expect(canStart(rows, {})).toBe(false);
    expect(canStart(rows, { [rows[0].key]: { kind: "keep_local" } })).toBe(true);
    expect(canStart(rows, { [rows[0].key]: { kind: "manual", title: "  ", lyrics: "текст" } })).toBe(false);
    expect(canStart(rows, { [rows[0].key]: { kind: "manual", title: "Своя", lyrics: "текст" } })).toBe(true);
  });

  it("counts what was found and sends only answered conflicts", () => {
    expect(foundLine(1, 2, 5)).toBe("Найдено: 1 сборник, 2 модуля Библии, 5 планов.");
    expect(countPhrase(11, "план", "плана", "планов")).toBe("11 планов");
    const key = songKey("/tmp/a.vbm", "song");
    const sent = decisionsFrom(
      { [key]: { kind: "keep_both" } },
      [{ book_path: "/tmp/a.vbm", fbid: "song", action: "conflict" }],
      [],
    );
    expect(sent.songs).toEqual([{ book_path: "/tmp/a.vbm", fbid: "song", choice: "keep_both", title: "", lyrics: "" }]);
  });

  it("starts once every conflict is chosen and sends those choices", () => {
    const songs = [
      { book_path: "a.vbm", fbid: "s1", action: "conflict" },
      { book_path: "a.vbm", fbid: "s2", action: "pull_update" },
    ];
    const plans = [{ key: "plan-1", action: "conflict" }];
    const rows = conflictRows(songs, plans);
    expect(rows.map((row) => row.key)).toEqual([songKey("a.vbm", "s1"), planKey("plan-1")]);
    expect(canStart(rows, {})).toBe(false);
    const chosen = {
      [songKey("a.vbm", "s1")]: { kind: "take_server" as const },
      [planKey("plan-1")]: { kind: "keep_local" as const },
    };
    expect(canStart(rows, chosen)).toBe(true);
    expect(decisionsFrom(chosen, songs, plans)).toEqual({
      songs: [{ book_path: "a.vbm", fbid: "s1", choice: "take_server", title: "", lyrics: "" }],
      plans: [{ key: "plan-1", choice: "keep_local" }],
      groups: [],
    });
    const groups = [{ book_path: "a.vbm", section_id: "sec", action: "conflict" }];
    const withGroup = conflictRows(songs, plans, groups);
    expect(canStart(withGroup, chosen)).toBe(false);
    const groupChosen = { ...chosen, [groupKey("a.vbm", "sec")]: { kind: "keep_local" as const } };
    expect(canStart(withGroup, groupChosen)).toBe(true);
    expect(decisionsFrom(groupChosen, songs, plans, groups).groups).toEqual([
      { book_path: "a.vbm", section_id: "sec", choice: "keep_local" },
    ]);
  });
});
