import type { PlanItem } from "./commands";

export const planFormats = [
  { id: "vplt", label: "plan.vplt" },
  { id: "history", label: "History.ini" },
  { id: "plandef", label: "PlanDef.ini" },
  { id: "json", label: "JSON" },
] as const;

export type PlanFormat = (typeof planFormats)[number]["id"];

export type DeleteScope = "local" | "server" | "both";

export function importFormatFromName(name: string): PlanFormat | "" {
  const file = name.split(/[/\\]/).pop()?.toLowerCase() ?? "";
  if (file.endsWith(".vplt")) return "vplt";
  if (file.includes("history")) return "history";
  if (file.includes("plandef")) return "plandef";
  if (file.endsWith(".json")) return "json";
  return "";
}

export function exportFileName(format: string): string {
  if (format === "vplt") return "plan.vplt";
  if (format === "history") return "History.ini";
  if (format === "plandef") return "PlanDef.ini";
  return "plan.json";
}

export function exportExtension(format: string): string {
  if (format === "vplt") return "vplt";
  if (format === "json") return "json";
  return "ini";
}

export function fileLabel(path: string): string {
  const parts = path.split(/[/\\]/);
  return parts[parts.length - 1] || path;
}

export function duplicateTitle(title: string, existing: string[]): string {
  const base = `Копия — ${title.trim() || "План"}`;
  const names = new Set(existing);
  if (!names.has(base)) return base;
  let index = 2;
  while (names.has(`${base} (${index})`)) index += 1;
  return `${base} (${index})`;
}

export function clonePlanItems(items: PlanItem[]): PlanItem[] {
  return items.map((item) => {
    const copy = { ...item };
    delete copy.id;
    delete copy.position;
    return copy;
  });
}

export function pointsLabel(count: number): string {
  const value = count || 0;
  const mod10 = value % 10;
  const mod100 = value % 100;
  if (mod10 === 1 && mod100 !== 11) return `${value} пункт`;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return `${value} пункта`;
  return `${value} пунктов`;
}

export function deleteScope(localPath: string, serverId: string, loggedIn: boolean): DeleteScope {
  const local = localPath.trim() !== "";
  const server = loggedIn && serverId.trim() !== "";
  if (local && server) return "both";
  if (server) return "server";
  return "local";
}

export function deletePrompt(input: {
  name: string;
  count: number;
  scope: DeleteScope;
  place?: string;
  keptOnServer?: boolean;
}): { title: string; text: string } {
  const title = "Удалить план?";
  const label = input.name.trim() || "План";
  const points = pointsLabel(input.count);
  const place = (input.place || "").trim();
  if (input.scope === "local") {
    const tail = input.keptOnServer ? " Копия на сервере останется." : "";
    return { title, text: `Удалить план «${label}» с этого компьютера? В нём ${points}.${tail}` };
  }
  const where = place ? ` из общины «${place}»` : "";
  if (input.scope === "server") {
    const tail = place
      ? "План пропадёт у всех участников общины и на компьютерах после следующей синхронизации."
      : "План пропадёт на ваших компьютерах после следующей синхронизации.";
    return { title, text: `Удалить план «${label}»${where}? В нём ${points}. ${tail}` };
  }
  const tail = place
    ? "Файл на этом компьютере удалится сразу. У участников общины план пропадёт после следующей синхронизации."
    : "Файл на этом компьютере удалится сразу. На других компьютерах план пропадёт после следующей синхронизации.";
  return { title, text: `Удалить план «${label}»${where} на сервере и с этого компьютера? В нём ${points}. ${tail}` };
}
