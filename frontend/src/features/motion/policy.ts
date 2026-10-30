import { create } from "zustand";

/** What decides whether motion covers play. */
export interface MotionInputs {
  /** Settings → Appearance → Motion artwork. */
  enabled: boolean;
  /** Sona's Reduce effects. */
  reduceEffects: boolean;
  /** The system's prefers-reduced-motion. */
  reducedMotion: boolean;
  /** UPower: the machine runs on battery. */
  onBattery: boolean;
}

/** Motion covers play only when asked for and nothing argues against them. */
export function motionAllowed(i: MotionInputs): boolean {
  return i.enabled && !i.reduceEffects && !i.reducedMotion && !i.onBattery;
}

/** Go's power state mirror ("power:changed", MotionService.Power). */
export const usePower = create<{ onBattery: boolean }>(() => ({ onBattery: false }));
