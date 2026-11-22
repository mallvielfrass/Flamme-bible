import { invoke } from "@tauri-apps/api/core";
import { save } from "@tauri-apps/plugin-dialog";
import {
  exportFileName,
  type Community,
  type LocalMark,
  type Plan,
  type PlanItem,
  type SongbookGroup,
  type SongHit,
} from "./model";

export interface PlanResult {
  plan: Plan;
  conflict: boolean;
}

export interface ImportedPlan extends Plan {
  mojibake?: boolean;
  encoding_warning?: string;
  suggested_text?: string;
  suggested_title?: string;
  suggested_items?: PlanItem[];
}

export interface ParsedPlan {
  title: string;
  items: PlanItem[];
  encoding_notice?: {
    mojibake?: boolean;
    encoding_warning?: string;
    suggested_text?: string;
    suggested_items?: PlanItem[];
  };
}

export interface SongText {
  title: string;
  songbook_title: string;
  section_title: string;
  text: string;
}

export interface VerseText {
  module_short_name: string;
  module_title: string;
  text: string;
}

export interface PlansApi {
  list(): Promise<Plan[]>;
  read(id: string): Promise<Plan>;
  create(title: string, dimensionId: string, items: PlanItem[]): Promise<Plan>;
  save(plan: Plan): Promise<PlanResult>;
  remove(plan: Plan): Promise<PlanResult>;
  importPlan(body: { format: string; payload: string; title: string; dimension_id: string }): Promise<ImportedPlan>;
  parse(format: string, payload: string): Promise<ParsedPlan>;
  download(id: string, format: string, encoding: string): Promise<boolean>;
  folder(): Promise<LocalMark[]>;
  songbooks(): Promise<SongbookGroup[]>;
  findSongs(query: string, songbookId: string, sectionId: string): Promise<SongHit[]>;
  communities(): Promise<Community[]>;
  songText(songbookId: string, fbid: string): Promise<SongText>;
  verseText(module: string, book: number, chapter: number, verse: number): Promise<VerseText>;
}

function extension(format: string): string {
  if (format === "json") return "json";
  if (format === "vplt") return "vplt";
  return "ini";
}

export function explain(err: unknown): string {
  if (typeof err === "string" && err.trim()) return err;
  if (err instanceof Error && err.message.trim()) return err.message;
  return "Не удалось выполнить запрос";
}

export const plansApi: PlansApi = {
  list: () => invoke<Plan[]>("plans_list"),
  read: (id) => invoke<Plan>("plans_read", { id }),
  create: (title, dimensionId, items) =>
    invoke<Plan>("plans_create", { title, dimensionId, items }),
  save: (plan) =>
    invoke<PlanResult>("plans_save", {
      id: plan.id,
      title: plan.title,
      version: plan.version,
      updatedAt: plan.updated_at || "",
      items: plan.items,
    }),
  remove: (plan) =>
    invoke<PlanResult>("plans_delete", {
      id: plan.id,
      version: plan.version,
      updatedAt: plan.updated_at || "",
    }),
  importPlan: (body) =>
    invoke<ImportedPlan>("plans_import", {
      format: body.format,
      payload: body.payload,
      title: body.title,
      dimensionId: body.dimension_id,
    }),
  parse: (format, payload) => invoke<ParsedPlan>("plans_parse", { format, payload }),
  async download(id, format, encoding) {
    const picked = await save({
      defaultPath: exportFileName(format),
      filters: [{ name: "План", extensions: [extension(format)] }],
    });
    if (typeof picked !== "string" || !picked) return false;
    await invoke("plans_download", { id, format, encoding, path: picked });
    return true;
  },
  folder: () => invoke<LocalMark[]>("plans_folder"),
  songbooks: () => invoke<SongbookGroup[]>("plans_songbooks"),
  songText: (songbookId, fbid) => invoke<SongText>("plans_song_text", { songbookId, fbid }),
  verseText: (module, book, chapter, verse) => invoke<VerseText>("plans_verse_text", { module, book, chapter, verse }),
  findSongs: (query, songbookId, sectionId) =>
    invoke<SongHit[]>("plans_find_songs", { query, songbookId, sectionId }),
  async communities() {
    const list = await invoke<Community[]>("list_communities");
    return (list || []).map((item) => ({ id: item.id, name: item.name || "Община" }));
  },
};
