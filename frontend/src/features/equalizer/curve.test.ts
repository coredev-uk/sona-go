import { Equalizer } from "@bindings/config";
import { EqualizerPreset } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { CUSTOM, formatDb, formatHz, gainsOf, pregainOf, withBand } from "./curve";

const presets = [
  new EqualizerPreset({ id: "flat", name: "Flat", gains: [0, 0, 0] }),
  new EqualizerPreset({ id: "bass", name: "Bass Boost", gains: [6, 3, 0] }),
];

describe("withBand", () => {
  it("starts the user's curve from the selected preset", () => {
    const eq = new Equalizer({ enabled: true, preset: "bass", custom: [-1, -1, -1], preamp: 0 });
    const next = withBand(eq, presets, 2, 4);
    expect(next.preset).toBe(CUSTOM);
    expect(next.custom).toEqual([6, 3, 4]);
    expect(presets[1].gains).toEqual([6, 3, 0]);
  });

  it("edits the user's own curve in place", () => {
    const eq = new Equalizer({ enabled: true, preset: CUSTOM, custom: [1, 2, 3], preamp: 0 });
    expect(withBand(eq, presets, 0, -5).custom).toEqual([-5, 2, 3]);
    expect(gainsOf(eq, presets)).toEqual([1, 2, 3]);
  });
});

describe("pregainOf", () => {
  it("reserves headroom for the largest boost plus the preamp trim", () => {
    const eq = new Equalizer({ enabled: true, preset: "bass", custom: [], preamp: -2 });
    expect(pregainOf(eq, gainsOf(eq, presets))).toBe(-8);
  });

  it("is zero when the equaliser is off or only cuts", () => {
    const eq = new Equalizer({ enabled: false, preset: "bass", custom: [], preamp: -2 });
    expect(pregainOf(eq, gainsOf(eq, presets))).toBe(0);
    expect(pregainOf({ ...eq, enabled: true, preamp: 0 }, [-3, -6])).toBe(0);
  });
});

describe("labels", () => {
  it("formats gains and frequencies compactly", () => {
    expect([formatDb(0), formatDb(3), formatDb(-4.5), formatDb(0.2)]).toEqual([
      "0",
      "+3",
      "−4.5",
      "0",
    ]);
    expect([29, 947, 1889, 3770, 15011].map(formatHz)).toEqual([
      "29",
      "947",
      "1.9k",
      "3.8k",
      "15k",
    ]);
  });
});
