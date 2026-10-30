import { DiscoverService, type ItemPage } from "@bindings/services";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";

/** Query keys; every key starts with "discover" so a sign-in change can drop them all. */
export const discoverKeys = {
  all: ["discover"] as const,
  home: ["discover", "home"] as const,
  homeShelf: (id: string) => ["discover", "home-shelf", id] as const,
  page: (kind: string, id: string) => ["discover", "page", kind, id] as const,
  room: (id: string, sort: string) => ["discover", "room", id, sort] as const,
  charts: (genre: string) => ["discover", "charts", genre] as const,
  chart: (key: string, chartId: string, genre: string) =>
    ["discover", "chart", key, chartId, genre] as const,
};

/** Editorial pages change hourly at most; Go caches them and serves stale copies offline. */
const editorialStale = 10 * 60 * 1000;

export type EditorialKind = "new" | "curator" | "multiroom" | "grouping";

export function useEditorialPage(kind: EditorialKind, id: string) {
  return useQuery({
    queryKey: discoverKeys.page(kind, id),
    queryFn: () => {
      switch (kind) {
        case "new":
          return DiscoverService.New();
        case "curator":
          return DiscoverService.Curator(id);
        case "multiroom":
          return DiscoverService.Multiroom(id);
        case "grouping":
          return DiscoverService.Grouping(id);
      }
    },
    staleTime: editorialStale,
  });
}

export function useRoom(id: string, sort: string) {
  return useQuery({
    queryKey: discoverKeys.room(id, sort),
    queryFn: () => DiscoverService.Room(id, sort),
    staleTime: editorialStale,
  });
}

/**
 * Further pages of a room or chart, from `start` (the first page's next
 * offset). Pages load only when asked (`fetchNextPage`), as the list nears
 * its end.
 */
export function useMorePages<K extends readonly unknown[]>(
  key: K,
  start: number,
  load: (offset: number) => Promise<ItemPage>,
) {
  return useInfiniteQuery({
    queryKey: [...key, "more", start],
    queryFn: ({ pageParam }) => load(pageParam),
    initialPageParam: start,
    getNextPageParam: (last: ItemPage) => last.next || undefined,
    enabled: false,
    staleTime: editorialStale,
  });
}

export function useCharts(genre: string) {
  return useQuery({
    queryKey: discoverKeys.charts(genre),
    queryFn: () => DiscoverService.Charts(genre),
    staleTime: editorialStale,
  });
}
