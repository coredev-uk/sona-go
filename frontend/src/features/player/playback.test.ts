import { describe, expect, it } from "vitest";

import type { PlaybackState } from "@bindings/services";

import { emptyState, livePosition } from "./playback";

const base: PlaybackState = {
  ...emptyState,
  state: "playing",
  positionMs: 90_000,
  durationMs: 318_000,
  at: 1_000,
};

describe("livePosition", () => {
  it("extrapolates from the snapshot time while playing", () => {
    expect(livePosition(base, 1_000)).toBe(90_000);
    expect(livePosition(base, 3_500)).toBe(92_500);
  });

  it("clamps at the duration", () => {
    expect(livePosition(base, 1_000_000)).toBe(318_000);
  });

  it("never rewinds on clock skew", () => {
    expect(livePosition(base, 500)).toBe(90_000);
  });

  it("does not clamp when the duration is unknown", () => {
    expect(livePosition({ ...base, durationMs: 0 }, 11_000)).toBe(100_000);
  });

  it("holds the reported position when not playing", () => {
    for (const state of ["paused", "loading", "idle", "ended", "error"]) {
      expect(livePosition({ ...base, state }, 60_000)).toBe(90_000);
    }
  });
});
