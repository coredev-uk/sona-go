import {
  CatalogService,
  LibraryService,
  PlaylistsService,
  Source,
  type Song,
} from "@bindings/services";

import type { ActionItem } from "./registry";

/** The songs a song, album or playlist holds (an album's or playlist's tracks, in order). */
export async function tracksOf(item: ActionItem): Promise<Song[]> {
  switch (item.kind) {
    case "song":
      return [item.target];
    case "album": {
      const a = item.target;
      const d =
        a.source === Source.SourceLibrary
          ? await LibraryService.Album(a.id)
          : await CatalogService.Album(a.id);
      return d.tracks;
    }
    case "playlist":
      return (await PlaylistsService.Playlist(item.target.id)).tracks;
    default:
      return [];
  }
}

/** Every song the items hold, in item order; rejects when a collection fails to load. */
export async function songsOf(items: readonly ActionItem[]): Promise<Song[]> {
  if (items.every((i) => i.kind === "song")) return items.map((i) => i.target as Song);
  const lists = await Promise.all(items.map(tracksOf));
  return lists.flat();
}
