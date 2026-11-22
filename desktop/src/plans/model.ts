import {
  appendItems as appendPlanItems,
  bibleItem,
  cloneItems as clonePlanItems,
  countByKind as countPlanKinds,
  exportFileName,
  importFormatFromName,
  itemTitle,
  maxPlanItems,
  moveItem as movePlanItem,
  planUpdatedLabel,
  pointsLabel,
  songItem,
  textItem,
} from "../../../web/src/planItems.js";
import {
  clipPlanText,
  importEncodingNotice,
  readPlanFile,
} from "../../../web/src/checks.js";

export {
  clipPlanText,
  exportFileName,
  importEncodingNotice,
  importFormatFromName,
  itemTitle,
  maxPlanItems,
  planUpdatedLabel,
  pointsLabel,
  readPlanFile,
};

export interface PlanItem {
  id?: string;
  kind: string;
  song_id?: string;
  songbook_id?: string;
  title?: string;
  module_short_name?: string;
  module_short_name_second?: string;
  book?: number;
  chapter?: number;
  verse?: number;
  caption?: string;
  reference?: string;
  quote?: string;
  class?: string;
  position?: number;
  created_at?: string;
  updated_at?: string;
  songbook_title?: string;
  section_title?: string;
}

export interface Plan {
  id: string;
  dimension_id: string | null;
  title: string;
  version: number;
  updated_at: string;
  items: PlanItem[];
}

export interface LocalMark {
  title: string;
  server_id: string;
}

export interface Community {
  id: string;
  name: string;
}

export interface SongHit {
  songbook_id: string;
  fbid: string;
  title: string;
  songbook_title: string;
  section_title?: string;
  status?: string;
}

export interface SongbookSection {
  id: string;
  title: string;
  count: number;
}

export interface SongbookNode {
  id: string;
  title: string;
  short_name: string;
  sections: SongbookSection[];
}

export interface SongbookGroup {
  group: string;
  name: string;
  songbooks: SongbookNode[];
}

export function kindLabel(kind: string): string {
  if (kind === "song") return "Песня";
  if (kind === "bible") return "Стих";
  return "Текст";
}

export function visiblePlans(plans: Plan[], space: string): Plan[] {
  return plans.filter((plan) => {
    if (space === "all") return true;
    if (space === "personal") return !plan.dimension_id;
    if (space === "community") return Boolean(plan.dimension_id);
    return plan.dimension_id === space;
  });
}

export function planCounts(plans: Plan[]): Record<string, number> {
  const counts: Record<string, number> = { all: plans.length, personal: 0, community: 0 };
  for (const plan of plans) {
    if (plan.dimension_id) {
      counts.community += 1;
      counts[plan.dimension_id] = (counts[plan.dimension_id] || 0) + 1;
    } else {
      counts.personal += 1;
    }
  }
  return counts;
}

export function createDimension(space: string, communityIds: string[]): string {
  return communityIds.includes(space) ? space : "";
}

export function planInFolder(plan: { id: string; title: string }, locals: LocalMark[]): boolean {
  if (locals.some((item) => item.server_id === plan.id)) return true;
  const same = locals.filter((item) => item.title !== "" && item.title === plan.title);
  if (same.length !== 1) return false;
  const hit = same[0];
  return hit.server_id === "" || hit.server_id === plan.id;
}

export function folderHint(inFolder: boolean): string {
  return inFolder
    ? "Есть в VisioBible · файл обновится при синхронизации"
    : "Только на сервере · появится в папке после синхронизации";
}

export function folderMeta(inFolder: boolean): string {
  return inFolder ? "есть в VisioBible" : "только на сервере";
}

export function copyTitle(title: string): string {
  const name = title.trim() || "План";
  return `Копия — ${name}`;
}

export function badgeFor(dimensionId: string | null, communities: Community[]): string {
  if (!dimensionId) return "Личный";
  return communities.find((item) => item.id === dimensionId)?.name || "Община";
}

export function makeSong(hit: SongHit, title: string): PlanItem {
  return songItem(hit, title) as PlanItem;
}

export function makeBible(fields: PlanItem): PlanItem {
  return bibleItem(fields) as PlanItem;
}

export function makeText(fields: PlanItem): PlanItem {
  return textItem(fields) as PlanItem;
}

export function moveItem(items: PlanItem[], index: number, delta: number): PlanItem[] {
  return movePlanItem(items, index, delta) as PlanItem[];
}

export function cloneItems(items: PlanItem[]): PlanItem[] {
  return clonePlanItems(items) as PlanItem[];
}

export function appendItems(
  items: PlanItem[],
  extra: PlanItem[],
  options: { kinds: string[]; skipSongs: boolean },
): PlanItem[] {
  return appendPlanItems(items, extra, options) as PlanItem[];
}

export function countByKind(items: PlanItem[]): { song: number; bible: number; text: number } {
  return countPlanKinds(items);
}

export function snap(plan: { title: string; items: PlanItem[] }): string {
  return JSON.stringify({
    title: plan.title || "",
    items: (plan.items || []).map((item) => ({
      id: item.id || "",
      kind: item.kind,
      song_id: item.song_id || "",
      songbook_id: item.songbook_id || "",
      title: item.title || "",
      module_short_name: item.module_short_name || "",
      module_short_name_second: item.module_short_name_second || "",
      book: item.book || 0,
      chapter: item.chapter || 0,
      verse: item.verse || 0,
      caption: item.caption || "",
      reference: item.reference || "",
      quote: item.quote || "",
      class: item.class || "",
    })),
  });
}
