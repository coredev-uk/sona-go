import {
  CatalogService,
  LibraryService,
  PlaylistsService,
  type Item,
  type Song,
} from "@bindings/services";

import { playSongs } from "@/features/player";
import { registerItemPlayer, registerSongsPlayer } from "@/shared/items/play";
import { callFailure } from "@/shared/lib/errors";
import { toast } from "sonner";

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
  if (item.album) {
    const d =
      item.album.source === "library"
        ? await load("Couldn't load the album", LibraryService.Album(item.album.id))
        : await load("Couldn't load the album", CatalogService.Album(item.album.id));
    return d?.tracks;
  }
  if (item.playlist) {
    const d = await load("Couldn't load the playlist", PlaylistsService.Playlist(item.playlist.id));
    return d?.tracks;
  }
  return undefined;
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
    if (tracks) playSongs(tracks, 0, item.name);
  };
  registerItemPlayer("album", collection);
  registerItemPlayer("playlist", collection);
}
