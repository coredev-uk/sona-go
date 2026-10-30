import type { ListeningHistory, Song } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { dateFormat } from "@/shared/lib/format";

/** One day of the History page: each song once, at its latest play that day. */
export interface HistoryGroup {
  /** Local midnight (Unix ms); 0 for recent plays on other devices, which Apple gives no time. */
  day: number;
  songs: Song[];
  /** Every play that day, repeats included. */
  plays: number;
  /** Songs Apple reports from the account's other devices. */
  elsewhere: number;
}

/** Go's days as the page lists them: repeats of a song within a day fold into its latest play. */
export function historyGroups(h: ListeningHistory): HistoryGroup[] {
  return h.days.map((d) => {
    const seen = new Set<string>();
    const songs: Song[] = [];
    let elsewhere = 0;
    for (const e of d.entries) {
      if (seen.has(e.song.id)) continue;
      seen.add(e.song.id);
      songs.push(e.song);
      if (!e.playedAt) elsewhere++;
    }
    return { day: d.day, songs, plays: d.entries.length, elsewhere };
  });
}

const DAY = 86_400_000;

/**
 * A day heading: "Today", "Yesterday", the weekday within the last week,
 * then the date ("Monday 26 October"), with the year once it is not this
 * year's, in the active locale. `day` and `now` are local times.
 */
export function dayLabel(day: number, now: number): string {
  if (!day) return t`Recently, on your other devices`;
  const d = new Date(day);
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  // Whole days between the two midnights (DST days are 23 or 25 hours long).
  const ago = Math.round((today.getTime() - d.getTime()) / DAY);
  if (ago === 0) return t`Today`;
  if (ago === 1) return t`Yesterday`;
  if (ago > 1 && ago < 7) return dateFormat({ weekday: "long" }).format(d);
  return dateFormat({
    weekday: "long",
    day: "numeric",
    month: "long",
    year: d.getFullYear() === today.getFullYear() ? undefined : "numeric",
  }).format(d);
}
