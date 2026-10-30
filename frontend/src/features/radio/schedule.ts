import type { Station, StationEvent } from "@bindings/services";

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

const clock = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const day = new Intl.DateTimeFormat(undefined, { weekday: "short" });

/** "10:00–12:00", with the weekday when the slot starts on another day than `now`. */
export function slotLabel(e: StationEvent, now: number): string {
  const range = `${clock.format(e.startMs)}–${clock.format(e.endMs)}`;
  const start = new Date(e.startMs);
  const today = new Date(now);
  return start.toDateString() === today.toDateString() ? range : `${day.format(start)} ${range}`;
}

/** The station's catalogue line: what it is, who runs it, and an episode's date and length. */
export function stationLine(s: Station): string {
  const parts: string[] = [];
  switch (s.kind) {
    case "live":
      parts.push("Live radio");
      break;
    case "broadcast":
      parts.push("Live broadcast");
      break;
    case "episode":
      parts.push("Episode");
      break;
    default:
      parts.push("Station");
  }
  if (s.kind === "episode" && s.showName) parts.push(s.showName);
  else if (s.provider) parts.push(s.provider);
  if (s.airTimeMs > 0) {
    parts.push(
      new Date(s.airTimeMs).toLocaleDateString(undefined, {
        day: "numeric",
        month: "short",
        year: "numeric",
      }),
    );
  }
  if (s.durationMs > 0) parts.push(`${Math.round(s.durationMs / 60000)} min`);
  return parts.join(" · ");
}
