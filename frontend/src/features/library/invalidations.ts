import type { QueryKey } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

/** sync:progress payload (library.Progress). */
export interface SyncProgress {
  domain: string;
  done: number;
  total: number;
  finished: boolean;
}

/** Library domains whose rows a sync domain writes. */
const domainLists: Record<string, QueryKey[]> = {
  artists: [
    ["library", "artists"],
    ["library", "artist"],
  ],
  albums: [
    ["library", "albums"],
    ["library", "album"],
    ["library", "recently-added"],
  ],
  songs: [
    ["library", "songs"],
    ["library", "album"],
    ["library", "artist"],
    ["library", "favorite-songs"],
  ],
  playlists: [
    ["library", "playlists"],
    ["library", "recently-added"],
  ],
  "playlist-tracks": [["library", "playlist"]],
  ratings: [
    ["library", "favorite-songs"],
    ["library", "state"],
  ],
};

/**
 * Queries invalidated by sync progress: when a domain finishes, its lists
 * and the counts refresh, so the library fills in domain by domain instead
 * of waiting for the whole sync. Mid-domain ticks only move the indicator.
 */
export function syncInvalidations(p: SyncProgress): QueryKey[] {
  if (!p.finished) return [];
  return [...(domainLists[p.domain] ?? [keys.library]), keys.libraryCounts, keys.librarySync];
}

/** sync:done: everything the library shows, plus the search index. */
export const syncDoneInvalidations: QueryKey[] = [keys.library];
