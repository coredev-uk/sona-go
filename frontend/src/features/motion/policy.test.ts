import type { PowerState } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { motionAt } from "@/shared/lib/motion";

import { motionCodec, motionState } from "./policy";

const mains: PowerState = {
  followed: true,
  onBattery: false,
  source: "mains",
  upower: false,
  ignored: 0,
  unreadable: false,
};

describe("motionState", () => {
  const on = {
    setting: true,
    reduceEffects: false,
    reducedMotion: false,
    power: mains,
    h264: "probably",
    hevc: "",
  };

  it("plays when the setting is on and nothing argues against it", () => {
    expect(motionState(on)).toMatchObject({ allowed: true, codec: "avc", probed: true });
    // Before Go answers, the computer counts as on mains power.
    expect(motionState({ ...on, power: null }).allowed).toBe(true);
  });

  it("stops for the setting, reduced effects or motion, battery, and no decoder", () => {
    expect(motionState({ ...on, setting: false }).allowed).toBe(false);
    expect(motionState({ ...on, reduceEffects: true }).allowed).toBe(false);
    expect(motionState({ ...on, reducedMotion: true }).allowed).toBe(false);
    expect(motionState({ ...on, power: { ...mains, onBattery: true } }).allowed).toBe(false);
    expect(motionState({ ...on, h264: "", hevc: "" })).toMatchObject({ allowed: false, codec: "" });
  });
});

describe("motionCodec", () => {
  it("prefers H.264 and falls back to HEVC only without it", () => {
    expect(motionCodec("probably", "probably")).toBe("avc");
    expect(motionCodec("maybe", "")).toBe("avc");
    expect(motionCodec("", "probably")).toBe("hevc");
    expect(motionCodec("", "")).toBe("");
  });
});

describe("motionAt", () => {
  const url = "/motion/0123456789abcdef01234567/720.mp4";

  it("asks for the drawn size at the screen's density, at most 2×", () => {
    window.devicePixelRatio = 1;
    expect(motionAt(url, 200)).toBe("/motion/0123456789abcdef01234567/200.mp4");
    window.devicePixelRatio = 3;
    expect(motionAt(url, 200)).toBe("/motion/0123456789abcdef01234567/400.mp4");
  });

  it("adds the codec only for HEVC and replaces an earlier one", () => {
    window.devicePixelRatio = 2;
    expect(motionAt(url, 180, "hevc")).toBe("/motion/0123456789abcdef01234567/360.mp4?codec=hevc");
    expect(motionAt(`${url}?codec=hevc`, 180)).toBe("/motion/0123456789abcdef01234567/360.mp4");
  });
});
