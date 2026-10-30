import { CatalogService, LibraryService } from "@bindings/services";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

/** A library artist: their albums and songs in your library. */
export function useLibraryArtist(id: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.libraryArtist(id),
    queryFn: () => LibraryService.Artist(id),
    enabled,
  });
}

/** A catalogue artist page: biography, top songs and shelves. */
export function useCatalogArtist(id: string, enabled = true) {
  return useQuery({
    queryKey: keys.catalogArtist(id),
    queryFn: () => CatalogService.Artist(id),
    enabled,
  });
}

/** Page size of a see-all list. */
const VIEW_PAGE = 50;

/** Every page of an artist view (a see-all list), fetched as it scrolls. */
export function useArtistView(id: string, view: string, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: keys.catalogArtistView(id, view),
    enabled,
    queryFn: ({ pageParam }) => CatalogService.ArtistView(id, view, pageParam, VIEW_PAGE),
    initialPageParam: 0,
    getNextPageParam: (last) => (last.next >= 0 ? last.next : undefined),
  });
}
