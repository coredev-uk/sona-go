import {
  LibraryService,
  type Album,
  type Artist,
  type LibraryEntry,
  type Playlist,
  type Song,
} from "@bindings/services";
import { useQueries, useQuery, type QueryKey } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";

import { keys, type LibraryKind } from "@/shared/lib/keys";

export function useCounts() {
  return useQuery({ queryKey: keys.libraryCounts, queryFn: () => LibraryService.Counts() });
}

export function useSyncStatus() {
  return useQuery({ queryKey: keys.librarySync, queryFn: () => LibraryService.SyncStatus() });
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
  const pages: number[] = [];
  for (let p = Math.max(1, range[0] - 1); p <= Math.min(lastPage, range[1] + 1); p++) pages.push(p);
  const rest = useQueries({
    queries: pages.map((p) => ({ queryKey: pageKey(p), queryFn: () => fetchPage(p) })),
  });

  // Pages change identity only when their data does; the stamp tracks that.
  const stamp = rest.map((r) => `${r.dataUpdatedAt}`).join() + pages.join();
  const rows = useMemo(() => {
    const out: (T | undefined)[] = new Array(total);
    first.data?.items.forEach((it, i) => (out[i] = it));
    rest.forEach((r, j) => r.data?.items.forEach((it, i) => (out[pages[j] * PAGE + i] = it)));
    return out;
    // `rest` and `pages` are new arrays every render; `stamp` covers them.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [total, first.data, stamp]);

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
  ) => Promise<{ items: ListItem[K][]; total: number }>;
} = {
  songs: LibraryService.Songs,
  albums: LibraryService.Albums,
  artists: LibraryService.Artists,
  playlists: LibraryService.Playlists,
};

/** A library list (songs, albums, artists, playlists) in an order. */
export function useLibraryList<K extends LibraryKind>(kind: K, sort: string, desc: boolean) {
  return usePagedList<ListItem[K]>(
    (page) => keys.libraryPage(kind, sort, desc, page),
    (offset, limit) => fetchers[kind](sort, desc, offset, limit),
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

/** The account's personal mixes (Apple; needs the network). */
export function useMadeForYou() {
  return useQuery({
    queryKey: keys.libraryMadeForYou,
    queryFn: () => LibraryService.MadeForYou(),
    staleTime: 10 * 60_000,
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
