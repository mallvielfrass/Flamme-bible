export function formatSize(size: number): string {
  const n = Number(size) || 0;
  if (n <= 0) return "";
  if (n < 1024) return `${n} Б`;
  if (n < 1024 * 1024) return `${trim(n / 1024)} КБ`;
  return `${trim(n / (1024 * 1024))} МБ`;
}

export function shortHash(hash: string): string {
  const clean = hash.trim().toLowerCase();
  if (clean.length <= 32) return clean;
  return `${clean.slice(0, 32)}…`;
}

function trim(value: number): string {
  const text = value.toFixed(1);
  return text.endsWith(".0") ? text.slice(0, -2) : text;
}
