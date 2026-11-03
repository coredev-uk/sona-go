import { ConcertsRequest, ConcertsService, type ConcertLocation } from "@bindings/services";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { t } from "@lingui/core/macro";

import { callFailure } from "@/shared/lib/errors";

/** Query keys; every key starts with "concerts" so a sign-in or location change drops them all. */
export const concertsKeys = {
  all: ["concerts"] as const,
  page: (req: ConcertsRequest) => ["concerts", "page", req] as const,
  artist: (id: string) => ["concerts", "artist", id] as const,
  places: (query: string) => ["concerts", "places", query] as const,
  location: ["concerts", "location"] as const,
};

/** How many concerts a listing asks for at first, and for each "Show more". */
export const PAGE_SIZE = 100;

/** Listings change as shows are announced, not by the minute; Go caches them for hours. */
const STALE = 10 * 60 * 1000;

/**
 * The concerts near the chosen place. A change of filter keeps the list on
 * screen (`placeholderData`) until the new one arrives, so the page never
 * flashes empty between choices.
 */
export function useConcerts(req: ConcertsRequest) {
  return useQuery({
    queryKey: concertsKeys.page(req),
    queryFn: () => ConcertsService.Concerts(req),
    staleTime: STALE,
    placeholderData: keepPreviousData,
  });
}

/** The listing's request: the date range, the genre and how many to ask for (0: Go's default). */
export function requestOf(f: {
  from?: string;
  to?: string;
  genre?: string;
  limit?: number;
}): ConcertsRequest {
  return new ConcertsRequest({
    from: f.from ?? "",
    to: f.to ?? "",
    genre: f.genre ?? "",
    limit: f.limit ?? 0,
  });
}

export function useArtistConcerts(artistId: string) {
  return useQuery({
    queryKey: concertsKeys.artist(artistId),
    queryFn: () => ConcertsService.ForArtist(artistId),
    staleTime: STALE,
  });
}

/** Where concerts are looked for now (also carried by every page). */
export function useLocation() {
  return useQuery({
    queryKey: concertsKeys.location,
    queryFn: () => ConcertsService.Location(),
    staleTime: Infinity,
  });
}

/** Cities for the picker: the storefront's own and a few well-known when nothing is typed. */
export function usePlaces(query: string) {
  return useQuery({
    queryKey: concertsKeys.places(query),
    queryFn: () => ConcertsService.Places(query),
    placeholderData: keepPreviousData,
    staleTime: Infinity,
  });
}

/** Chooses the city ("" the storefront's main one); every listing is asked again. */
export function useSetLocation(onDone?: (loc: ConcertLocation) => void) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => ConcertsService.SetLocation(id),
    onSuccess: (loc) => {
      qc.setQueryData(concertsKeys.location, loc);
      void qc.invalidateQueries({ queryKey: concertsKeys.all, refetchType: "active" });
      onDone?.(loc);
    },
    onError: (err) =>
      toast.error(t`Couldn't save your city`, { description: callFailure(err).message }),
  });
}
