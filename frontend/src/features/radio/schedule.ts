import type { Station, StationEvent } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { clockTime, dateFormat, numberFormat } from "@/shared/lib/format";

/** The programme on air at `now`, if the schedule covers it. */
export function onAir(schedule: readonly StationEvent[], now: number): StationEvent | undefined {
  return schedule.find((e) => e.startMs <= now && now < e.endMs);
}

/** A stretch of air time: a slot, or a run of them. */
type Span = Pick<StationEvent, "startMs" | "endMs">;

/** How far through its slot programme `e` is at `now`, 0..1. */
export function progressOf(e: Span, now: number): number {
  const span = e.endMs - e.startMs;
  if (span <= 0) return 0;
  return Math.min(1, Math.max(0, (now - e.startMs) / span));
}

/** The weekday before a time, when it falls on another day than `now`: "Tue ". */
function dayPrefix(t: number, now: number): string {
  const day = new Date(t);
  return day.toDateString() === new Date(now).toDateString()
    ? ""
    : `${dateFormat({ weekday: "short" }).format(day)} `;
}

/** "10:00–12:00", with the weekday when the slot starts on another day than `now`. */
export function slotLabel(e: Span, now: number): string {
  return `${dayPrefix(e.startMs, now)}${clockTime(e.startMs)}–${clockTime(e.endMs)}`;
}

/** When a programme starts: "12:00", or "Tue 09:00" on another day than `now`. */
export function startLabel(e: StationEvent, now: number): string {
  return `${dayPrefix(e.startMs, now)}${clockTime(e.startMs)}`;
}

const same = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

/**
 * What a programme is called beside its station's name: its title, or for
 * a slot named after the station itself ("Apple Music Hits" on Apple Music
 * Hits) its description; "" when that only repeats the station's tagline.
 */
export function programmeName(station: Pick<Station, "name" | "tagline">, e: StationEvent): string {
  if (!same(e.title, station.name)) return e.title;
  return e.description && !same(e.description, station.tagline) ? e.description : "";
}

/**
 * The next programme with a name of its own: the first slot after the one
 * on air that is called neither after the station nor like the programme
 * on air, so a run of the station's own hours is skipped.
 */
export function nextProgramme(
  station: Pick<Station, "name">,
  schedule: readonly StationEvent[],
  now: number,
): StationEvent | undefined {
  const current = onAir(schedule, now);
  const from = current ? current.endMs : now;
  return schedule.find(
    (e) =>
      e.startMs >= from &&
      !same(e.title, station.name) &&
      !(current && same(e.title, current.title)),
  );
}

/**
 * The stretch the programme on air holds: its slot together with the
 * unbroken run of slots with the same title around it, so the station's
 * own hours ("Apple Music Hits", 08:00–10:00, 10:00–12:00) count down to
 * the next show with a name of its own, not to an hour nobody named.
 * Undefined when nothing is on air, or when the station's own hours run to
 * the end of the schedule (there is nothing ahead to count down to).
 */
export function onAirSpan(
  station: Pick<Station, "name">,
  schedule: readonly StationEvent[],
  now: number,
): Span | undefined {
  const i = schedule.findIndex((e) => e.startMs <= now && now < e.endMs);
  if (i < 0) return undefined;
  const title = schedule[i].title;
  let first = i;
  let last = i;
  while (
    first > 0 &&
    same(schedule[first - 1].title, title) &&
    schedule[first - 1].endMs === schedule[first].startMs
  )
    first--;
  while (
    last < schedule.length - 1 &&
    same(schedule[last + 1].title, title) &&
    schedule[last + 1].startMs === schedule[last].endMs
  )
    last++;
  if (last === schedule.length - 1 && same(title, station.name)) return undefined;
  return { startMs: schedule[first].startMs, endMs: schedule[last].endMs };
}

/** The station's catalogue line: what it is, who runs it, and an episode's date and length. */
export function stationLine(s: Station): string {
  const parts: string[] = [];
  switch (s.kind) {
    case "live":
      parts.push(t`Live radio`);
      break;
    case "broadcast":
      parts.push(t`Live broadcast`);
      break;
    case "episode":
      parts.push(t`Episode`);
      break;
    default:
      parts.push(t`Station`);
  }
  if (s.kind === "episode" && s.showName) parts.push(s.showName);
  else if (s.provider) parts.push(s.provider);
  if (s.airTimeMs > 0) {
    parts.push(dateFormat({ day: "numeric", month: "short", year: "numeric" }).format(s.airTimeMs));
  }
  if (s.durationMs > 0) {
    parts.push(
      numberFormat({ style: "unit", unit: "minute", unitDisplay: "short" }).format(
        Math.round(s.durationMs / 60000),
      ),
    );
  }
  return parts.join(" · ");
}
