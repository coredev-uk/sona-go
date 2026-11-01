import { useEffect, useState } from "react";

/**
 * The clock, re-read every `ms`: for what follows it (a station's on-air
 * marker, an album's release countdown) without a ticking second counter.
 */
export function useNow(ms: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), ms);
    return () => window.clearInterval(t);
  }, [ms]);
  return now;
}
