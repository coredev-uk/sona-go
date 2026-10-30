import type { PlaylistDetail } from "@bindings/services";

/** A playlist page with the track at `from` moved to `to` (what the user sees at once). */
export function withMove(d: PlaylistDetail, from: number, to: number): PlaylistDetail {
  if (from === to || from < 0 || to < 0 || from >= d.tracks.length || to >= d.tracks.length)
    return d;
  const tracks = [...d.tracks];
  const [moved] = tracks.splice(from, 1);
  tracks.splice(to, 0, moved);
  return { ...d, tracks };
}

/** A playlist page without the tracks at `positions`. */
export function withRemoved(d: PlaylistDetail, positions: readonly number[]): PlaylistDetail {
  const drop = new Set(positions);
  const tracks = d.tracks.filter((_, i) => !drop.has(i));
  const durationMs = tracks.reduce((sum, t) => sum + t.durationMs, 0);
  return {
    ...d,
    tracks,
    durationMs,
    playlist: { ...d.playlist, trackCount: tracks.length },
  };
}

const queues = new Map<string, Promise<unknown>>();
const pending = new Map<string, number>();

/**
 * Runs edits of one playlist one after another, in the order they were
 * made: each edit names positions in the state the previous one left, so
 * Go must apply them in that order.
 */
export function serial<T>(playlistId: string, edit: () => Promise<T>): Promise<T> {
  pending.set(playlistId, (pending.get(playlistId) ?? 0) + 1);
  const done = () => {
    const n = (pending.get(playlistId) ?? 1) - 1;
    if (n > 0) pending.set(playlistId, n);
    else pending.delete(playlistId);
  };
  // The count drops before the caller hears the result, so its handler
  // sees whether later edits are still queued.
  const step = () => edit().finally(done);
  const prev = queues.get(playlistId) ?? Promise.resolve();
  const next = prev.then(step, step);
  const settled = next.catch(() => {});
  queues.set(playlistId, settled);
  void settled.then(() => {
    if (queues.get(playlistId) === settled) queues.delete(playlistId);
  });
  return next;
}

/** Whether an edit of the playlist is queued or on its way to Apple. */
export function editing(playlistId: string): boolean {
  return pending.has(playlistId);
}

/** Where a dragged track lands when dropped into gap `slot` (0…n) of the list. */
export function dropIndex(from: number, slot: number): number {
  return slot > from ? slot - 1 : slot;
}
