import type { Station, StationEvent } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { dateFormat } from "@/shared/lib/format";

import {
  nextProgramme,
  onAirSpan,
  onAir,
  programmeName,
  progressOf,
  slotLabel,
  startLabel,
  stationLine,
} from "./schedule";

const ev = (id: string, startMs: number, endMs: number, description = "") =>
  ({
    id,
    title: id,
    description,
    startMs,
    endMs,
    art: { small: "", medium: "", large: "" },
  }) as StationEvent;

describe("schedule", () => {
  const h = 3600_000;
  const t0 = new Date(2026, 9, 30, 10, 0).getTime();
  const sched = [ev("a", t0, t0 + h), ev("b", t0 + h, t0 + 3 * h)];

  it("finds the programme on air", () => {
    expect(onAir(sched, t0 + 10)?.id).toBe("a");
    expect(onAir(sched, t0 + h)?.id).toBe("b");
    expect(onAir(sched, t0 + 3 * h)).toBeUndefined();
    expect(onAir([], t0)).toBeUndefined();
  });

  it("measures progress through a slot", () => {
    expect(progressOf(sched[1], t0 + 2 * h)).toBe(0.5);
    expect(progressOf(sched[0], t0 - 1)).toBe(0);
    expect(progressOf(sched[0], t0 + 5 * h)).toBe(1);
    expect(progressOf(ev("z", t0, t0), t0)).toBe(0);
  });

  it("adds the weekday only for another day", () => {
    expect(slotLabel(sched[0], t0)).not.toMatch(/^\D{2,}\s/);
    const tomorrow = ev("c", t0 + 24 * h, t0 + 25 * h);
    const weekday = dateFormat({ weekday: "short" }).format(tomorrow.startMs);
    expect(slotLabel(tomorrow, t0)).toBe(`${weekday} ${slotLabel(tomorrow, tomorrow.startMs)}`);
    expect(startLabel(tomorrow, t0)).toBe(`${weekday} ${startLabel(tomorrow, tomorrow.startMs)}`);
    expect(slotLabel(sched[0], t0).startsWith(startLabel(sched[0], t0))).toBe(true);
  });
});

describe("now and next", () => {
  const h = 3600_000;
  const t0 = new Date(2026, 9, 30, 10, 0).getTime();
  const station = { name: "Hits", tagline: "Songs you know." };

  it("names a programme by its title, or by what it says when it is the station's own hour", () => {
    expect(programmeName(station, ev("The Zane Lowe Show", t0, t0 + h))).toBe("The Zane Lowe Show");
    expect(programmeName(station, ev("hits", t0, t0 + h, "Club euphoria with NAINA."))).toBe(
      "Club euphoria with NAINA.",
    );
    expect(programmeName(station, ev("Hits", t0, t0 + h, "Songs you know."))).toBe("");
    expect(programmeName(station, ev("Hits", t0, t0 + h))).toBe("");
  });

  it("finds the next programme with a name of its own", () => {
    const day = [
      ev("Hits", t0, t0 + h),
      ev("Hits", t0 + h, t0 + 2 * h),
      ev("The Matt Wilkinson Show", t0 + 2 * h, t0 + 4 * h),
      ev("The Matt Wilkinson Show", t0 + 4 * h, t0 + 5 * h),
      ev("Rebecca Judd", t0 + 5 * h, t0 + 6 * h),
    ];
    expect(nextProgramme(station, day, t0 + 10)?.startMs).toBe(t0 + 2 * h);
    // The show on air is not its own "next", even when it runs on.
    expect(nextProgramme(station, day, t0 + 3 * h)?.title).toBe("Rebecca Judd");
    expect(nextProgramme(station, day, t0 + 5 * h)).toBeUndefined();
    expect(nextProgramme(station, day.slice(0, 2), t0)).toBeUndefined();
    // Nothing on air (a gap in the schedule): the next named slot from now.
    expect(nextProgramme(station, day.slice(2), t0)?.startMs).toBe(t0 + 2 * h);
  });

  it("counts the station's own hours and a show's run of slots as one stretch", () => {
    const day = [
      ev("Hits", t0, t0 + h),
      ev("Hits", t0 + h, t0 + 2 * h),
      ev("Matt", t0 + 2 * h, t0 + 3 * h),
      ev("Matt", t0 + 3 * h, t0 + 4 * h),
      ev("Hits", t0 + 4 * h, t0 + 5 * h),
    ];
    expect(onAirSpan(station, day, t0 + 10)).toEqual({ startMs: t0, endMs: t0 + 2 * h });
    expect(onAirSpan(station, day, t0 + 3 * h)).toEqual({ startMs: t0 + 2 * h, endMs: t0 + 4 * h });
    // The station's own hours to the end of the schedule: nothing to count down to.
    expect(onAirSpan(station, day, t0 + 4 * h)).toBeUndefined();
    expect(onAirSpan(station, day.slice(0, 2), t0)).toBeUndefined();
    // A break in the run ends it.
    const gap = [ev("Hits", t0, t0 + h), ev("Hits", t0 + 2 * h, t0 + 3 * h), day[2]];
    expect(onAirSpan(station, gap, t0)).toEqual({ startMs: t0, endMs: t0 + h });
    expect(onAirSpan(station, day, t0 - 1)).toBeUndefined();
  });
});

describe("stationLine", () => {
  const base = { provider: "", showName: "", airTimeMs: 0, durationMs: 0 } as Station;
  it("names the kind and owner", () => {
    expect(stationLine({ ...base, kind: "live", provider: "Apple Music" })).toBe(
      "Live radio · Apple Music",
    );
    expect(stationLine({ ...base, kind: "tracks" })).toBe("Station");
    expect(
      stationLine({
        ...base,
        kind: "episode",
        showName: "Zane Lowe",
        provider: "Apple Music",
        durationMs: 3_420_000,
      }),
    ).toBe("Episode · Zane Lowe · 57 min");
  });
});
