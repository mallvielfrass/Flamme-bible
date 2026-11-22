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

export function canStart(rows: { action: string; key: string }[], chosen: Record<string, Choice>): boolean {
  return unresolved(rows, chosen).length === 0;
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
