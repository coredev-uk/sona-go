import { PlaylistsService, type PlaylistDetail, type Song } from "@bindings/services";
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
        toast("This playlist changed on another device", {
          description: "Here is its latest version. Make the change again.",
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
    "Couldn't move the song",
  );
}

/** Removes the track at `index` (that copy only). */
export function removeTrack(qc: QueryClient, id: string, index: number, song: Song) {
  editTracks(
    qc,
    id,
    (d) => withRemoved(d, [index]),
    () => PlaylistsService.RemoveTracks(id, [index], [song.id]),
    `Couldn't remove “${song.name}”`,
  );
}

let client: QueryClient | null = null;

/** Gives row actions (which run outside React) the app's query client. */
export function bindQueryClient(qc: QueryClient) {
  client = qc;
}

export function boundQueryClient(): QueryClient | null {
  return client;
}
