import { plural } from "@lingui/core/macro";
import { ScrobbleService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

/** The scrobblers' live state; Go's scrobble:changed replaces it. */
export const scrobbleStatusKey = ["scrobble", "status"] as const;

export function useScrobbleStatus() {
  return useQuery({ queryKey: scrobbleStatusKey, queryFn: () => ScrobbleService.Status() });
}

/** "1 song", "12 songs". */
export function songs(n: number) {
  return plural(n, { one: "# song", other: "# songs" });
}
