import type { QueueItem, Song } from "@bindings/services";
import { createContext, useContext } from "react";

/** What shared lists need to mark the playing track; the player provides it. */
export interface NowPlaying {
  current: QueueItem | null;
  /** Playing or loading: the equaliser moves. */
  playing: boolean;
}

export const NowPlayingContext = createContext<NowPlaying>({ current: null, playing: false });

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
