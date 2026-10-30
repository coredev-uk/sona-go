import type { Item, Song } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { toast } from "sonner";

import { playSongs } from "@/features/player";
import { tracksOf as collectionTracks } from "@/shared/actions/songs";
import { actionItemOf } from "@/shared/items/links";
import { registerItemPlayer, registerSongsPlayer } from "@/shared/items/play";
import { callFailure } from "@/shared/lib/errors";

/** Awaits `p`; a failure is a toast naming what failed, and `undefined`. */
async function load<T>(what: string, p: Promise<T>): Promise<T | undefined> {
  try {
    return await p;
  } catch (err) {
    toast.error(what, { description: callFailure(err).message });
    return undefined;
  }
}

async function tracksOf(item: Item): Promise<Song[] | undefined> {
  const target = actionItemOf(item);
  if (target?.kind !== "album" && target?.kind !== "playlist") return undefined;
  return load(
    target.kind === "album" ? t`Couldn't load the album` : t`Couldn't load the playlist`,
    collectionTracks(target),
  );
}

/**
 * How shared cards play songs, albums and playlists (shared/items/play):
 * a song alone, a collection from its first track, a track shelf as the
 * queue.
 */
export function registerPlayers() {
  registerSongsPlayer(playSongs);
  registerItemPlayer("song", (item) => {
    if (item.song) playSongs([item.song], 0, item.name);
  });
  const collection = async (item: Item) => {
    const tracks = await tracksOf(item);
    if (tracks) playSongs(tracks, 0, { kind: item.kind, id: item.id });
  };
  registerItemPlayer("album", collection);
  registerItemPlayer("playlist", collection);
}
