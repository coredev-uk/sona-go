import { CatalogService, LibraryService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

/** A library album (its tracks in your library). */
export function useLibraryAlbum(id: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.libraryAlbum(id),
    queryFn: () => LibraryService.Album(id),
    enabled,
  });
}

/** A catalogue album page: tracks plus Apple's sections. */
export function useCatalogAlbum(id: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.catalogAlbum(id),
    queryFn: () => CatalogService.Album(id),
    enabled,
  });
}
