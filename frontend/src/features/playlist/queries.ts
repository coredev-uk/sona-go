import {
  PlaylistsService,
  type PlaylistDetail,
  type PlaylistItem,
  type Song,
} from "@bindings/services";
import { useQuery, type QueryClient } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";
import { isLibraryId } from "@/shared/lib/queries";

/**
 * This module's query keys. They sit under "library" so a library sync
 * invalidates them with everything else it stored.
 */
export const playlistKeys = {
  tree: ["library", "playlist-tree"] as const,
  pins: ["library", "pins"] as const,
  recentTargets: ["library", "playlist-recent-targets"] as const,
  suggestions: (id: string) => ["library", "playlist-suggestions", id] as const,
  detail: (id: string) =>
    isLibraryId(id, "playlist") ? keys.libraryPlaylist(id) : keys.catalogPlaylist(id),
};

export function usePlaylist(id: string) {
  return useQuery({
    queryKey: playlistKeys.detail(id),
    queryFn: () => PlaylistsService.Playlist(id),
  });
}

/** Folders and library playlists, from SQLite. */
export function usePlaylistTree() {
  return useQuery({ queryKey: playlistKeys.tree, queryFn: () => PlaylistsService.Tree() });
}

export function usePins() {
  return useQuery({ queryKey: playlistKeys.pins, queryFn: () => PlaylistsService.Pins() });
}

/** Songs Apple suggests for a library playlist (network; cached for the session). */
export function useSuggestions(id: string, enabled: boolean) {
  return useQuery({
    queryKey: playlistKeys.suggestions(id),
    queryFn: () => PlaylistsService.Suggestions(id),
    enabled,
    staleTime: 10 * 60_000,
    retry: false,
  });
}

/** Apple's recently used playlists for "Add to playlist" (library IDs, newest first). */
export function useRecentTargets(enabled: boolean) {
  return useQuery({
    queryKey: playlistKeys.recentTargets,
    queryFn: () => PlaylistsService.RecentTargets(),
    enabled,
    staleTime: 60_000,
    retry: false,
  });
}

/** What an action adds: a song, an album or a playlist, in its own source. */
export function itemOf(
  kind: "song" | "album" | "playlist",
  target: { id: string; source: Song["source"] },
): PlaylistItem {
  return { kind, id: target.id, source: target.source };
}

/** Puts a playlist page returned by an edit into the cache. */
export function setDetail(qc: QueryClient, d: PlaylistDetail) {
  qc.setQueryData(playlistKeys.detail(d.playlist.id), d);
}
