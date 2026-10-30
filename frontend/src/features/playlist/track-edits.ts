import { PlaylistsService, type PlaylistDetail, type Song } from "@bindings/services";
import { t } from "@lingui/core/macro";
import type { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { callFailure } from "@/shared/lib/errors";

import { editing, serial, withMove, withRemoved } from "./edit";
import { playlistKeys } from "./queries";

/**
 * An optimistic edit of a library playlist's tracks: shown at once, sent
 * in order with the playlist's other edits, then replaced by Go's answer
 * (or rolled back to what SQLite holds when it fails).
 */
function editTracks(
  qc: QueryClient,
  id: string,
  apply: (d: PlaylistDetail) => PlaylistDetail,
  call: () => Promise<PlaylistDetail>,
  what: string,
) {
  const key = playlistKeys.detail(id);
  const prev = qc.getQueryData<PlaylistDetail>(key);
  if (prev) qc.setQueryData(key, apply(prev));
  serial(id, call).then(
    (d) => {
      // Later edits are still on their way: their optimistic state stays.
      if (!editing(id)) qc.setQueryData(key, d);
    },
    (err) => {
      const f = callFailure(err);
      if (f.code === "conflict") {
        toast(t`This playlist changed on another device`, {
          description: t`Here is its latest version. Make the change again.`,
        });
      } else {
        toast.error(what, { description: f.message });
      }
      // SQLite holds Apple's current order either way.
      void qc.invalidateQueries({ queryKey: key });
    },
  );
}

/** Moves the track at `from` (the song the user saw there) to `to`. */
export function moveTrack(qc: QueryClient, id: string, from: number, to: number, song: Song) {
  editTracks(
    qc,
    id,
    (d) => withMove(d, from, to),
    () => PlaylistsService.MoveTrack(id, from, to, song.id),
    t`Couldn't move the song`,
  );
}

/** Removes the tracks at `indices` (those copies only). */
export function removeTracks(
  qc: QueryClient,
  id: string,
  indices: readonly number[],
  songs: readonly Song[],
) {
  editTracks(
    qc,
    id,
    (d) => withRemoved(d, indices),
    () =>
      PlaylistsService.RemoveTracks(
        id,
        [...indices],
        songs.map((s) => s.id),
      ),
    removeFailure(songs),
  );
}

function removeFailure(songs: readonly Song[]): string {
  if (songs.length !== 1) return t`Couldn't remove the songs`;
  const name = songs[0].name;
  return t`Couldn't remove “${name}”`;
}

let client: QueryClient | null = null;

/** Gives row actions (which run outside React) the app's query client. */
export function bindQueryClient(qc: QueryClient) {
  client = qc;
}

export function boundQueryClient(): QueryClient | null {
  return client;
}
