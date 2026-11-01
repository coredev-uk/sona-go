import type { Album } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { Trans } from "@lingui/react/macro";

import { formatTimeLeft, longDate } from "@/shared/lib/format";
import { fullReleaseDate } from "@/shared/lib/release";

/**
 * An upcoming album's release, where its play controls will be: the time
 * left to the minute ("Out in 18 days, 6 hours", re-read every minute by
 * the page, never a ticking counter), the day in full, and how many of
 * its songs are already out (they play from their rows).
 */
export function ReleaseStatus({
  album,
  now,
  out,
  total,
}: {
  album: Album;
  now: number;
  /** Songs already out (playable). */
  out: number;
  total: number;
}) {
  if (album.releaseAt === 0) {
    // Apple gave only a year: no countdown to give.
    const releaseDate = longDate(album.releaseDate);
    return (
      <p className="text-heading text-ink">
        <Trans>Out {releaseDate}</Trans>
      </p>
    );
  }
  const timeLeft = formatTimeLeft(album.releaseAt - now);
  const songsOut =
    out > 0
      ? plural(total, { one: `${out} of # song out now`, other: `${out} of # songs out now` })
      : "";
  return (
    <div>
      <p className="text-heading text-ink">
        <Trans>Out in {timeLeft}</Trans>
      </p>
      <p className="mt-0.5 text-meta text-ink-muted">
        <time dateTime={album.releaseDate}>{fullReleaseDate(album.releaseAt)}</time>
        {songsOut && ` · ${songsOut}`}
      </p>
    </div>
  );
}
