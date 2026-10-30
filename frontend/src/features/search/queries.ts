import {
  SearchService,
  Source,
  type Item,
  type LibrarySearch,
  type RecentItem,
} from "@bindings/services";
import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useCallback } from "react";

import { keys } from "@/shared/lib/keys";

import type { SearchScope } from "./search-params";

/** Apple Music results for a term: every group, in Apple's order. */
export function useCatalogSearch(term: string) {
  return useQuery({
    queryKey: keys.catalogSearch(term),
    queryFn: () => SearchService.Catalog(term),
    enabled: !!term,
    placeholderData: keepPreviousData,
    retry: false,
  });
}

/** Library matches per kind, from the local index. */
export function useLibrarySearch(term: string, perKind: number, enabled = true) {
  return useQuery({
    queryKey: keys.librarySearch(term, perKind),
    queryFn: () => SearchService.Library(term, perKind),
    enabled: enabled && !!term,
    placeholderData: keepPreviousData,
  });
}

/** "See all" pages of one group; pageParam is the next page's offset. */
export function useGroupPages(term: string, group: string) {
  return useInfiniteQuery({
    queryKey: keys.catalogSearchGroup(term, group),
    queryFn: ({ pageParam }) => SearchService.Group(term, group, pageParam),
    initialPageParam: 0,
    getNextPageParam: (last) => (last.next > 0 ? last.next : undefined),
    enabled: !!term && !!group,
    retry: false,
  });
}

/** Browse categories (the empty search page). */
export function useBrowse(enabled: boolean) {
  return useQuery({
    queryKey: keys.catalogBrowse,
    queryFn: () => SearchService.Browse(),
    enabled,
    staleTime: 30 * 60_000,
    retry: false,
  });
}

export function useProfile(id: string) {
  return useQuery({
    queryKey: keys.catalogProfile(id),
    queryFn: () => SearchService.Profile(id),
    retry: false,
  });
}

/** Library search results as cards, per kind. */
export function libraryCards(r: LibrarySearch) {
  return {
    artists: r.artists.map((a) => card("artist", a.id, a.name, a.genre, a.art, { artist: a })),
    albums: r.albums.map((a) => card("album", a.id, a.name, a.artistName, a.art, { album: a })),
    playlists: r.playlists.map((p) =>
      card("playlist", p.id, p.name, p.curatorName, p.art, { playlist: p }),
    ),
    songs: r.songs.map((s) => card("song", s.id, s.name, s.artistName, s.art, { song: s })),
  };
}

function card(
  kind: string,
  id: string,
  name: string,
  subtitle: string,
  art: Item["art"],
  typed: Partial<Item>,
): Item {
  return {
    kind,
    id,
    name,
    subtitle,
    caption: "",
    art,
    wide: { small: "", medium: "", large: "" },
    color: "",
    explicit: false,
    url: "",
    ...typed,
  } as Item;
}

/** The Source of an item's ID (library items come from the local index). */
export function itemSource(item: Item): Source {
  const typed = item.song ?? item.album ?? item.playlist ?? item.artist;
  return typed?.source === Source.SourceLibrary ? Source.SourceLibrary : Source.SourceCatalog;
}

/** Recent searches, and the calls that change them (each refreshes the list). */
export function useRecentSearches() {
  return useQuery({
    queryKey: keys.libraryRecentSearches,
    queryFn: () => SearchService.Recent(),
    staleTime: Infinity,
  });
}

export function useRecentActions() {
  const qc = useQueryClient();
  const refresh = useCallback(
    () => void qc.invalidateQueries({ queryKey: keys.libraryRecentSearches }),
    [qc],
  );
  // Remembering is best-effort: a failed write must never block opening a result.
  const rememberTerm = useCallback(
    (term: string, scope: SearchScope) =>
      void SearchService.RememberTerm(term, scope).then(refresh, () => undefined),
    [refresh],
  );
  const rememberItem = useCallback(
    (item: Item, source: Source = itemSource(item)) => {
      // A library song has no page to return to.
      if (item.kind === "song" && source === Source.SourceLibrary) return;
      const entry: RecentItem = {
        kind: item.kind,
        id: item.id,
        source,
        name: item.name,
        subtitle: item.subtitle,
        art: item.art.small,
      };
      void SearchService.RememberItem(entry).then(refresh, () => undefined);
    },
    [refresh],
  );
  const forget = useCallback(
    (key: string) => SearchService.ForgetRecent(key).then(refresh),
    [refresh],
  );
  const clear = useCallback(() => SearchService.ClearRecent().then(refresh), [refresh]);
  return { rememberTerm, rememberItem, forget, clear };
}
