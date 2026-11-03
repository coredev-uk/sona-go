import {
  LibraryService,
  type Album,
  type Artist,
  type LibraryEntry,
  type Playlist,
  type Song,
} from "@bindings/services";
import {
  keepPreviousData,
  queryOptions,
  useQueries,
  useQuery,
  type QueryKey,
} from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";

import { keys, type LibraryKind } from "@/shared/lib/keys";

import { useSync } from "./sync";

export function useCounts() {
  return useQuery({ queryKey: keys.libraryCounts, queryFn: () => LibraryService.Counts() });
}

/**
 * LibraryService.SyncStatus. Every answer also brings the sync store up to
 * date (unless a sync event overtook it, see useSync's snapshot), so the
 * indicator follows Go at start-up and whenever the status is refetched.
 */
export const syncStatusQuery = queryOptions({
  queryKey: keys.librarySync,
  queryFn: async () => {
    const at = useSync.getState().events;
    const status = await LibraryService.SyncStatus();
    useSync.getState().snapshot(status.running, at);
    return status;
  },
});

export function useSyncStatus() {
  return useQuery(syncStatusQuery);
}

const PAGE = 200;

/**
 * A long list read in pages of 200, fetched only for the rows on screen
 * (the list reports them through `onRange`). `rows` has the full length;
 * rows of unfetched pages are `undefined`.
 */
function usePagedList<T>(
  pageKey: (page: number) => QueryKey,
  fetch: (offset: number, limit: number) => Promise<{ items: T[]; total: number }>,
) {
  const [range, setRange] = useState<[number, number]>([0, 0]);
  const fetchPage = (page: number) => fetch(page * PAGE, PAGE);
  const first = useQuery({ queryKey: pageKey(0), queryFn: () => fetchPage(0) });
  const total = first.data?.total ?? 0;
  const lastPage = Math.max(0, Math.ceil(total / PAGE) - 1);
  const from = Math.max(1, range[0] - 1);
  const pages: number[] = [];
  for (let p = from; p <= Math.min(lastPage, range[1] + 1); p++) pages.push(p);
  // The combined data keeps its identity until a page's data changes.
  const rest = useQueries({
    queries: pages.map((p) => ({ queryKey: pageKey(p), queryFn: () => fetchPage(p) })),
    combine: (results) => results.map((r) => r.data),
  });

  const rows = useMemo(() => {
    const out: (T | undefined)[] = new Array(total);
    first.data?.items.forEach((it, i) => (out[i] = it));
    rest.forEach((page, j) => page?.items.forEach((it, i) => (out[(from + j) * PAGE + i] = it)));
    return out;
  }, [total, first.data, rest, from]);

  const onRange = useCallback((start: number, end: number) => {
    const next: [number, number] = [Math.floor(start / PAGE), Math.floor(end / PAGE)];
    setRange((r) => (r[0] === next[0] && r[1] === next[1] ? r : next));
  }, []);

  return {
    rows,
    total,
    onRange,
    isPending: first.isPending,
    error: first.error,
    refetch: first.refetch,
    isRefetching: first.isRefetching,
  };
}

interface ListItem {
  songs: Song;
  albums: Album;
  artists: Artist;
  playlists: Playlist;
}

const fetchers: {
  [K in LibraryKind]: (
    sort: string,
    desc: boolean,
    offset: number,
    limit: number,
    type: string,
  ) => Promise<{ items: ListItem[K][]; total: number }>;
} = {
  songs: LibraryService.Songs,
  albums: (sort, desc, offset, limit, type) =>
    LibraryService.Albums(sort, desc, type, offset, limit),
  artists: LibraryService.Artists,
  playlists: LibraryService.Playlists,
};

/**
 * A library list (songs, albums, artists, playlists) in an order. `type`
 * narrows albums to standard albums or singles and EPs ("" is all).
 */
export function useLibraryList<K extends LibraryKind>(
  kind: K,
  sort: string,
  desc: boolean,
  type = "",
) {
  return usePagedList<ListItem[K]>(
    (page) => keys.libraryPage(kind, sort, desc, page, type),
    (offset, limit) => fetchers[kind](sort, desc, offset, limit, type),
  );
}

/** Library albums and playlists, newest added first. */
export function useRecentlyAdded() {
  return usePagedList<LibraryEntry>(keys.libraryRecentlyAdded, (offset, limit) =>
    LibraryService.RecentlyAdded(offset, limit),
  );
}

/** Library songs rated Favourite, most recently favourited first. */
export function useFavoriteSongs() {
  return usePagedList<Song>(keys.libraryFavorites, (offset, limit) =>
    LibraryService.FavoriteSongs(offset, limit),
  );
}

/**
 * The listening history of the last `days` days (this computer, plus
 * Apple's recently played when reachable). Refetched when the playing song
 * changes (under keys.libraryRecent); earlier days keep showing while more
 * load.
 */
export function useHistory(days: number) {
  return useQuery({
    queryKey: keys.libraryHistory(days),
    queryFn: () => LibraryService.History(days),
    placeholderData: keepPreviousData,
  });
}

/** Library music videos (Apple; needs the network). Libraries rarely hold more than a page. */
export function useMusicVideos() {
  return useQuery({
    queryKey: keys.libraryMusicVideos,
    queryFn: async () => {
      const first = await LibraryService.MusicVideos(0, 100);
      const items = [...first.items];
      while (items.length < first.total) {
        const next = await LibraryService.MusicVideos(items.length, 100);
        if (next.items.length === 0) break;
        items.push(...next.items);
      }
      return items;
    },
    staleTime: 10 * 60_000,
  });
}
