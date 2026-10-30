import { PlaylistsService, type PlaylistItem, type Song } from "@bindings/services";
import { toast } from "sonner";

import { playSongs, shuffleSongs } from "@/features/player";
import { confirm } from "@/shared/lib/confirm";
import { callFailure } from "@/shared/lib/errors";
import { run } from "@/shared/lib/run";

import { showDialog } from "./dialogs";

/** Opens the picker for adding items (`label` names them: “Hundred”, “Parachutes”). */
export function addToPlaylist(items: PlaylistItem[], label: string) {
  showDialog({ kind: "add", items, label });
}

/** Asks, then deletes a library playlist; resolves true once it is gone. */
export async function deletePlaylist(id: string, name: string): Promise<boolean> {
  const ok = await confirm({
    title: `Delete “${name}”?`,
    body: "It's removed from your library on all your devices. The songs in it stay in your library.",
    confirmLabel: "Delete playlist",
    destructive: true,
  });
  if (!ok) return false;
  const done = await run(`Couldn't delete “${name}”`, PlaylistsService.Delete(id));
  if (done) toast(`Deleted “${name}”`);
  return done;
}

/** Asks, then deletes an empty folder. */
export async function deleteFolder(id: string, name: string): Promise<void> {
  const ok = await confirm({
    title: `Delete the folder “${name}”?`,
    body: "The empty folder is removed on all your devices.",
    confirmLabel: "Delete folder",
    destructive: true,
  });
  if (!ok) return;
  if (await run(`Couldn't delete the folder “${name}”`, PlaylistsService.DeleteFolder(id))) {
    toast(`Deleted the folder “${name}”`);
  }
}

/** Pins a library item to the sidebar (Apple keeps six). */
export async function pin(id: string, name: string) {
  try {
    await PlaylistsService.Pin(id);
    toast(`Pinned “${name}”`);
  } catch (err) {
    const f = callFailure(err);
    toast.error(`Couldn't pin “${name}”`, {
      description:
        f.code === "pins_full" ? "You have six pins. Unpin one to make room." : f.message,
    });
  }
}

export function unpin(id: string, name: string) {
  return run(`Couldn't unpin “${name}”`, PlaylistsService.Unpin(id));
}

/** Plays (or shuffles) what a pin holds, from the local library. */
export async function playPin(id: string, name: string, shuffle: boolean) {
  let songs: Song[];
  try {
    songs = await PlaylistsService.PinTracks(id);
  } catch (err) {
    toast.error(`Couldn't play “${name}”`, { description: callFailure(err).message });
    return;
  }
  if (!songs.length) {
    toast(`“${name}” has no songs to play`);
    return;
  }
  if (shuffle) void shuffleSongs(songs, id);
  else playSongs(songs, 0, id);
}
