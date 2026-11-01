import type { MotionStatus, PowerState } from "@bindings/services";
import { describe, expect, it } from "vitest";

import type { MotionState } from "@/shared/lib/motion";

import { motionGroup, motionReport } from "./MotionDiagnostics";

const mains: PowerState = {
  followed: true,
  onBattery: false,
  source: "mains",
  upower: false,
  ignored: 0,
  error: "",
};

const playing: MotionState = {
  probed: true,
  allowed: true,
  setting: true,
  reduceEffects: false,
  reducedMotion: false,
  power: mains,
  h264: "probably",
  hevc: "probably",
  codec: "avc",
};

const status: MotionStatus = {
  power: mains,
  cacheFiles: 2,
  cacheBytes: 2048,
  last: { at: 0, size: 0, codec: "", rendition: "", error: "" },
};

const row = (s: MotionState, label: string, st: MotionStatus | undefined = status) =>
  motionGroup(s, null, st, true)?.rows.find((r) => r.label === label);

describe("motionGroup", () => {
  it("is absent without the motion module", () => {
    expect(motionGroup({ ...playing, probed: false }, null, status, true)).toBeNull();
  });

  it("names the first input that stops motion", () => {
    expect(row(playing, "Motion artwork")).toMatchObject({ status: "ok" });
    expect(row({ ...playing, allowed: false, setting: false }, "Motion artwork")?.value).toMatch(
      /Settings/,
    );
    expect(
      row({ ...playing, allowed: false, power: { ...mains, onBattery: true } }, "Motion artwork")
        ?.value,
    ).toMatch(/battery/);
  });

  it("flags a missing H.264 decoder as a problem, with the packages to install", () => {
    const none = { ...playing, allowed: false, h264: "", hevc: "", codec: "" as const };
    expect(row(none, "H.264 decoding")).toMatchObject({ status: "error" });
    expect(row(none, "H.264 decoding")?.detail).toMatch(/gst-libav/);
    expect(row(none, "Motion artwork")).toMatchObject({ status: "error" });
  });

  it("explains a desktop UPower calls on battery, and ignored peripherals", () => {
    const p = row({ ...playing, power: { ...mains, upower: true, ignored: 2 } }, "Power");
    expect(p).toMatchObject({ status: "ok" });
    expect(p?.detail).toMatch(/2 peripheral batteries/);
    expect(p?.detail).toMatch(/UPower itself reports this computer on battery/);
  });

  it("shows Go's last download failure", () => {
    const failed = {
      ...status,
      last: { at: Date.now(), size: 480, codec: "avc", rendition: "", error: "no H.264 rendition" },
    };
    expect(row(playing, "Last download", failed)).toMatchObject({
      status: "warning",
      detail: "no H.264 rendition",
    });
  });
});

describe("motionReport", () => {
  it("carries the web view's side for the bundle", () => {
    const r = motionReport(
      playing,
      { id: 1, src: "/motion/k/400.mp4", at: 0, outcome: "media-error", detail: "MediaError 4" },
      1,
      true,
    );
    expect(r).toMatchObject({ allowed: true, h264: "probably", playing: 1, visible: true });
    expect(r.last).toBe("media-error 1970-01-01T00:00:00.000Z /motion/k/400.mp4 MediaError 4");
  });
});
