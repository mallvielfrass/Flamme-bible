export interface BibleLink {
  path: string;
  short_name: string;
  title: string;
  content_hash: string;
  server_id: string;
  uploaded: boolean;
}

export function bibleStatus(link: Pick<BibleLink, "server_id" | "uploaded"> | undefined): string {
  if (!link || !link.server_id) return "Ещё не сверено с сервером";
  if (link.uploaded) return "Отправлено на сервер";
  return "Уже на сервере";
}
