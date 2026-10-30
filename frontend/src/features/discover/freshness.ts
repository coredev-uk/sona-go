import { relativeTime } from "@/shared/lib/format";

/** "Updated 12 min ago", or the honest stale line when Apple couldn't be reached. */
export function freshness(fetchedAt: number, stale: boolean): string | undefined {
  if (!fetchedAt) return undefined;
  const when = relativeTime(fetchedAt);
  return stale ? `Couldn't reach Apple Music · showing the copy from ${when}` : `Updated ${when}`;
}
