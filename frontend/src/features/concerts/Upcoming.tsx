import type { Concert } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { ChevronDown } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/shared/components/EmptyState";
import { Shelf, ShelfToggle } from "@/shared/components/Shelf";
import { cn } from "@/shared/lib/utils";

import { AgendaSkeleton } from "./ConcertsPage";
import { ConcertRow } from "./ConcertRow";
import { PAGE_SIZE, requestOf, useArtistConcerts, useConcerts } from "./queries";

/** Rows an artist's shelf shows before "Show all". */
const ARTIST_ROWS = 3;
/** Shows on Home's shelf. */
const HOME_SHOWS = 6;

function Rows({
  shows,
  columns = 1,
  artist = false,
}: {
  shows: readonly Concert[];
  columns?: 1 | 2;
  artist?: boolean;
}) {
  return (
    <ul
      className={cn(
        "grid gap-x-8",
        columns === 2 &&
          "min-[900px]:grid-flow-col min-[900px]:grid-cols-2 min-[900px]:grid-rows-3",
      )}
    >
      {shows.map((c) => (
        <ConcertRow key={c.id} concert={c} date artist={artist} />
      ))}
    </ul>
  );
}

/**
 * An artist's upcoming concerts, those near the chosen city first. Left
 * out (nothing is drawn) while loading, when there are none and when they
 * can't be had: the page is about the artist, and it is complete without.
 */
export function ArtistConcerts({ artistId }: { artistId: string }) {
  const { t } = useLingui();
  const q = useArtistConcerts(artistId);
  const [all, setAll] = useState(false);
  const data = q.data;
  if (!data) return null;
  const shows = [...data.nearby, ...data.elsewhere];
  if (shows.length === 0) return null;
  const count = shows.length;
  const city = data.location.name;
  const hidden = shows.length > ARTIST_ROWS;
  return (
    <Shelf
      title={t`Upcoming concerts`}
      actions={
        hidden && (
          <ShelfToggle open={all} onToggle={() => setAll((o) => !o)}>
            {all ? <Trans>Show fewer</Trans> : <Trans>Show all {count}</Trans>}
            <ChevronDown
              aria-hidden
              className={cn("size-3.5 transition-transform duration-100", all && "rotate-180")}
              strokeWidth={1.75}
            />
          </ShelfToggle>
        )
      }
    >
      <Rows shows={all ? shows : shows.slice(0, ARTIST_ROWS)} artist />
      <p className="mt-2 px-2 text-meta text-ink-faint">
        <Link to="/concerts" className="hover:text-ink hover:underline">
          <Trans>More concerts near {city}</Trans>
        </Link>
      </p>
    </Shelf>
  );
}

/**
 * Home's concerts shelf: the next shows near the chosen city, with the
 * agenda one link away. A failure leaves the shelf out (Home has plenty
 * without); no shows says so, with the way to another city.
 */
export function ConcertsShelf() {
  const { t } = useLingui();
  const q = useConcerts(requestOf({ limit: PAGE_SIZE }));
  if (q.isError && !q.data) return null;
  const data = q.data;
  const city = data?.location.name ?? "";
  const title = data ? t`Concerts in ${city}` : t`Concerts`;
  return (
    <Shelf title={title} more={{ to: "/concerts" }}>
      {!data ? (
        <AgendaSkeleton rows={3} />
      ) : data.concerts.length === 0 ? (
        <EmptyState title={t`No concerts`} action={{ label: t`Set location`, to: "/concerts" }}>
          <Trans>No upcoming concerts. Please check back later.</Trans>
        </EmptyState>
      ) : (
        <Rows shows={data.concerts.slice(0, HOME_SHOWS)} columns={2} />
      )}
    </Shelf>
  );
}
