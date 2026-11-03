import type { ConcertLocation, ConcertsPage as Page } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { getRouteApi } from "@tanstack/react-router";
import { MapPin } from "lucide-react";
import { useMemo, useState } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { relativeTime } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { ConcertRow, DateBlock } from "./ConcertRow";
import { groupShows, isoDay, rangeFor, showDayLong, type When } from "./dates";
import { useRoving } from "./roving";
import { LocationDialog } from "./LocationDialog";
import { PAGE_SIZE, requestOf, useConcerts, useLocation } from "./queries";
import type { ConcertsSearch } from "./search-params";

const route = getRouteApi("/concerts");

/** Radix Select holds no empty value; "any" stands for no filter. */
const ANY = "any";

/**
 * Concerts: the shows near a city, soonest first, set as an agenda by the
 * venue's own calendar days (docs/design.md, Concerts).
 */
export function ConcertsPage() {
  const { t } = useLingui();
  const search = route.useSearch();
  const navigate = route.useNavigate();
  const auth = useAuth();
  const [choosing, setChoosing] = useState(false);

  const range = rangeFor(search, new Date());
  const filterKey = [range?.from, range?.to, search.genre].join("|");
  const [more, setMore] = useState({ key: "", limit: PAGE_SIZE });
  const limit = more.key === filterKey ? more.limit : PAGE_SIZE;

  const q = useConcerts(
    requestOf({ from: range?.from, to: range?.to, genre: search.genre, limit }),
  );
  const fallback = useLocation().data;
  const page = q.data;
  const location = page?.location ?? fallback;

  const set = (next: Partial<ConcertsSearch>) =>
    void navigate({ search: (prev) => ({ ...prev, ...next }), replace: true });

  const yoursAvailable = hasAccount(auth.data?.state) && (page?.yourArtists ?? 0) > 0;
  const onlyYours = !!search.yours && yoursAvailable;
  const shown = useMemo(
    () => (onlyYours ? (page?.concerts ?? []).filter((c) => c.yourArtist) : (page?.concerts ?? [])),
    [page, onlyYours],
  );
  const filtered = !!range || !!search.genre || onlyYours;

  return (
    <>
      <PageHeader
        title={t`Concerts`}
        subline={location && <Subline page={page} location={location} />}
        actions={
          <Button variant="outline" size="sm" onClick={() => setChoosing(true)}>
            <MapPin aria-hidden className="size-4" strokeWidth={1.5} />
            {location?.isDefault ? t`Set location` : (location?.name ?? t`Location`)}
          </Button>
        }
      />
      <Filters search={search} page={page} yoursAvailable={yoursAvailable} onChange={set} />
      {q.isPending ? (
        <AgendaSkeleton />
      ) : q.isError && !page ? (
        <ErrorBlock
          title={t`Couldn't load the concerts`}
          error={q.error}
          onRetry={() => void q.refetch()}
          retrying={q.isRefetching}
        />
      ) : shown.length === 0 ? (
        <NoConcerts
          location={page?.location}
          filtered={filtered}
          onClear={() =>
            set({
              when: undefined,
              from: undefined,
              to: undefined,
              genre: undefined,
              yours: undefined,
            })
          }
          onChoose={() => setChoosing(true)}
        />
      ) : (
        <div aria-busy={q.isFetching && q.isPlaceholderData}>
          <Agenda shows={shown} />
          {page?.more && !onlyYours && (
            <div className="mt-8 flex justify-center">
              <Button
                variant="outline"
                size="sm"
                disabled={q.isFetching}
                onClick={() => setMore({ key: filterKey, limit: limit + PAGE_SIZE })}
              >
                {q.isFetching ? <Trans>Loading…</Trans> : <Trans>Show more concerts</Trans>}
              </Button>
            </div>
          )}
        </div>
      )}
      <LocationDialog open={choosing} onOpenChange={setChoosing} location={location} />
    </>
  );
}

/** What the list is about, in one line: the place, and how fresh the listing is. */
function Subline({ page, location }: { page: Page | undefined; location: ConcertLocation }) {
  const { t } = useLingui();
  const city = location.name;
  const place = location.isDefault
    ? t`Near ${city}, the main city of your Apple Music storefront`
    : t`Near ${city}`;
  if (!page?.fetchedAt) return place;
  const when = relativeTime(page.fetchedAt);
  return (
    <>
      {place}
      {" · "}
      {page.stale
        ? t`Couldn't reach Apple Music · showing the copy from ${when}`
        : t`Updated ${when}`}
    </>
  );
}

function Filters({
  search,
  page,
  yoursAvailable,
  onChange,
}: {
  search: ConcertsSearch;
  page: Page | undefined;
  yoursAvailable: boolean;
  onChange: (next: Partial<ConcertsSearch>) => void;
}) {
  const { t } = useLingui();
  const genres = page?.genres ?? [];
  const yours = page?.yourArtists ?? 0;
  return (
    <div
      role="group"
      aria-label={t`Filter concerts`}
      className="mb-6 flex flex-wrap items-center gap-x-3 gap-y-2"
    >
      <Select
        value={search.when ?? ANY}
        onValueChange={(v) =>
          onChange(
            v === ANY
              ? { when: undefined, from: undefined, to: undefined }
              : { when: v as When, ...(v === "dates" ? {} : { from: undefined, to: undefined }) },
          )
        }
      >
        <SelectTrigger aria-label={t`Dates`} className="h-8 w-44 text-ui">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ANY}>{t`Any date`}</SelectItem>
          <SelectItem value="week">{t`This week`}</SelectItem>
          <SelectItem value="weekend">{t`This weekend`}</SelectItem>
          <SelectItem value="month">{t`Next 30 days`}</SelectItem>
          <SelectItem value="dates">{t`Choose dates…`}</SelectItem>
        </SelectContent>
      </Select>
      {search.when === "dates" && (
        <>
          <DateInput
            label={t`From`}
            value={search.from ?? ""}
            min={isoDay(new Date())}
            max={search.to}
            onChange={(from) => onChange({ from: from || undefined })}
          />
          <DateInput
            label={t`To`}
            value={search.to ?? ""}
            min={search.from}
            onChange={(to) => onChange({ to: to || undefined })}
          />
          {!rangeFor(search, new Date()) && (
            <span className="text-sub text-ink-muted">
              <Trans>Choose both dates</Trans>
            </span>
          )}
        </>
      )}
      {genres.length > 1 && (
        <Select
          value={search.genre ?? ANY}
          onValueChange={(v) => onChange({ genre: v === ANY ? undefined : v })}
        >
          <SelectTrigger aria-label={t`Genres`} className="h-8 w-48 text-ui">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {genres.map((g) => (
              <SelectItem key={g.id || ANY} value={g.id || ANY}>
                {g.id ? g.name : t`All genres`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {yoursAvailable && (
        <label className="ml-1 inline-flex h-8 items-center gap-2 text-ui text-ink">
          <Switch
            size="sm"
            checked={!!search.yours}
            onCheckedChange={(on) => onChange({ yours: on ? true : undefined })}
          />
          <Trans>Your artists ({yours})</Trans>
        </label>
      )}
    </div>
  );
}

function DateInput({
  label,
  value,
  min,
  max,
  onChange,
}: {
  label: string;
  value: string;
  min?: string;
  max?: string;
  onChange: (v: string) => void;
}) {
  return (
    <label className="inline-flex h-8 items-center gap-2 text-sub text-ink-muted">
      {label}
      <Input
        type="date"
        value={value}
        min={min}
        max={max}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 w-40 text-ui text-ink"
      />
    </label>
  );
}

/** The shows by month, then by the venue's day: a date column and that day's rows. */
function Agenda({ shows }: { shows: Page["concerts"] }) {
  const groups = useMemo(() => groupShows(shows), [shows]);
  const roving = useRoving([shows]);
  return (
    <div {...roving}>
      {groups.map((month) => (
        <section key={month.key} aria-label={month.title} className="mt-10 first:mt-0">
          <h2 className="mb-2 text-heading text-ink">{month.title}</h2>
          {month.days.map((day) => (
            <div key={day.day} className="grid grid-cols-[3.5rem_minmax(0,1fr)] gap-x-4 py-1">
              <DateBlock concert={day.shows[0]} className="px-2 pt-3" />
              <ul aria-label={showDayLong(day.shows[0])}>
                {day.shows.map((c) => (
                  <ConcertRow key={c.id} concert={c} />
                ))}
              </ul>
            </div>
          ))}
        </section>
      ))}
    </div>
  );
}

function NoConcerts({
  location,
  filtered,
  onClear,
  onChoose,
}: {
  location: ConcertLocation | undefined;
  filtered: boolean;
  onClear: () => void;
  onChoose: () => void;
}) {
  const { t } = useLingui();
  const city = location?.name ?? "";
  return filtered ? (
    <EmptyState title={t`No concerts match`} action={{ label: t`Clear filters`, onClick: onClear }}>
      <Trans>
        Apple Music lists nothing near {city} for these filters. Widen the dates or choose another
        genre.
      </Trans>
    </EmptyState>
  ) : (
    <EmptyState
      title={t`No concerts near ${city}`}
      action={{ label: t`Choose another city`, onClick: onChoose }}
    >
      <Trans>
        Apple Music lists no upcoming concerts in this area. A larger city nearby may have some.
      </Trans>
    </EmptyState>
  );
}

/** The agenda's frame while it loads: a month heading and rows at their real height. */
export function AgendaSkeleton({ rows = 5 }: { rows?: number }) {
  const { t } = useLingui();
  return (
    <div aria-busy aria-label={t`Loading`}>
      <div className="mb-3 h-4 w-32 rounded-sm bg-hover" />
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex min-h-[72px] items-center gap-4 px-2 py-2">
          <div className="h-10 w-14 rounded-sm bg-hover" />
          <div className="size-14 rounded-full bg-hover" />
          <div className="flex-1">
            <div className="h-3.5 w-1/3 rounded-sm bg-hover" />
            <div className="mt-2 h-3 w-1/4 rounded-sm bg-hover" />
          </div>
          <div className="h-8 w-20 rounded-md bg-hover" />
        </div>
      ))}
    </div>
  );
}
