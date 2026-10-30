import { CatalogService } from "@bindings/services";
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
