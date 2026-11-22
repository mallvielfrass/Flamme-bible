import { describe, expect, it } from "vitest";
import { actionLabel, adoptionKey, bannerText, canStart, closeDecision, conflictRows, countPhrase, decisionsFrom, foundLine, freshChipText, groupKey, groupRows, hasUnsyncedChanges, planKey, songKey, syncCounts, syncReady, syncScreen, type PreviewData, type PreviewPlan, type PreviewSong } from "./syncState";

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
  });

  it("asks for a group name only when both sides renamed it, then sends the choice", () => {
    const group = { book_path: "pv.vbm", book_title: "Песнь Возрождения", section_id: "sec", name: "Хор", action: "conflict", local_name: "Хор А", remote_name: "Хор Б" };
    const data = preview({ groups: [group, { ...group, section_id: "sec2", action: "push_update" }] });
    expect(syncCounts(data)).toMatchObject({ conflicts: 1, send: 1 });
    const open = syncScreen(data, {}, {});
    expect(open.ready).toBe(false);
    expect(open.books[0]).toMatchObject({ mode: "name", kind: "group", serverId: "sec", conflictId: "pv.vbm" });
    expect(open.banner).toContain("Группу «Хор А»");
    const chosen = { [groupKey("pv.vbm", "sec")]: { kind: "take_server" as const } };
    expect(syncScreen(data, chosen, {}).ready).toBe(true);
    expect(decisionsFrom(chosen, [], [], data.groups).groups).toEqual([{ book_path: "pv.vbm", section_id: "sec", choice: "take_server" }]);
  });

  it("flags preview rows that are not yet the same in the folder", () => {
    expect(hasUnsyncedChanges(null)).toBe(false);
    expect(hasUnsyncedChanges({ songs: [{ action: "unchanged" }], plans: [{ action: "unchanged" }] })).toBe(false);
    expect(hasUnsyncedChanges({ songs: [{ action: "push_update" }], plans: [] })).toBe(true);
    expect(hasUnsyncedChanges({ songs: [], plans: [{ action: "pull_add" }] })).toBe(true);
    expect(hasUnsyncedChanges({ songs: [{ action: "conflict" }], plans: [] })).toBe(true);
    expect(
      hasUnsyncedChanges(
        { songs: [{ action: "pull_add", absent: true, server_id: "book-1" }], plans: [] },
        { songbooks: ["book-1"], plans: [] },
      ),
    ).toBe(false);
    expect(
      hasUnsyncedChanges(
        { songs: [{ action: "pull_add", absent: true, server_id: "book-1" }], plans: [{ action: "push_update" }] },
        { songbooks: ["book-1"], plans: [] },
      ),
    ).toBe(true);
  });

  it("waits for a running sync and asks only when the last preview still has changes", () => {
    expect(closeDecision(false, null)).toBe("close");
    expect(closeDecision(false, { songs: [{ action: "unchanged" }], plans: [] })).toBe("close");
    expect(closeDecision(true, null)).toBe("wait");
    expect(closeDecision(true, { songs: [{ action: "push_update" }], plans: [] })).toBe("wait");
    expect(closeDecision(false, { songs: [{ action: "push_update" }], plans: [] })).toBe("ask");
    expect(closeDecision(false, { songs: [], plans: [{ action: "pull_add" }] })).toBe("ask");
    expect(
      closeDecision(false, { songs: [{ action: "pull_add", absent: true, server_id: "book-1" }], plans: [] }, { songbooks: ["book-1"], plans: [] }),
    ).toBe("close");
  });
});

function song(patch: Partial<PreviewSong> & Pick<PreviewSong, "book_path" | "action">): PreviewSong {
  return {
    book_title: "Сборник",
    fbid: "song",
    title: "Песня",
    server_id: "",
    absent: false,
    ...patch,
  };
}

function plan(patch: Partial<PreviewPlan> & Pick<PreviewPlan, "key" | "action">): PreviewPlan {
  return {
    name: "План",
    server_id: "",
    absent: false,
    summary: "",
    local_summary: "здесь",
    remote_summary: "там",
    ...patch,
  };
}

function preview(patch: Partial<PreviewData> = {}): PreviewData {
  return { songs: [], plans: [], groups: [], skipped: [], ...patch };
}

describe("sync list", () => {
  it("counts sends, downloads, new items and skips, and leaves remembered skips out of the banner", () => {
    const data = preview({
      songs: [
        song({ book_path: "pv.vbm", book_title: "Песнь Возрождения", action: "push_update", fbid: "a", title: "Одна" }),
        song({ book_path: "pv.vbm", book_title: "Песнь Возрождения", action: "push_add", fbid: "b", title: "Две" }),
        song({ book_path: "pv.vbm", book_title: "Песнь Возрождения", action: "unchanged", fbid: "c" }),
        song({ book_path: "young.vbm", book_title: "Молодёжный сборник", action: "pull_add", fbid: "d", server_id: "book-new", absent: true }),
        song({ book_path: "skip.vbm", book_title: "Старый", action: "pull_add", fbid: "e", server_id: "book-skip", absent: true }),
      ],
      plans: [
        plan({ key: "sunday", name: "Воскресенье 15.11.2026", action: "pull_update" }),
        plan({ key: "youth", name: "Молодёжка 22.11", action: "push_update" }),
        plan({ key: "same", action: "unchanged" }),
      ],
      skipped: [{ kind: "songbook", id: "book-skip", title: "Старый" }],
    });
    const visible = {
      ...data,
      songs: data.songs.filter((row) => row.server_id !== "book-skip"),
    };
    expect(syncCounts(visible)).toMatchObject({
      send: 3,
      receive: 1,
      freshBooks: 1,
      freshPlans: 0,
      conflicts: 0,
      skipped: 1,
      unchanged: 2,
    });
    expect(freshChipText(1, 0)).toBe("1 новый сборник");
    const text = bannerText(
      preview({
        songs: data.songs.filter((row) => row.server_id !== "book-skip"),
        plans: data.plans,
      }),
      {},
    );
    expect(text).toContain("Сборник «Молодёжный сборник» ещё не в папке.");
    expect(text).toContain("План «Воскресенье 15.11.2026» изменён на сервере — в VisioBible попадёт после синхронизации.");
    expect(text).not.toContain("Песнь Возрождения");
    expect(text).not.toContain("Молодёжка");
    expect(text).not.toContain("Старый");
    expect(bannerText(preview({ songs: [song({ book_path: "a.vbm", book_title: "Песнь Возрождения", action: "push_update" })] }), {})).toContain("Песнь Возрождения");
    const screen = syncScreen(data, {}, {}, { songbooks: ["book-skip"], plans: [] });
    expect(screen.books.map((row) => row.title)).toEqual(["Песнь Возрождения", "Молодёжный сборник"]);
    expect(screen.books[0].meta).toBe("2 песни отправятся на сервер · 3 песни в файле");
    expect(screen.books[0].status).toBe("Изменено здесь");
    expect(screen.books[1].mode).toBe("adopt");
    expect(screen.plans.map((row) => row.status)).toEqual(["Изменено на сервере", "Изменено в VisioBible"]);
    expect(screen.ready).toBe(false);
    expect(screen.hint).toBe("Сначала выберите: добавить новый сборник в папку или пропустить его.");
  });

  it("starts only after a new book is accepted and every conflict is chosen", () => {
    const data = preview({
      songs: [
        song({ book_path: "pv.vbm", book_title: "Песнь Возрождения", action: "conflict", fbid: "song-1", title: "Ты моя сокровенная жизнь" }),
        song({ book_path: "young.vbm", book_title: "Молодёжный сборник", action: "pull_add", server_id: "book-new", absent: true }),
      ],
    });
    expect(syncReady(data, {}, {})).toBe(false);
    const adopted = { [adoptionKey("songbook", "book-new")]: true as const };
    expect(syncReady(data, {}, adopted)).toBe(false);
    const chosen = { [songKey("pv.vbm", "song-1")]: { kind: "take_server" as const } };
    expect(syncReady(data, chosen, adopted)).toBe(true);
    expect(canStart(conflictRows(data.songs, []), chosen, [])).toBe(true);
    const screen = syncScreen(data, chosen, adopted);
    expect(screen.ready).toBe(true);
    expect(screen.hint).toBe("");
    expect(screen.books.find((row) => row.title === "Песнь Возрождения")?.status).toBe("Изменено на сервере");
    expect(screen.chips.some((chip) => chip.id === "conflicts")).toBe(true);
    const open = syncScreen(data, {}, adopted);
    expect(open.banner).toContain("Песню «Ты моя сокровенная жизнь» изменили и в VisioBible, и на сервере — нужно выбрать версию.");
    expect(open.books.find((row) => row.mode === "compare")?.conflictId).toBe("song-1");
  });
});
