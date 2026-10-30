import type { HistoryEntry, ListeningHistory, Song } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { dayLabel, historyGroups } from "./history";

const play = (id: string, playedAt: number): HistoryEntry =>
  ({ playedAt, playedMs: 0, context: "", song: { id } as Song }) as HistoryEntry;

describe("historyGroups", () => {
  it("folds a day's repeats into the latest play and counts plays from other devices", () => {
    const h = {
      days: [
        { day: 0, entries: [play("x", 0)] },
        { day: 1000, entries: [play("a", 30), play("b", 0), play("a", 20), play("c", 10)] },
        { day: 500, entries: [play("a", 5)] },
      ],
      since: 0,
      more: false,
      apple: true,
    } as ListeningHistory;
    const groups = historyGroups(h);
    expect(groups.map((g) => g.songs.map((s) => s.id))).toEqual([["x"], ["a", "b", "c"], ["a"]]);
    expect(groups.map((g) => [g.plays, g.elsewhere])).toEqual([
      [1, 1],
      [4, 1],
      [1, 0],
    ]);
  });
});

describe("dayLabel", () => {
  const now = new Date(2026, 9, 30, 14, 30).getTime(); // Friday 30 October 2026
  const day = (y: number, m: number, d: number) => new Date(y, m, d).getTime();

  it("names recent days and dates older ones", () => {
    expect(dayLabel(day(2026, 9, 30), now)).toBe("Today");
    expect(dayLabel(day(2026, 9, 29), now)).toBe("Yesterday");
    expect(dayLabel(day(2026, 9, 26), now)).toBe("Monday");
    expect(dayLabel(day(2026, 9, 23), now)).toBe("Friday, October 23");
    expect(dayLabel(day(2025, 11, 31), now)).toBe("Wednesday, December 31, 2025");
  });

  it("labels the undated plays from other devices", () => {
    expect(dayLabel(0, now)).toBe("Recently, on your other devices");
  });
});
