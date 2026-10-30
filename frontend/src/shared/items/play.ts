import type { Item, Song } from "@bindings/services";

/**
 * Playing items from shared components. Shared code cannot import
 * features, so the features that own playback register players here at
 * start-up (discover: songs, albums, playlists; radio: stations).
 */
type ItemPlayer = (item: Item) => void | Promise<void>;
type SongsPlayer = (songs: readonly (Song | undefined)[], start: number, label: string) => void;

const players = new Map<string, ItemPlayer>();
let songsPlayer: SongsPlayer | undefined;

/** Registers how items of `kind` play; a later registration replaces it. */
export function registerItemPlayer(kind: string, play: ItemPlayer) {
  players.set(kind, play);
}

/** Registers how a list of songs plays as the queue (track shelves). */
export function registerSongsPlayer(play: SongsPlayer) {
  songsPlayer = play;
}

/** Whether `item` has a play control: a registered kind that Sona can play. */
export function canPlay(item: Item): boolean {
  if (!players.has(item.kind)) return false;
  switch (item.kind) {
    case "song":
      return !!item.song?.playable;
    case "station":
      return !!item.station?.playable;
    default:
      return true;
  }
}

export function playItem(item: Item) {
  void players.get(item.kind)?.(item);
}

/** Plays `songs` from `start` as the queue, labelled with where they came from. */
export function playSongList(songs: readonly (Song | undefined)[], start: number, label: string) {
  songsPlayer?.(songs, start, label);
}
