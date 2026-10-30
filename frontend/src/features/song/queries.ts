import { CatalogService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

/** A catalogue song page: album, credits, lyrics excerpt, videos. */
export function useSongPage(id: string) {
  return useQuery({
    queryKey: keys.catalogSongPage(id),
    queryFn: () => CatalogService.SongPage(id),
  });
}

/** The song's full lyrics (shared with the now-playing lyrics view's cache). */
export function useFullLyrics(id: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.lyrics(id),
    queryFn: () => CatalogService.Lyrics(id),
    enabled,
    retry: false,
  });
}
