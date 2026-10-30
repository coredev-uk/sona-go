/** Formats milliseconds as m:ss or h:mm:ss. */
export function formatTime(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}

/** A total running time for the catalogue line: "42 min", "1 hr 12 min". */
export function formatRunningTime(ms: number): string {
  const minutes = Math.round(ms / 60_000);
  if (minutes < 60) return `${minutes} min`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m ? `${h} hr ${m} min` : `${h} hr`;
}

/** "1 song", "12 songs". */
export function plural(n: number, one: string, many = `${one}s`): string {
  return `${n.toLocaleString()} ${n === 1 ? one : many}`;
}

/** The year of an ISO date ("2008-06-02" or "2008"), or "". */
export function yearOf(date: string | undefined): string {
  const m = date?.match(/^(\d{4})/);
  return m ? m[1] : "";
}

/**
 * A release date in words: "10 October 2007" (`month: "short"`: "10 Oct
 * 2007") for a full ISO date, the year for a partial one, "" for none.
 */
export function longDate(date: string | undefined, month: "long" | "short" = "long"): string {
  if (!date || !/^\d{4}-\d{2}-\d{2}$/.test(date)) return yearOf(date);
  return new Date(`${date}T12:00:00`).toLocaleDateString(undefined, {
    year: "numeric",
    month,
    day: "numeric",
  });
}

/**
 * The catalogue line (docs/design.md): `Album · 2008 · 10 songs, 42 min`.
 * Empty parts are dropped; the song count and running time share a slot.
 */
export function catalogueLine(
  kind: string,
  year: string,
  songs?: number,
  runningMs?: number,
): string {
  const count = songs ? plural(songs, "song") : "";
  const time = runningMs ? formatRunningTime(runningMs) : "";
  const tail = [count, time].filter(Boolean).join(", ");
  return [kind, year, tail].filter(Boolean).join(" · ");
}

/** "3 minutes ago" for a Unix-ms stamp; 0 means never. */
export function relativeTime(t: number, now = Date.now()): string {
  if (!t || t <= 0) return "never";
  const s = Math.round((now - t) / 1000);
  if (s < 45) return "just now";
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto", style: "short" });
  const m = Math.round(s / 60);
  if (m < 60) return rtf.format(-m, "minute");
  const h = Math.round(m / 60);
  if (h < 24) return rtf.format(-h, "hour");
  return rtf.format(-Math.round(h / 24), "day");
}

/** Bytes as "12.4 MB". */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`;
}
