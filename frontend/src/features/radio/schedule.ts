import type { Station, StationEvent } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { clockTime, dateFormat, numberFormat } from "@/shared/lib/format";

/** The programme on air at `now`, if the schedule covers it. */
export function onAir(schedule: readonly StationEvent[], now: number): StationEvent | undefined {
  return schedule.find((e) => e.startMs <= now && now < e.endMs);
}

/** How far through its slot programme `e` is at `now`, 0..1. */
export function progressOf(e: StationEvent, now: number): number {
  const span = e.endMs - e.startMs;
  if (span <= 0) return 0;
  return Math.min(1, Math.max(0, (now - e.startMs) / span));
}

/** "10:00–12:00", with the weekday when the slot starts on another day than `now`. */
export function slotLabel(e: StationEvent, now: number): string {
  const range = `${clockTime(e.startMs)}–${clockTime(e.endMs)}`;
  const start = new Date(e.startMs);
  const today = new Date(now);
  return start.toDateString() === today.toDateString()
    ? range
    : `${dateFormat({ weekday: "short" }).format(start)} ${range}`;
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
