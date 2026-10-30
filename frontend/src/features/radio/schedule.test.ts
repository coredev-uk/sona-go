import type { Station, StationEvent } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { onAir, progressOf, slotLabel, stationLine } from "./schedule";

const ev = (id: string, startMs: number, endMs: number) =>
  ({
    id,
    title: id,
    description: "",
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
    expect(slotLabel(ev("c", t0 + 24 * h, t0 + 25 * h), t0).split(" ").length).toBe(2);
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
