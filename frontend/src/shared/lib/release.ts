import type { Album } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { dateFormat } from "@/shared/lib/format";

/**
 * Pre-release albums (docs/frontend-architecture.md, "Pre-release
 * albums"). Go marks an album not out yet (`prerelease`) when it makes the
 * DTO; `releaseAt` (local midnight on the release date, 0 without a full
 * date) lets an open page see the release happen.
 */
type Release = Pick<Album, "prerelease" | "releaseAt">;

/** Whether an album is still to come at `now`. */
export function isUpcoming(album: Release, now = Date.now()): boolean {
  return album.prerelease && (album.releaseAt === 0 || now < album.releaseAt);
}

/** A release day, short: "19 Nov", with the year when it is not this one's. */
export function shortReleaseDate(releaseAt: number, now = Date.now()): string {
  const sameYear = new Date(releaseAt).getFullYear() === new Date(now).getFullYear();
  return dateFormat(
    sameYear
      ? { day: "numeric", month: "short" }
      : { day: "numeric", month: "short", year: "numeric" },
  ).format(releaseAt);
}

/** A release day in full: "Thursday 19 November 2026". */
export function fullReleaseDate(releaseAt: number): string {
  return dateFormat({ weekday: "long", day: "numeric", month: "long", year: "numeric" }).format(
    releaseAt,
  );
}

/**
 * The badge on an upcoming album's cover ("Out 19 Nov"); undefined for an
 * album that is out or has no full date.
 */
export function releaseBadge(album: Release, now = Date.now()): string | undefined {
  if (!isUpcoming(album, now) || album.releaseAt === 0) return undefined;
  const releaseDate = shortReleaseDate(album.releaseAt, now);
  return t`Out ${releaseDate}`;
}
