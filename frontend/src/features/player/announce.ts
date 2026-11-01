import type { PlaybackState } from "@bindings/services";

type Snapshot = Pick<PlaybackState, "state" | "current">;

/**
 * The track a track change should announce, or null. `seen` is the id of
 * the last track announced. Only a different track that is playing counts:
 * not while it loads, because what the snapshot says about the track (that
 * it plays as a preview) is only sure once the engine has reported on it.
 * The track restored paused at launch stays quiet until it plays, and
 * play/pause, seeking and repeats of the same track stay quiet.
 */
export function announcedTrack(
  seen: string | undefined,
  next: Snapshot,
): NonNullable<Snapshot["current"]> | null {
  const c = next.current;
  if (!c || c.id === seen) return null;
  if (next.state !== "playing") return null;
  return c;
}
