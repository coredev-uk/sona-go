import { t } from "@lingui/core/macro";
import type { Concert } from "@bindings/services";

import { dateFormat, formatLocales } from "@/shared/lib/format";

/**
 * Dates on the Concerts page. A show happens on the venue's calendar day,
 * at the venue's clock: Go sends the instant (`startsAt`) and the venue's
 * IANA zone, and every day, time and month here is read in that zone, in
 * the language of the interface. The day filters are the user's own
 * calendar days, "2026-11-14".
 */

/** The date filter's choices; "dates" is a range the user types in. */
export const whenValues = ["week", "weekend", "month", "dates"] as const;
export type When = (typeof whenValues)[number];

/** A calendar day as "2026-11-14". */
export function isoDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** The days a preset covers from `now`, both ends included; undefined for "any time". */
export function presetRange(
  when: Exclude<When, "dates"> | undefined,
  now: Date,
): { from: string; to: string } | undefined {
  if (!when) return undefined;
  const day = (offset: number) =>
    new Date(now.getFullYear(), now.getMonth(), now.getDate() + offset);
  const weekday = now.getDay(); // 0 is Sunday
  switch (when) {
    case "week":
      // Today to Sunday: the rest of the week as people count it.
      return { from: isoDay(day(0)), to: isoDay(day((7 - weekday) % 7)) };
    case "weekend": {
      // Friday to Sunday; from today when it is already the weekend.
      const toFriday = weekday === 0 ? -2 : weekday === 6 ? -1 : 5 - weekday;
      const toSunday = (7 - weekday) % 7;
      return { from: isoDay(day(Math.max(toFriday, 0))), to: isoDay(day(toSunday)) };
    }
    case "month":
      return { from: isoDay(day(0)), to: isoDay(day(29)) };
  }
}

/** The range a filter asks for: a preset, or the dates typed in (both ends or none). */
export function rangeFor(
  f: { when?: When; from?: string; to?: string },
  now: Date,
): { from: string; to: string } | undefined {
  if (f.when === "dates") {
    return f.from && f.to && f.from <= f.to ? { from: f.from, to: f.to } : undefined;
  }
  return presetRange(f.when, now);
}

type Show = Pick<Concert, "startsAt" | "timezone">;

const parts = (c: Show, o: Intl.DateTimeFormatOptions) =>
  dateFormat({ timeZone: c.timezone, ...o }).formatToParts(c.startsAt);

/** The show's calendar day at the venue, "2026-11-14". */
export function showDay(c: Show): string {
  const get = (type: string) =>
    parts(c, { year: "numeric", month: "2-digit", day: "2-digit" }).find((p) => p.type === type)
      ?.value ?? "";
  // formatToParts reads the locale's numerals; the key is for grouping only.
  return `${get("year")}-${get("month")}-${get("day")}`;
}

/** "November 2026". */
export function showMonth(c: Show): string {
  return dateFormat({ timeZone: c.timezone, month: "long", year: "numeric" }).format(c.startsAt);
}

/** "Wed", the day's weekday in short. */
export function showWeekday(c: Show): string {
  return dateFormat({ timeZone: c.timezone, weekday: "short" }).format(c.startsAt);
}

/** "14", the day of the month. */
export function showDate(c: Show): string {
  return dateFormat({ timeZone: c.timezone, day: "numeric" }).format(c.startsAt);
}

/** "Wed 14 Apr": the whole day, for where it stands without a day heading. */
export function showDayLong(c: Show): string {
  return dateFormat({
    timeZone: c.timezone,
    weekday: "short",
    day: "numeric",
    month: "short",
  }).format(c.startsAt);
}

/** Whether the venue keeps the same clock as this computer at the show's time. */
function sameClock(c: Show): boolean {
  const here = dateFormat({ timeZoneName: "longOffset" }).format(c.startsAt).split(" ").pop();
  const there = dateFormat({ timeZone: c.timezone, timeZoneName: "longOffset" })
    .format(c.startsAt)
    .split(" ")
    .pop();
  return here === there;
}

/**
 * The show's start at the venue's clock, "18:00" (or "6:00 pm"): with the
 * zone's name ("EDT") when that clock is not this computer's, so a show in
 * another country is never read as local time.
 */
export function showTime(c: Show): string {
  const base = { timeZone: c.timezone, hour: "numeric", minute: "2-digit" } as const;
  return sameClock(c)
    ? dateFormat(base).format(c.startsAt)
    : dateFormat({ ...base, timeZoneName: "short" }).format(c.startsAt);
}

/** A group of shows on one day. */
export interface DayGroup {
  day: string;
  shows: Concert[];
}
/** A month of shows, by the venue's calendar. */
export interface MonthGroup {
  key: string;
  title: string;
  days: DayGroup[];
}

/** The shows (already soonest first) grouped by month, then by day. */
export function groupShows(shows: readonly Concert[]): MonthGroup[] {
  const months: MonthGroup[] = [];
  for (const c of shows) {
    const day = showDay(c);
    const key = day.slice(0, 7);
    let month = months.at(-1);
    if (month?.key !== key) {
      month = { key, title: showMonth(c), days: [] };
      months.push(month);
    }
    let group = month.days.at(-1);
    if (group?.day !== day) {
      group = { day, shows: [] };
      month.days.push(group);
    }
    group.shows.push(c);
  }
  return months;
}

/** Regions whose roads, and so whose "how far", are in miles. */
// eslint-disable-next-line lingui/no-unlocalized-strings -- ISO region codes, not text
const mileRegions = new Set(["US", "GB", "LR", "MM"]);

function regionOfLocale(): string {
  const tag = formatLocales()[0] ?? "en";
  try {
    return new Intl.Locale(tag).maximize().region ?? "";
  } catch {
    return "";
  }
}

/** "12 km away", or "7 mi away" where distances are in miles; "" when unknown. */
export function distanceLabel(km: number): string {
  if (km <= 0) return "";
  const miles = mileRegions.has(regionOfLocale());
  const value = miles ? Math.max(1, Math.round(km * 0.621371)) : km;
  const text = new Intl.NumberFormat(formatLocales(), {
    style: "unit",
    unit: miles ? "mile" : "kilometer",
    unitDisplay: "short",
  }).format(value);
  return t`${text} away`;
}

/** "Example Hall · London", the venue as its line gives it. */
export function venueLine(c: Concert): string {
  const v = c.venue;
  if (!v) return "";
  return [v.name, v.city].filter(Boolean).join(" · ");
}

/** A country's name in the interface's language, from its ISO code; the code when the system lacks it. */
export function countryName(code: string): string {
  const upper = code.toUpperCase();
  try {
    return new Intl.DisplayNames(formatLocales(), { type: "region" }).of(upper) ?? upper;
  } catch {
    return upper;
  }
}
