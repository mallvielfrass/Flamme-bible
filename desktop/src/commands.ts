import { invoke } from "@tauri-apps/api/core";
import type { PlanDecision, SongDecision } from "./syncState";

export interface Settings {
  server_url: string;
  email: string;
  logged_in: boolean;
  visio_path: string;
  user_id: string;
  dimension_id: string;
  role: string;
  plan_windows1251: boolean;
}

export interface BibleBook {
  short_name: string;
  full_name: string;
}

export interface Scan {
  root: string;
  modules_dir: string;
  plans_dir: string;
  needs_choice: boolean;
  catalogs: string[];
  module_refs: string[];
  songbooks: { path: string; title: string; short_name: string; song_count: number; titles: string[]; error: string }[];
  bibles: { path: string; name: string; short_name: string; books: BibleBook[] }[];
  plans: {
    path: string;
    name: string;
    kind: string;
    items: PlanItem[];
    mojibake?: boolean;
    encoding_warning?: string;
    preview_before?: string;
    preview_after?: string;
  }[];
  plan_def: { name: string; module_short_name: string; module_short_name_second: string; journals: string[] };
}

export interface SongRow {
  book_path: string;
  book_title: string;
  fbid: string;
  title: string;
  action: string;
  local_title: string;
  local_lyrics: string;
  remote_title: string;
  remote_lyrics: string;
  server_id: string;
  absent: boolean;
}

export interface PlanRow {
  key: string;
  path: string;
  name: string;
  action: string;
  summary: string;
  local_summary: string;
  remote_summary: string;
  server_id: string;
  absent: boolean;
}

export interface Preview {
  songs: SongRow[];
  plans: PlanRow[];
  warnings: string[];
  grouped: { title: string; note: string }[];
  skipped: { kind: string; id: string; title: string }[];
}

export interface PlanItem {
  kind: string;
  title: string;
  song_id: string;
  songbook_id: string;
  caption: string;
  quote: string;
  reference: string;
  class: string;
  module_short_name: string;
  module_short_name_second: string;
  book: number;
  chapter: number;
  verse: number;
}

export interface Progress {
  stage: string;
  current: string;
  completed: number;
  total: number;
  percentage: number;
}

export interface Dimension {
  id: string;
  kind: string;
  name: string;
}

export interface SongHit {
  songbook_id: string;
  fbid: string;
  title: string;
  songbook_title: string;
}

export interface UpdateView {
  current: string;
  latest: string;
  notes: string;
  available: boolean;
  platform: string;
}

export const api = {
  version: () => invoke<string>("client_version"),
  loadSettings: () => invoke<Settings>("load_settings"),
  saveSettings: (server_url: string) => invoke<Settings>("save_settings", { server_url }),
  setPlanEncoding: (windows1251: boolean) => invoke<Settings>("set_plan_encoding", { windows1251 }),
  repairPlanEncoding: (path: string) => invoke<Scan>("repair_plan_encoding", { path }),
  register: (email: string, password: string) => invoke<Settings>("register", { email, password }),
  login: (email: string, password: string) => invoke<Settings>("login", { email, password }),
  logout: () => invoke<Settings>("logout"),
  dimensions: () => invoke<Dimension[]>("list_dimensions"),
  selectDimension: (dimension_id: string) => invoke<Settings>("select_dimension", { dimension_id }),
  openVisio: (path: string) => invoke<Scan>("open_visio", { path }),
  preview: () => invoke<Preview>("preview_sync"),
  start: (songs: SongDecision[], plans: PlanDecision[]) =>
    invoke<{ notes: string[]; backups: string[] }>("start_sync", { request: { songs, plans } }),
  setSkipped: (kind: "songbook" | "plan", id: string, skip: boolean) =>
    invoke<void>("set_skipped", { kind, id, skip }),
  createPlan: (name: string, items: PlanItem[]) => invoke<Scan["plans"][number]>("create_plan", { name, items }),
  searchSongs: (query: string) => invoke<SongHit[]>("search_songs", { query }),
  checkUpdate: () => invoke<UpdateView>("check_update"),
  downloadUpdate: () => invoke<string>("download_update"),
  setCloseGuard: (enabled: boolean) => invoke<void>("set_close_guard", { enabled }),
  finishClose: () => invoke<void>("finish_close"),
};
