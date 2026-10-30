import { RadioService, type StationEvent } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { radioKeys } from "./queries";
import { onAir } from "./schedule";

/**
 * The programme on air on live station `id` (undefined when the station
 * has no schedule or `id` is not a live stream). It moves on by itself
 * when the slot ends.
 */
export function useOnAirProgramme(id: string | undefined): StationEvent | undefined {
  const q = useQuery({
    queryKey: radioKeys.station(id ?? ""),
    queryFn: () => RadioService.Station(id ?? ""),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
  });
  const [now, setNow] = useState(() => Date.now());
  const programme = q.data ? onAir(q.data.schedule, now) : undefined;
  const until = programme?.endMs ?? 0;
  useEffect(() => {
    if (!id) return;
    // Wake at the end of the slot (or in a minute when nothing is on air).
    const wait = until > 0 ? Math.max(until - Date.now(), 1000) : 60_000;
    const t = window.setTimeout(() => setNow(Date.now()), wait);
    return () => window.clearTimeout(t);
  }, [id, until, now]);
  return programme;
}
