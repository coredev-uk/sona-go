import type { OnAir, QueueItem, Song } from "@bindings/services";
import { createContext, useContext } from "react";

/** What shared lists need to mark the playing track; the player provides it. */
export interface NowPlaying {
  current: QueueItem | null;
  /** Playing or loading: the equaliser moves. */
  playing: boolean;
  /** What the playing local broadcaster's stream names on air (song, artist, cover); null otherwise. */
  onAir: OnAir | null;
  /**
   * The page and the queue are covered by the Now Playing view or
   * full-screen lyrics: nothing under them needs to move.
   */
  covered: boolean;
}

export const NowPlayingContext = createContext<NowPlaying>({
  current: null,
  playing: false,
  onAir: null,
  covered: false,
});

export function useNowPlaying(): NowPlaying {
  return useContext(NowPlayingContext);
}

/** True when `song` is the queue's current item, wherever the song is listed. */
export function isCurrent(
  song: Pick<Song, "id" | "catalogId">,
  current: QueueItem | null,
): boolean {
  if (!current) return false;
  if (current.id === song.id) return true;
  return !!song.catalogId && current.catalogId === song.catalogId;
}
