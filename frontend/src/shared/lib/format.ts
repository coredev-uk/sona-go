import { i18n } from "@lingui/core";
import { plural, t } from "@lingui/core/macro";

/**
 * Number, date and duration formatting in the active locale: the regional
 * tags `shared/i18n` activated for the shown catalogue (`i18n.locales`,
 * e.g. en-GB for the English catalogue on a British system).
 * Formatters are cached per locale and options: building an `Intl` object
 * costs far more than formatting with one, and rows call these per render.
 */
const formatters = new Map<string, unknown>();

/** The locales `Intl` should format with now. */
export function formatLocales(): string[] {
  const l = i18n.locales ?? i18n.locale;
  return typeof l === "string" ? [l] : l;
}

function cached<F>(kind: string, options: object, make: (locales: string[]) => F): F {
  const locales = formatLocales();
  const key = `${locales.join()}|${kind}|${JSON.stringify(options)}`;
  let f = formatters.get(key) as F | undefined;
  if (!f) {
    f = make(locales);
    formatters.set(key, f);
  }
  return f;
}

/** An `Intl.NumberFormat` for the active locale. */
export function numberFormat(options: Intl.NumberFormatOptions = {}): Intl.NumberFormat {
  return cached("number", options, (l) => new Intl.NumberFormat(l, options));
}

/** An `Intl.DateTimeFormat` for the active locale. */
export function dateFormat(options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  return cached("date", options, (l) => new Intl.DateTimeFormat(l, options));
}

/** A number in the active locale: "1,234". */
export function formatNumber(n: number): string {
  return numberFormat().format(n);
}

/** Formats milliseconds as m:ss or h:mm:ss. */
export function formatTime(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}

/** A total running time for the catalogue line: "42 min", "1 hr, 12 min". */
export function formatRunningTime(ms: number): string {
  const minutes = Math.round(ms / 60_000);
  const duration = { hours: Math.floor(minutes / 60), minutes: minutes % 60 };
  return cached("duration", {}, (l) => new Intl.DurationFormat(l, { style: "short" })).format(
    duration,
  );
}

/**
 * The time left until a moment, in words and to the minute, never finer:
 * days and hours a day or more ahead, then hours and minutes ("18 days,
 * 6 hours", "5 hours, 12 minutes", "1 minute").
 */
export function formatTimeLeft(ms: number): string {
  const minutes = Math.max(1, Math.ceil(ms / 60_000));
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  const duration =
    days > 0 ? { days, hours } : hours > 0 ? { hours, minutes: minutes % 60 } : { minutes };
  return cached("duration-long", {}, (l) => new Intl.DurationFormat(l, { style: "long" })).format(
    duration,
  );
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
  const [y, m, d] = date.split("-").map(Number);
  // Midday local time, so no time zone moves it to another day.
  return dateFormat({ year: "numeric", month, day: "numeric" }).format(new Date(y, m - 1, d, 12));
}

/** A clock time in the active locale: "21:30" or "9:30 PM". */
export function clockTime(t: number | Date): string {
  return dateFormat({ hour: "2-digit", minute: "2-digit" }).format(t);
}

/**
 * The catalogue line (docs/design.md): `Album · 2008 · 10 songs, 42 min`.
 * Empty parts are dropped; the song count and running time share a slot.
 * `kind` is already translated.
 */
export function catalogueLine(
  kind: string,
  year: string,
  songs?: number,
  runningMs?: number,
): string {
  const count = songs ? plural(songs, { one: "# song", other: "# songs" }) : "";
  const time = runningMs ? formatRunningTime(runningMs) : "";
  const tail = count && time ? t`${count}, ${time}` : count || time;
  return [kind, year, tail].filter(Boolean).join(" · ");
}

/** "3 minutes ago" for a Unix-ms stamp; 0 means never. */
export function relativeTime(stamp: number, now = Date.now()): string {
  if (!stamp || stamp <= 0) return t`never`;
  const s = Math.round((now - stamp) / 1000);
  if (s < 45) return t`just now`;
  const rtf = cached(
    "relative",
    {},
    (l) => new Intl.RelativeTimeFormat(l, { numeric: "auto", style: "short" }),
  );
  const m = Math.round(s / 60);
  if (m < 60) return rtf.format(-m, "minute");
  const h = Math.round(m / 60);
  if (h < 24) return rtf.format(-h, "hour");
  return rtf.format(-Math.round(h / 24), "day");
}

const byteUnits = ["kilobyte", "megabyte", "gigabyte", "terabyte"] as const;

/** Bytes as "12.4 MB" in the active locale. */
export function formatBytes(n: number): string {
  if (n < 1024) {
    return numberFormat({ style: "unit", unit: "byte", unitDisplay: "short" }).format(n);
  }
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < byteUnits.length - 1) {
    v /= 1024;
    i++;
  }
  const digits = v < 10 ? 1 : 0;
  return numberFormat({
    style: "unit",
    unit: byteUnits[i],
    unitDisplay: "short",
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(v);
}
