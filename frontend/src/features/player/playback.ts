import { useEffect, useState } from "react";
import { create } from "zustand";

import type { PlaybackState, QueueState } from "@bindings/services";

/** Engine states, mirroring Go's playback.State values. */
export type EngineState = "idle" | "loading" | "playing" | "paused" | "ended" | "error";
export type Repeat = "off" | "all" | "one";

export const emptyState: PlaybackState = {
  state: "idle",
  current: null,
  index: -1,
  positionMs: 0,
  durationMs: 0,
  at: 0,
  volume: 0.8,
  shuffle: false,
  repeat: "off",
  context: "",
  queueLength: 0,
  queueVersion: 0,
  canNext: false,
  canPrevious: false,
  error: "",
  authRequired: false,
  noCdm: false,
  preview: false,
  onAir: null,
  autoPlay: false,
  station: null,
  container: null,
  sleep: null,
  canUndoReplace: false,
};

interface PlaybackStore extends PlaybackState {
  queue: QueueState;
  /** Replaces the mirror with a snapshot from Go (playback:state, latest wins). */
  apply: (state: PlaybackState) => void;
  /** Replaces the queue (playback:queue, sent when its version changes). */
  applyQueue: (queue: QueueState) => void;
  /** Optimistic local change while a command is in flight (seek, volume). */
  patch: (p: Partial<PlaybackState>) => void;
}

/** `prev` when `next` holds the same value: Go's snapshots arrive as new objects. */
function keep<T>(prev: T, next: T): T {
  return prev === next || JSON.stringify(prev) === JSON.stringify(next) ? prev : next;
}

/**
 * Frontend mirror of the Go playback coordinator. Go owns the queue and the
 * engine and sends a snapshot only when something changes, so the UI
 * derives the live position locally with `livePosition` instead of polling.
 * A snapshot keeps the previous `current` and `station` objects when they
 * are unchanged, so a play/pause or seek does not re-render every component
 * (and track list) that selects them.
 */
export const usePlayback = create<PlaybackStore>((set) => ({
  ...emptyState,
  queue: { version: 0, index: -1, items: [], history: [], station: null },
  apply: (state) =>
    set((s) => ({
      ...state,
      current: keep(s.current, state.current),
      station: keep(s.station, state.station),
      sleep: keep(s.sleep, state.sleep),
    })),
  applyQueue: (queue) => set({ queue }),
  patch: (p) => set(p),
}));

/** Position in ms at `now`, extrapolated while playing and clamped to the duration. */
export function livePosition(
  s: Pick<PlaybackState, "state" | "positionMs" | "durationMs" | "at">,
  now: number,
): number {
  if (s.state !== "playing") return s.positionMs;
  const pos = s.positionMs + Math.max(0, now - s.at);
  return s.durationMs > 0 ? Math.min(pos, s.durationMs) : pos;
}

/**
 * The live position, re-rendering the caller every `intervalMs` while
 * playing. Only small leaf components (seek bar, lyrics) should use it.
 */
export function useLivePosition(intervalMs = 250): number {
  const state = usePlayback((s) => s.state);
  const positionMs = usePlayback((s) => s.positionMs);
  const durationMs = usePlayback((s) => s.durationMs);
  const at = usePlayback((s) => s.at);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (state !== "playing") return;
    const id = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(id);
  }, [state, intervalMs]);
  // A snapshot newer than the last tick starts from its own position.
  return livePosition({ state, positionMs, durationMs, at }, Math.max(now, at));
}
