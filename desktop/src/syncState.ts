export type Choice =
  | { kind: "keep_local" }
  | { kind: "take_server" }
  | { kind: "keep_both" }
  | { kind: "manual"; title: string; lyrics: string };

export interface Row {
  action: string;
  key: string;
  book_title?: string;
  title?: string;
  name?: string;
}

export function songKey(bookPath: string, fbid: string): string {
  return JSON.stringify(["song", bookPath, fbid]);
}

export function planKey(key: string): string {
  return JSON.stringify(["plan", key]);
}

export function groupKey(bookPath: string, sectionId: string): string {
  return JSON.stringify(["group", bookPath, sectionId]);
}

export function countPhrase(count: number, one: string, few: string, many: string): string {
  const mod10 = count % 10;
  const mod100 = count % 100;
  const word = mod10 === 1 && mod100 !== 11 ? one : mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) ? few : many;
  return `${count} ${word}`;
}

export function foundLine(songbooks: number, bibles: number, plans: number): string {
  return `Найдено: ${countPhrase(songbooks, "сборник", "сборника", "сборников")}, ${countPhrase(bibles, "модуль Библии", "модуля Библии", "модулей Библии")}, ${countPhrase(plans, "план", "плана", "планов")}.`;
}

export function groupRows<T extends { action: string }>(rows: T[]) {
  return {
    upload: rows.filter((row) => row.action.startsWith("push")),
    download: rows.filter((row) => row.action.startsWith("pull")),
    unchanged: rows.filter((row) => row.action === "unchanged"),
    conflicts: rows.filter((row) => row.action === "conflict"),
  };
}

export function actionLabel(action: string): string {
  switch (action) {
    case "push_add":
      return "Новое, отправится на сервер";
    case "push_update":
      return "Изменено здесь";
    case "push_delete":
      return "Удалено здесь";
    case "pull_add":
      return "Новое на сервере";
    case "pull_update":
      return "Изменено на сервере";
    case "pull_delete":
      return "Удалено на сервере";
    case "conflict":
      return "Нужно выбрать";
    default:
      return "Без изменений";
  }
}

export function choiceReady(choice: Choice | undefined): boolean {
  if (!choice) return false;
  if (choice.kind === "manual") return choice.title.trim().length > 0;
  return choice.kind === "keep_local" || choice.kind === "take_server" || choice.kind === "keep_both";
}

export function unresolved(rows: { action: string; key: string }[], chosen: Record<string, Choice>): string[] {
  return rows.filter((row) => row.action === "conflict" && !choiceReady(chosen[row.key])).map((row) => row.key);
}

export function canStart(
  rows: { action: string; key: string }[],
  chosen: Record<string, Choice>,
  pending: string[] = [],
): boolean {
  return unresolved(rows, chosen).length === 0 && pending.length === 0;
}

export interface SkipLists {
  songbooks: string[];
  plans: string[];
}

export function isRememberedSkip(row: { absent?: boolean; server_id?: string }, ids: string[]): boolean {
  return Boolean(row.absent && row.server_id && ids.includes(row.server_id));
}

/** Есть ли в ответе preview_sync то, что ещё не совпадает с папкой.
 *  Считаются песни и планы с действием кроме `unchanged`.
 *  Запомненный пропуск этого компьютера не считается: строки с absent и id из списка отбрасываются.
 *  null значит, что список не запрашивали: закрытие из-за этого не блокируется. */
export function hasUnsyncedChanges(
  preview: {
    songs: { action: string; absent?: boolean; server_id?: string }[];
    plans: { action: string; absent?: boolean; server_id?: string }[];
  } | null,
  skips: SkipLists = { songbooks: [], plans: [] },
): boolean {
  if (!preview) return false;
  const songs = preview.songs.filter((row) => !isRememberedSkip(row, skips.songbooks));
  const plans = preview.plans.filter((row) => !isRememberedSkip(row, skips.plans));
  return [...songs, ...plans].some((row) => row.action !== "unchanged");
}

export function conflictRows(
  songs: { action: string; book_path: string; fbid: string }[],
  plans: { action: string; key: string }[],
  groups: { action: string; book_path: string; section_id: string }[] = [],
): { action: string; key: string }[] {
  return [
    ...songs.filter((row) => row.action === "conflict").map((row) => ({ action: row.action, key: songKey(row.book_path, row.fbid) })),
    ...plans.filter((row) => row.action === "conflict").map((row) => ({ action: row.action, key: planKey(row.key) })),
    ...groups.filter((row) => row.action === "conflict").map((row) => ({ action: row.action, key: groupKey(row.book_path, row.section_id) })),
  ];
}

export interface SongDecision {
  book_path: string;
  fbid: string;
  choice: string;
  title: string;
  lyrics: string;
}

export interface PlanDecision {
  key: string;
  choice: string;
}

export interface GroupDecision {
  book_path: string;
  section_id: string;
  choice: string;
}

export function decisionsFrom(
  chosen: Record<string, Choice>,
  songs: { book_path: string; fbid: string; action: string }[],
  plans: { key: string; action: string }[],
  groups: { book_path: string; section_id: string; action: string }[] = [],
): { songs: SongDecision[]; plans: PlanDecision[]; groups: GroupDecision[] } {
  const songDecisions: SongDecision[] = [];
  for (const song of songs) {
    if (song.action !== "conflict") continue;
    const choice = chosen[songKey(song.book_path, song.fbid)];
    if (!choiceReady(choice)) continue;
    songDecisions.push({
      book_path: song.book_path,
      fbid: song.fbid,
      choice: choice.kind,
      title: choice.kind === "manual" ? choice.title : "",
      lyrics: choice.kind === "manual" ? choice.lyrics : "",
    });
  }
  const planDecisions: PlanDecision[] = [];
  for (const plan of plans) {
    if (plan.action !== "conflict") continue;
    const choice = chosen[planKey(plan.key)];
    if (!choice || choice.kind === "manual" || !choiceReady(choice)) continue;
    planDecisions.push({ key: plan.key, choice: choice.kind });
  }
  const groupDecisions: GroupDecision[] = [];
  for (const group of groups) {
    if (group.action !== "conflict") continue;
    const choice = chosen[groupKey(group.book_path, group.section_id)];
    if (!choice || choice.kind === "manual" || choice.kind === "keep_both" || !choiceReady(choice)) continue;
    groupDecisions.push({ book_path: group.book_path, section_id: group.section_id, choice: choice.kind });
  }
  return { songs: songDecisions, plans: planDecisions, groups: groupDecisions };
}

export interface PreviewSong {
  book_path: string;
  book_title: string;
  fbid: string;
  title: string;
  action: string;
  server_id: string;
  absent: boolean;
}

export interface PreviewPlan {
  key: string;
  name: string;
  action: string;
  server_id: string;
  absent: boolean;
  summary: string;
  local_summary: string;
  remote_summary: string;
}

export interface PreviewGroup {
  book_path: string;
  book_title: string;
  section_id: string;
  name: string;
  action: string;
  local_name: string;
  remote_name: string;
}

export interface PreviewSkipped {
  kind: string;
  id: string;
  title: string;
}

export interface PreviewData {
  songs: PreviewSong[];
  plans: PreviewPlan[];
  groups: PreviewGroup[];
  skipped: PreviewSkipped[];
}

export type RowTone = "push" | "pull" | "conflict" | "skip" | "ok";
export type RowMode = "compare" | "adopt" | "choose" | "name" | "plain";

export interface ListedRow {
  key: string;
  title: string;
  meta: string;
  status: string;
  tone: RowTone;
  rowClass: "" | "add" | "conflict" | "skip";
  aside: string;
  mode: RowMode;
  serverId: string;
  kind: "songbook" | "plan" | "group";
  conflictId: string;
}

export interface SyncChip {
  id: string;
  text: string;
  tone: "plain" | "ok" | "bad" | "zero";
}

export interface SyncCounts {
  send: number;
  receive: number;
  freshBooks: number;
  freshPlans: number;
  conflicts: number;
  skipped: number;
  unchanged: number;
}

export function adoptionKey(kind: "songbook" | "plan", id: string): string {
  return JSON.stringify([kind, id]);
}

export function withoutRememberedSkips(preview: PreviewData, skips: SkipLists): PreviewData {
  return {
    ...preview,
    songs: preview.songs.filter((song) => !isRememberedSkip(song, skips.songbooks)),
    plans: preview.plans.filter((plan) => !isRememberedSkip(plan, skips.plans)),
  };
}

interface BookBucket {
  path: string;
  title: string;
  serverId: string;
  absent: boolean;
  songs: PreviewSong[];
  push: number;
  pull: number;
  conflicts: number;
}

function bookBuckets(songs: PreviewSong[]): BookBucket[] {
  const order: string[] = [];
  const map = new Map<string, PreviewSong[]>();
  for (const song of songs) {
    if (!map.has(song.book_path)) {
      map.set(song.book_path, []);
      order.push(song.book_path);
    }
    map.get(song.book_path)?.push(song);
  }
  return order.map((path) => {
    const group = map.get(path) ?? [];
    return {
      path,
      title: group[0]?.book_title || "Сборник",
      serverId: group.find((song) => song.server_id)?.server_id ?? "",
      absent: group.some((song) => song.absent),
      songs: group,
      push: group.filter((song) => song.action.startsWith("push")).length,
      pull: group.filter((song) => song.action.startsWith("pull")).length,
      conflicts: group.filter((song) => song.action === "conflict").length,
    };
  });
}

function decided(song: PreviewSong, chosen: Record<string, Choice>, side: "push" | "pull"): boolean {
  if (song.action !== "conflict") return false;
  const kind = chosen[songKey(song.book_path, song.fbid)]?.kind;
  if (!choiceReady(chosen[songKey(song.book_path, song.fbid)])) return false;
  if (side === "pull") return kind === "take_server";
  return kind === "keep_local" || kind === "keep_both" || kind === "manual";
}

function openConflicts(bucket: BookBucket, chosen: Record<string, Choice>): PreviewSong[] {
  return bucket.songs.filter((song) => song.action === "conflict" && !choiceReady(chosen[songKey(song.book_path, song.fbid)]));
}

export function syncCounts(preview: PreviewData): SyncCounts {
  const books = bookBuckets(preview.songs);
  const freshPaths = new Set(books.filter((book) => book.absent).map((book) => book.path));
  let send = 0;
  let receive = 0;
  let conflicts = 0;
  let unchanged = 0;
  for (const song of preview.songs) {
    if (song.action === "conflict") conflicts += 1;
    else if (song.action === "unchanged") unchanged += 1;
    else if (song.action.startsWith("push")) send += 1;
    else if (song.action.startsWith("pull") && !freshPaths.has(song.book_path)) receive += 1;
  }
  for (const group of preview.groups) {
    if (group.action === "conflict") conflicts += 1;
    else if (group.action.startsWith("push")) send += 1;
    else if (group.action.startsWith("pull")) receive += 1;
  }
  let freshPlans = 0;
  for (const plan of preview.plans) {
    if (plan.absent) freshPlans += 1;
    else if (plan.action === "conflict") conflicts += 1;
    else if (plan.action === "unchanged") unchanged += 1;
    else if (plan.action.startsWith("push")) send += 1;
    else if (plan.action.startsWith("pull")) receive += 1;
  }
  return {
    send,
    receive,
    freshBooks: books.filter((book) => book.absent).length,
    freshPlans,
    conflicts,
    skipped: preview.skipped.length,
    unchanged,
  };
}

export function freshChipText(books: number, plans: number): string {
  if (books > 0 && plans === 0) return countPhrase(books, "новый сборник", "новых сборника", "новых сборников");
  if (plans > 0 && books === 0) return countPhrase(plans, "новый план", "новых плана", "новых планов");
  if (books + plans === 0) return "0 новых";
  return `${books + plans} новых`;
}

export function bibleLine(bibles: { short_name: string; name: string }[]): string {
  if (!bibles.length) return "Модулей Библии в папке не видно. Синхронизации модулей нет.";
  const names = bibles.map((bible) => bible.short_name || bible.name).filter((name) => name.length > 0);
  return `Модули Библии (${names.length ? names.join(", ") : "без имени"}) пока только в обзоре, синхронизации модулей нет.`;
}

export function placeLine(folder: string, space: string): string {
  if (folder && space) return `Папка ${folder} · пространство «${space}»`;
  if (folder) return `Папка ${folder}`;
  return "Папка VisioBible ещё не выбрана. Откройте настройки.";
}

export function pendingAdoption(preview: PreviewData, adopted: Record<string, true>): string[] {
  const keys: string[] = [];
  for (const bucket of bookBuckets(preview.songs)) {
    if (!bucket.absent || !bucket.serverId) continue;
    const key = adoptionKey("songbook", bucket.serverId);
    if (!adopted[key]) keys.push(key);
  }
  for (const plan of preview.plans) {
    if (!plan.absent || !plan.server_id) continue;
    const key = adoptionKey("plan", plan.server_id);
    if (!adopted[key]) keys.push(key);
  }
  return keys;
}

export function bannerText(preview: PreviewData, chosen: Record<string, Choice>): string {
  const lines: string[] = [];
  const local: string[] = [];
  for (const bucket of bookBuckets(preview.songs)) {
    const unresolved = openConflicts(bucket, chosen);
    if (bucket.absent) {
      lines.push(`Сборник «${bucket.title}» ещё не в папке.`);
      continue;
    }
    for (const song of unresolved) {
      lines.push(`Песню «${song.title}» изменили и в VisioBible, и на сервере — нужно выбрать версию.`);
    }
    if (unresolved.length) continue;
    if (bucket.pull) lines.push(`В сборнике «${bucket.title}» есть изменения на сервере.`);
    else if (bucket.push) local.push(`Сборник «${bucket.title}»: ${countPhrase(bucket.push, "песня отправится", "песни отправятся", "песен отправятся")} на сервер.`);
  }
  for (const plan of preview.plans) {
    if (plan.action === "unchanged") continue;
    if (plan.absent) {
      lines.push(`План «${plan.name}» есть на сервере, на этом компьютере его ещё нет.`);
      continue;
    }
    if (plan.action === "conflict" && !choiceReady(chosen[planKey(plan.key)])) {
      lines.push(`План «${plan.name}» изменён и в VisioBible, и на сервере — нужно выбрать версию.`);
      continue;
    }
    if (plan.action === "conflict") continue;
    if (plan.action.startsWith("pull")) lines.push(`План «${plan.name}» изменён на сервере — в VisioBible попадёт после синхронизации.`);
    else if (plan.action.startsWith("push")) local.push(`План «${plan.name}» изменён в VisioBible — на сервер уйдёт при синхронизации.`);
  }
  for (const group of openGroupConflicts(preview, chosen)) {
    lines.push(`Группу «${group.local_name}» в сборнике «${group.book_title}» переименовали и в VisioBible, и на сервере — выберите название.`);
  }
  return (lines.length ? lines : local).join(" ");
}

function openGroupConflicts(preview: PreviewData, chosen: Record<string, Choice>): PreviewGroup[] {
  return preview.groups.filter((group) => group.action === "conflict" && !choiceReady(chosen[groupKey(group.book_path, group.section_id)]));
}

export function startHint(pending: string[], conflictsLeft: number): string {
  const songbook = pending.some((key) => key.startsWith("[\"songbook\""));
  const plan = pending.some((key) => key.startsWith("[\"plan\""));
  if (songbook && !plan && conflictsLeft === 0) return "Сначала выберите: добавить новый сборник в папку или пропустить его.";
  const parts: string[] = [];
  if (songbook) parts.push("добавить новый сборник в папку или пропустить его");
  if (plan) parts.push("добавить новый план в папку или пропустить его");
  if (conflictsLeft) parts.push("выбрать версию в конфликте");
  if (!parts.length) return "";
  return `Сначала выберите: ${parts.join("; ")}.`;
}

function songCountLine(count: number, tail: string): string {
  return `${countPhrase(count, "песня", "песни", "песен")} ${tail}`;
}

function listedBooks(preview: PreviewData, chosen: Record<string, Choice>, adopted: Record<string, true>): ListedRow[] {
  const rows: ListedRow[] = [];
  for (const bucket of bookBuckets(preview.songs)) {
    const unresolved = openConflicts(bucket, chosen);
    const adoptedNow = Boolean(bucket.serverId && adopted[adoptionKey("songbook", bucket.serverId)]);
    let status = "";
    let tone: RowTone = "ok";
    let rowClass: ListedRow["rowClass"] = "";
    let aside = "";
    let mode: RowMode = "plain";
    if (unresolved.length) {
      status = "Нужно выбрать";
      tone = "conflict";
      rowClass = "conflict";
      mode = "compare";
    } else if (bucket.absent && !adoptedNow) {
      status = "Новое на сервере";
      tone = "pull";
      rowClass = "add";
      mode = "adopt";
    } else if (bucket.absent) {
      status = "Новое на сервере";
      tone = "pull";
      rowClass = "add";
      aside = "добавится в Modules";
    } else if (bucket.push || bucket.songs.some((song) => decided(song, chosen, "push"))) {
      status = "Изменено здесь";
      tone = "push";
      aside = "из VisioBible";
    } else if (bucket.pull || bucket.songs.some((song) => decided(song, chosen, "pull"))) {
      status = "Изменено на сервере";
      tone = "pull";
      aside = "запишется при синхронизации";
    } else {
      continue;
    }
    const meta = unresolved.length
      ? unresolved.length === 1
        ? `«${unresolved[0].title}» — изменена здесь и на сервере`
        : `${countPhrase(unresolved.length, "песня изменена", "песни изменены", "песен изменены")} здесь и на сервере`
      : bucket.absent
        ? `${songCountLine(bucket.songs.length, "на сервере")} · в папке VisioBible этого файла ещё нет`
        : [
            bucket.push ? `${countPhrase(bucket.push, "песня отправится", "песни отправятся", "песен отправятся")} на сервер` : "",
            bucket.pull ? `${songCountLine(bucket.pull, "придут с сервера")}` : "",
            songCountLine(bucket.songs.length, "в файле"),
          ]
            .filter(Boolean)
            .join(" · ");
    rows.push({
      key: bucket.path,
      title: bucket.title,
      meta,
      status,
      tone,
      rowClass,
      aside,
      mode,
      serverId: bucket.serverId,
      kind: "songbook",
      conflictId: unresolved[0]?.fbid ?? "",
    });
  }
  for (const group of openGroupConflicts(preview, chosen)) {
    rows.push({
      key: groupKey(group.book_path, group.section_id),
      title: group.book_title || "Сборник",
      meta: `Группу переименовали: в VisioBible «${group.local_name}», на сервере «${group.remote_name}»`,
      status: "Нужно выбрать",
      tone: "conflict",
      rowClass: "conflict",
      aside: "",
      mode: "name",
      serverId: group.section_id,
      kind: "group",
      conflictId: group.book_path,
    });
  }
  return rows;
}

function listedPlans(preview: PreviewData, chosen: Record<string, Choice>, adopted: Record<string, true>): ListedRow[] {
  const rows: ListedRow[] = [];
  for (const plan of preview.plans) {
    const adoptedNow = Boolean(plan.server_id && adopted[adoptionKey("plan", plan.server_id)]);
    const open = plan.action === "conflict" && !choiceReady(chosen[planKey(plan.key)]);
    let status = "";
    let tone: RowTone = "ok";
    let rowClass: ListedRow["rowClass"] = "";
    let aside = "";
    let mode: RowMode = "plain";
    let meta = "";
    if (open) {
      status = "Нужно выбрать";
      tone = "conflict";
      rowClass = "conflict";
      mode = "choose";
      meta = `На этом компьютере: ${plan.local_summary}. На сервере: ${plan.remote_summary}`;
    } else if (plan.absent && !adoptedNow) {
      status = "Новое на сервере";
      tone = "pull";
      rowClass = "add";
      mode = "adopt";
      meta = "Есть на сервере · в папке VisioBible этого файла ещё нет";
    } else if (plan.absent) {
      status = "Новое на сервере";
      tone = "pull";
      rowClass = "add";
      aside = "добавится в Modules";
      meta = "Есть на сервере · в папку VisioBible этого файла ещё нет";
    } else if (plan.action === "conflict" && chosen[planKey(plan.key)]?.kind === "take_server") {
      status = "Изменено на сервере";
      tone = "pull";
      aside = "запишется при синхронизации";
      meta = "Изменён на сервере · в VisioBible пока старая копия";
    } else if (plan.action.startsWith("push") || plan.action === "conflict") {
      status = "Изменено в VisioBible";
      tone = "push";
      meta = "Файл в VisioBible изменили локально · на сервер уйдёт при синхронизации";
    } else if (plan.action.startsWith("pull")) {
      status = "Изменено на сервере";
      tone = "pull";
      aside = "запишется при синхронизации";
      meta = "Изменён на сервере · в VisioBible пока старая копия";
    } else {
      continue;
    }
    rows.push({
      key: plan.key,
      title: plan.name,
      meta,
      status,
      tone,
      rowClass,
      aside,
      mode,
      serverId: plan.server_id,
      kind: "plan",
      conflictId: "",
    });
  }
  return rows;
}

export function syncReady(
  preview: PreviewData | null,
  chosen: Record<string, Choice>,
  adopted: Record<string, true>,
  skips: SkipLists = { songbooks: [], plans: [] },
): boolean {
  if (!preview) return false;
  const visible = withoutRememberedSkips(preview, skips);
  return canStart(conflictRows(visible.songs, visible.plans, visible.groups), chosen, pendingAdoption(visible, adopted));
}

export interface SyncScreen {
  banner: string;
  chips: SyncChip[];
  books: ListedRow[];
  plans: ListedRow[];
  hint: string;
  ready: boolean;
  bible: string;
}

export function syncScreen(
  preview: PreviewData,
  chosen: Record<string, Choice>,
  adopted: Record<string, true>,
  bibles: { short_name: string; name: string }[],
  skips: SkipLists = { songbooks: [], plans: [] },
): SyncScreen {
  const visible = withoutRememberedSkips(preview, skips);
  const counts = syncCounts({ ...visible, skipped: preview.skipped });
  const pending = pendingAdoption(visible, adopted);
  const conflictsLeft = conflictRows(visible.songs, visible.plans, visible.groups).filter((row) => !choiceReady(chosen[row.key])).length;
  const banner = bannerText(visible, chosen);
  const chips: SyncChip[] = [
    { id: "send", text: `${counts.send} отправить`, tone: counts.send ? "plain" : "zero" },
    { id: "receive", text: `${counts.receive} получить`, tone: counts.receive ? "plain" : "zero" },
    { id: "fresh", text: freshChipText(counts.freshBooks, counts.freshPlans), tone: counts.freshBooks + counts.freshPlans ? "ok" : "zero" },
  ];
  if (counts.conflicts) chips.push({ id: "conflicts", text: countPhrase(counts.conflicts, "конфликт", "конфликта", "конфликтов"), tone: "bad" });
  chips.push(
    { id: "skipped", text: countPhrase(counts.skipped, "пропущен", "пропущено", "пропущено"), tone: "zero" },
    { id: "same", text: `${counts.unchanged} без изменений`, tone: "zero" },
  );
  return {
    banner,
    chips,
    books: listedBooks(visible, chosen, adopted),
    plans: listedPlans(visible, chosen, adopted),
    hint: startHint(pending, conflictsLeft),
    ready: pending.length === 0 && conflictsLeft === 0,
    bible: bibleLine(bibles),
  };
}
