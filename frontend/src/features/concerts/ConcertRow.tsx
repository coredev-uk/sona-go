import type { Concert } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Browser } from "@wailsio/runtime";
import { ArrowUpRight, ChevronDown } from "lucide-react";

import { Artwork } from "@/shared/components/Artwork";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";

import { distanceLabel, showDate, showTime, showWeekday, venueLine } from "./dates";
import { dateFormat } from "@/shared/lib/format";

/** The day as a ledger's date column: the weekday over the day number (and the month, off a day heading). */
export function DateBlock({
  concert,
  month = false,
  className,
}: {
  concert: Concert;
  month?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("w-14 shrink-0 tabular-nums", className)}>
      <p className="text-meta text-ink-muted">{showWeekday(concert)}</p>
      <p className="text-page text-ink">{showDate(concert)}</p>
      {month && (
        <p className="text-meta text-ink-muted">
          {dateFormat({ timeZone: concert.timezone, month: "short" }).format(concert.startsAt)}
        </p>
      )}
    </div>
  );
}

/** Opens a ticket seller's page in the browser; Go let through only https links. */
function open(url: string) {
  void Browser.OpenURL(url);
}

/** Tickets: one seller is a button, several a menu of them. Nothing when Apple lists none. */
export function TicketsButton({ concert }: { concert: Concert }) {
  const { t } = useLingui();
  const lineup = concert.lineup;
  const sellers = concert.tickets;
  if (sellers.length === 0) return null;
  if (sellers.length === 1) {
    const seller = sellers[0].vendor;
    return (
      <Button
        data-stop
        variant="ghost"
        size="sm"
        className="text-ink"
        onClick={() => open(sellers[0].url)}
        aria-label={t`Get tickets for ${lineup} from ${seller}, opens in your browser`}
      >
        <Trans>Get tickets</Trans>
        <ArrowUpRight aria-hidden className="size-3.5" strokeWidth={1.75} />
      </Button>
    );
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          data-stop
          variant="ghost"
          size="sm"
          className="text-ink"
          aria-label={t`Get tickets for ${lineup}`}
          // ↓ moves down the agenda (roving.ts); Enter and Space open the menu.
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") e.preventDefault();
          }}
        >
          <Trans>Get tickets</Trans>
          <ChevronDown aria-hidden className="size-3.5" strokeWidth={1.75} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {sellers.map((s) => (
          <DropdownMenuItem key={s.url} onSelect={() => open(s.url)}>
            {s.vendor}
            <ArrowUpRight
              aria-label={t`Opens in your browser`}
              className="ml-auto size-3.5 text-ink-muted"
              strokeWidth={1.75}
            />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * One show: the headliner's portrait, the line-up (a link to the artist),
 * the venue, the start at the venue's clock and how far it is, and
 * tickets. `date` puts the day's date column first, for lists that don't
 * sit under a day heading. On an artist's own page (`artist`) the portrait
 * and the name are left out, since the page is about them: the venue leads,
 * and the line-up only when others play too.
 */
export function ConcertRow({
  concert,
  date = false,
  artist = false,
}: {
  concert: Concert;
  date?: boolean;
  artist?: boolean;
}) {
  const { t } = useLingui();
  const lead = concert.artists[0];
  const distance = distanceLabel(concert.distanceKm);
  const meta = [showTime(concert), distance].filter(Boolean).join(" · ");
  const venue = concert.venue;
  const title = artist ? (venue?.name ?? concert.lineup) : concert.lineup;
  const place = artist ? venue?.city : venueLine(concert);
  const others = artist && lead && concert.lineup !== lead.name ? concert.lineup : "";
  return (
    <li
      data-row
      className="flex min-h-[72px] items-center gap-4 rounded-md px-2 py-2 hover:bg-hover"
    >
      {date && <DateBlock concert={concert} month className="mr-1" />}
      {!artist && <Artwork src={lead?.art.medium} size={56} shape="round" alt="" />}
      <div className="min-w-0 flex-1">
        <p className="truncate text-ui font-medium text-ink">
          {!artist && lead?.id ? (
            <Link data-stop to="/artist/$id" params={{ id: lead.id }} className="hover:underline">
              {title}
            </Link>
          ) : (
            title
          )}
        </p>
        {place && <p className="truncate text-sub text-ink-muted">{place}</p>}
        {others && <p className="truncate text-sub text-ink-muted">{others}</p>}
        <p className="truncate text-meta text-ink-faint">
          {meta}
          {concert.yourArtist && (
            <>
              {meta && " · "}
              <span className="text-ink-muted">{t`Your artist`}</span>
            </>
          )}
        </p>
      </div>
      <TicketsButton concert={concert} />
    </li>
  );
}
