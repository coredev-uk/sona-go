import { CatalogService, MotionService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

export function useLyrics(catalogId: string | undefined) {
  return useQuery({
    queryKey: keys.lyrics(catalogId ?? ""),
    queryFn: () => CatalogService.Lyrics(catalogId ?? ""),
    enabled: !!catalogId,
    retry: false,
  });
}

/**
 * The motion cover of the playing song's album ("" when it has none),
 * fetched only while motion may play.
 */
export function useAlbumMotion(albumId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["catalog", "album-motion", albumId] as const,
    queryFn: () => MotionService.Album(albumId),
    enabled: enabled && !!albumId,
    staleTime: 60 * 60 * 1000,
    retry: false,
  });
}
