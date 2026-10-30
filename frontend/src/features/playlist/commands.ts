import { PlaylistsService, type PlaylistItem, type Song } from "@bindings/services";
import { t } from "@lingui/core/macro";
import type { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { playSongs, shuffleSongs } from "@/features/player";
import type { ActionItem } from "@/shared/actions/registry";
import { confirm } from "@/shared/lib/confirm";
import { callFailure } from "@/shared/lib/errors";
import { run } from "@/shared/lib/run";

import { showDialog } from "./dialogs";
import { playlistKeys, setDetail } from "./queries";

/** Opens the picker for adding items (`label` names them: “Hundred”, “Parachutes”). */
export function addToPlaylist(items: PlaylistItem[], label: string) {
  showDialog({ kind: "add", items, label });
}

/** The songs, albums and playlists among `items`, as a playlist takes them. */
export function playlistItems(items: readonly ActionItem[]): PlaylistItem[] {
  return items.flatMap((i) =>
    i.kind === "song" || i.kind === "album" || i.kind === "playlist"
      ? [{ kind: i.kind, id: i.target.id, source: i.target.source }]
      : [],
  );
}

/**
 * Adds items to a library playlist (the picker's choice, or a drop on the
 * sidebar), with a toast either way; `open` goes to the playlist.
 */
export function addToPlaylistNow(
  qc: QueryClient,
  playlist: { id: string; name: string },
  items: PlaylistItem[],
  label: string,
  open: () => void,
) {
  const name = playlist.name;
  void PlaylistsService.Add(playlist.id, items).then(
    (detail) => {
      setDetail(qc, detail);
      void qc.invalidateQueries({ queryKey: playlistKeys.recentTargets });
      toast(t`Added ${label} to “${name}”`, { action: { label: t`Open`, onClick: open } });
    },
    (err) => toast.error(t`Couldn't add to “${name}”`, { description: callFailure(err).message }),
  );
}

/** Asks, then deletes a library playlist; resolves true once it is gone. */
export async function deletePlaylist(id: string, name: string): Promise<boolean> {
  const ok = await confirm({
    title: t`Delete “${name}”?`,
    body: t`It's removed from your library on all your devices. The songs in it stay in your library.`,
    confirmLabel: t`Delete playlist`,
    destructive: true,
  });
  if (!ok) return false;
  const done = await run(t`Couldn't delete “${name}”`, PlaylistsService.Delete(id));
  if (done) toast(t`Deleted “${name}”`);
  return done;
}

/** Asks, then deletes an empty folder. */
export async function deleteFolder(id: string, name: string): Promise<void> {
  const ok = await confirm({
    title: t`Delete the folder “${name}”?`,
    body: t`The empty folder is removed on all your devices.`,
    confirmLabel: t`Delete folder`,
    destructive: true,
  });
  if (!ok) return;
  if (await run(t`Couldn't delete the folder “${name}”`, PlaylistsService.DeleteFolder(id))) {
    toast(t`Deleted the folder “${name}”`);
  }
}

/** Pins a library item to the sidebar (Apple keeps six). */
export async function pin(id: string, name: string) {
  try {
    await PlaylistsService.Pin(id);
    toast(t`Pinned “${name}”`);
  } catch (err) {
    toast.error(t`Couldn't pin “${name}”`, { description: callFailure(err).message });
  }
}

export function unpin(id: string, name: string) {
  return run(t`Couldn't unpin “${name}”`, PlaylistsService.Unpin(id));
}

/** Plays (or shuffles) what a pin holds, from the local library. */
export async function playPin(id: string, name: string, shuffle: boolean) {
  let songs: Song[];
  try {
    songs = await PlaylistsService.PinTracks(id);
  } catch (err) {
    toast.error(t`Couldn't play “${name}”`, { description: callFailure(err).message });
    return;
  }
  if (!songs.length) {
    toast(t`“${name}” has no songs to play`);
    return;
  }
  // Pins are library playlists ("p.…") or albums ("l.…").
  const from = { kind: id.startsWith("p.") ? "playlist" : id.startsWith("l.") ? "album" : "", id };
  if (shuffle) void shuffleSongs(songs, from);
  else playSongs(songs, 0, from);
}
