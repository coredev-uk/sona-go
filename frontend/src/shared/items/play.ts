import type { Item, MusicVideo, Song } from "@bindings/services";

import { currentCapabilities } from "@/shared/lib/capabilities";
import { isUpcoming } from "@/shared/lib/release";

/**
 * Playing items from shared components. Shared code cannot import
 * features, so the features that own playback register players here at
 * start-up (discover: songs, albums, playlists, music videos; radio:
 * stations).
 */
type ItemPlayer = (item: Item) => void | Promise<void>;
type SongsPlayer = (songs: readonly (Song | undefined)[], start: number, label: string) => void;
type VideoPlayer = (video: MusicVideo) => void;

const players = new Map<string, ItemPlayer>();
let songsPlayer: SongsPlayer | undefined;
let videoPlayer: VideoPlayer | undefined;

/** Registers how items of `kind` play; a later registration replaces it. */
export function registerItemPlayer(kind: string, play: ItemPlayer) {
  players.set(kind, play);
}

/** Registers how a list of songs plays as the queue (track shelves). */
export function registerSongsPlayer(play: SongsPlayer) {
  songsPlayer = play;
}

/** Registers how a music video tile (a MusicVideo, not an Item) plays. */
export function registerVideoPlayer(play: VideoPlayer) {
  videoPlayer = play;
}

/**
 * Whether music videos have a play control: a video player is registered
 * and this platform and build play them (Capabilities.playback.video;
 * false until Go has answered).
 */
export function canPlayVideo(): boolean {
  return !!videoPlayer && (currentCapabilities()?.playback.video ?? false);
}

/** Plays a music video alone, as the queue. */
export function playVideo(video: MusicVideo) {
  videoPlayer?.(video);
}

/**
 * Whether `item` has a play control: a registered kind that Sona can play
 * (an album not out yet has none; a music video only where videos play).
 */
export function canPlay(item: Item): boolean {
  if (item.kind === "music-video") return !!item.musicVideo && canPlayVideo();
  if (!players.has(item.kind)) return false;
  switch (item.kind) {
    case "song":
      return !!item.song?.playable;
    case "album":
      return !item.album || !isUpcoming(item.album);
    case "station":
      return !!item.station?.playable;
    default:
      return true;
  }
}

export function playItem(item: Item) {
  if (item.kind === "music-video") {
    if (item.musicVideo) playVideo(item.musicVideo);
    return;
  }
  void players.get(item.kind)?.(item);
}

/** Plays `songs` from `start` as the queue, labelled with where they came from. */
export function playSongList(songs: readonly (Song | undefined)[], start: number, label: string) {
  songsPlayer?.(songs, start, label);
}
