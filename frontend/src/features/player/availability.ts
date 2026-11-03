import type { PlaybackControls, PlaybackState } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { announceUnavailable } from "@/shared/lib/unavailable";

import { usePlayback } from "./playback";

/** A control Go's snapshot says applies or not (`PlaybackState.controls`). */
export type Control = keyof PlaybackControls;

/** Why a live station has no such control; the text of every surface that shows it. */
export function liveStationReason(): string {
  return t`Not available while listening to a live station`;
}

/** Why a station of songs has no such control (shuffle, repeat, AutoPlay, queueing). */
export function stationReason(): string {
  return t`Not available while listening to a station`;
}

/**
 * Why `control` doesn't apply to what plays, or undefined when it does.
 * Go decides (`controls`); the reason is only worded here, and only for a
 * live station or a station of songs (one the user started, not
 * AutoPlay's): Next at the end of a queue is simply off.
 */
export function unavailableReason(
  s: Pick<PlaybackState, "current" | "controls" | "station">,
  control: Control,
): string | undefined {
  if (s.controls[control]) return undefined;
  if (s.current?.live) return liveStationReason();
  return s.station && !s.station.autoPlay ? stationReason() : undefined;
}

/** The reason now, for a shortcut's `unavailable`. */
export function unavailableNow(control: Control): string | undefined {
  return unavailableReason(usePlayback.getState(), control);
}

/** The reason for a component: re-renders when it comes or goes. */
export function useUnavailable(control: Control): string | undefined {
  return usePlayback((s) => unavailableReason(s, control));
}

/**
 * Stops a command (a key, a menu entry) that doesn't apply: says why in the
 * notifications region (one at a time, however fast the key repeats) and
 * returns true.
 */
export function refuse(control: Control): boolean {
  const reason = unavailableNow(control);
  if (!reason) return false;
  announceUnavailable(reason);
  return true;
}
