import type { PowerState } from "@bindings/services";
import { create } from "zustand";

import type { MotionCodec, MotionState } from "@/shared/lib/motion";

/** Apple's renditions as canPlayType sees them (H.264 High 3.1, HEVC Main). */
export const H264_TYPE = 'video/mp4; codecs="avc1.64001f"';
export const HEVC_TYPE = 'video/mp4; codecs="hvc1.2.20000000.L123.B0"';

/** What decides whether motion covers play. */
export interface MotionInputs {
  /** Settings → Appearance → Motion artwork. */
  setting: boolean;
  /** Sona's Reduce effects. */
  reduceEffects: boolean;
  /** The system's prefers-reduced-motion. */
  reducedMotion: boolean;
  /** Go's power source (null until Go answers: counted as mains power). */
  power: PowerState | null;
  /** canPlayType answers for H264_TYPE and HEVC_TYPE. */
  h264: string;
  hevc: string;
}

/**
 * The codec motion renditions are asked for: H.264 wherever WebKit plays
 * it (Apple always has it; cheapest to decode), HEVC only where WebKit
 * plays HEVC but not H.264, none when it plays neither.
 */
export function motionCodec(h264: string, hevc: string): MotionCodec {
  if (h264) return "avc";
  return hevc ? "hevc" : "";
}

/** Motion covers play only when asked for and nothing argues against them. */
export function motionState(i: MotionInputs): MotionState {
  const codec = motionCodec(i.h264, i.hevc);
  return {
    ...i,
    probed: true,
    codec,
    allowed:
      i.setting && !i.reduceEffects && !i.reducedMotion && !i.power?.onBattery && codec !== "",
  };
}

/** Go's power source mirror ("power:changed", MotionService.Power). */
export const usePower = create<{ power: PowerState | null }>(() => ({ power: null }));
