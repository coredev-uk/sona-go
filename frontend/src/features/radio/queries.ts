import { RadioService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

/** Query keys; every key starts with "radio" so sign-in changes can drop them all. */
export const radioKeys = {
  all: ["radio"] as const,
  grouping: ["radio", "grouping"] as const,
  live: ["radio", "live"] as const,
  /** Apple's history of the user's stations: user data. */
  recent: ["radio", "recent"] as const,
  station: (id: string) => ["radio", "station", id] as const,
};

/** Editorial radio changes hourly at most; Go caches it and serves it offline. */
const editorialStale = 10 * 60 * 1000;

export function useRadioGrouping() {
  return useQuery({
    queryKey: radioKeys.grouping,
    queryFn: () => RadioService.Grouping(),
    staleTime: editorialStale,
  });
}

export function useLiveStations() {
  return useQuery({
    queryKey: radioKeys.live,
    queryFn: () => RadioService.Live(),
    staleTime: editorialStale,
  });
}

export function useRecentStations(enabled: boolean) {
  return useQuery({
    queryKey: radioKeys.recent,
    queryFn: () => RadioService.Recent(),
    enabled,
    staleTime: 60 * 1000,
  });
}

/** A station page; a live station's schedule is refetched when its programme ends. */
export function useStation(id: string) {
  return useQuery({
    queryKey: radioKeys.station(id),
    queryFn: () => RadioService.Station(id),
    staleTime: 5 * 60 * 1000,
  });
}
