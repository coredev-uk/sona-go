import { describe, expect, it } from "vitest";

import { motionAllowed } from "./policy";

describe("motionAllowed", () => {
  const on = { enabled: true, reduceEffects: false, reducedMotion: false, onBattery: false };

  it("plays when the setting is on and nothing argues against it", () => {
    expect(motionAllowed(on)).toBe(true);
  });

  it("stops for the setting, reduced effects or motion, and battery", () => {
    expect(motionAllowed({ ...on, enabled: false })).toBe(false);
    expect(motionAllowed({ ...on, reduceEffects: true })).toBe(false);
    expect(motionAllowed({ ...on, reducedMotion: true })).toBe(false);
    expect(motionAllowed({ ...on, onBattery: true })).toBe(false);
  });
});
