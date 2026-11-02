import type { Section, Station } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import type { UseQueryResult } from "@tanstack/react-query";
import { useId, type ReactNode } from "react";

import { hasAccount, isSignedOut, useAuth } from "@/features/auth";
import type { CoverItem } from "@/shared/components/CoverGrid";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { EditorialSections } from "@/shared/items/EditorialSections";
import { STATION_GRID } from "@/shared/items/StationList";
import { openAccountDialog } from "@/shared/lib/account";
import { cn } from "@/shared/lib/utils";

import { bandTitle, placeSections, type BandId } from "./bands";
import { playStation } from "./commands";
import { OnAirBoard, OnAirSkeleton } from "./OnAir";
import { useLiveStations, useRadioGrouping, useRecentStations } from "./queries";

/** A station as a cover tile: its page, a play control, its menu. */
function stationCover(s: Station): CoverItem {
  return {
    key: s.id,
    title: s.name,
    subtitle: s.tagline || s.showName || s.provider || undefined,
    art: s.art.medium,
    motion: s.motion || undefined,
    link: { to: "/station/$id", params: { id: s.id } },
    // The tile names its play control "Play {label}".
    play: s.playable ? { label: s.name, run: () => void playStation(s.id) } : undefined,
    target: { kind: "station", target: s },
  };
}

/** True for a section Apple fills with its live stations, which the board above already lists. */
function allLive(s: Section): boolean {
  return s.items.length > 0 && s.items.every((i) => i.station?.kind === "live");
}

/**
 * Radio: what's on air now first (Apple's live stations as a now-and-next
 * board, then the local broadcasters), then Apple's radio page in bands:
 * stations to tune to (the user's own first), shows and episodes, and the
 * ways to browse further.
 */
export function RadioPage() {
  const { t } = useLingui();
  const auth = useAuth();
  const state = auth.data?.state;
  const signedIn = hasAccount(state);
  const live = useLiveStations();
  const recent = useRecentStations(signedIn);
  const grouping = useRadioGrouping();
  const retry = () => {
    void live.refetch();
    void grouping.refetch();
  };

  // Apple's page repeats the live stations (a featured row and a shelf);
  // the board already shows them, unless Sona couldn't list them.
  const boardShown = !!live.data?.length;
  const { live: liveSections, bands } = placeSections(
    (grouping.data?.sections ?? []).filter((s) => !(boardShown && allLive(s))),
  );

  if (live.isError && grouping.isError) {
    // Nothing on the page loaded: one block says so, not one per part.
    return (
      <>
        <PageHeader title={t`Radio`} />
        <ErrorBlock
          title={t`Couldn't load Apple Music's radio`}
          error={grouping.error}
          onRetry={retry}
          retrying={live.isRefetching || grouping.isRefetching}
        />
      </>
    );
  }

  const yours =
    signedIn && recent.data?.length ? (
      <CoverShelf title={t`Your stations`} items={recent.data.map(stationCover)} />
    ) : isSignedOut(state) ? (
      <EmptyState
        title={t`Your stations`}
        action={{ label: t`Sign in`, onClick: openAccountDialog }}
      >
        <Trans>Sign in to see the stations you play in Apple Music on any of your devices.</Trans>
      </EmptyState>
    ) : null;

  return (
    <>
      <PageHeader title={t`Radio`} />
      <LiveNow query={live}>
        {liveSections.length > 0 && (
          <div className="mt-10">
            <EditorialSections sections={liveSections} />
          </div>
        )}
      </LiveNow>
      {grouping.isPending ? (
        <BandsSkeleton />
      ) : grouping.isError ? (
        <div className="mt-14">
          {yours && <div className="mb-10">{yours}</div>}
          <ErrorBlock
            title={t`Couldn't load Apple Music's radio page`}
            error={grouping.error}
            onRetry={() => void grouping.refetch()}
            retrying={grouping.isRefetching}
          />
        </div>
      ) : (
        bands.map((band) => {
          const lead = band.id === "stations" ? yours : null;
          if (!lead && band.sections.length === 0) return null;
          return (
            <Band key={band.id} id={band.id}>
              {lead}
              {band.sections.map((s) => (
                <div
                  key={s.id}
                  // An untitled link list continues the one above it.
                  className={cn(!(s.kind === "links" && !s.title) && "mt-10 first:mt-0")}
                >
                  <EditorialSections sections={[s]} />
                </div>
              ))}
            </Band>
          );
        })
      )}
    </>
  );
}

/**
 * What's on air: Apple Music's live stations as the board, then the live
 * shelves of Apple's page (the local broadcasters). The section's heading
 * is at the bands' size, as the first of them.
 */
function LiveNow({ query, children }: { query: UseQueryResult<Station[]>; children: ReactNode }) {
  const { t } = useLingui();
  return (
    <section aria-labelledby="radio-live">
      <h2 id="radio-live" className="mb-5 text-page text-ink">
        <Trans>Live now</Trans>
      </h2>
      {query.isPending ? (
        <OnAirSkeleton />
      ) : query.isError ? (
        <ErrorBlock
          title={t`Couldn't load the live stations`}
          error={query.error}
          onRetry={() => void query.refetch()}
          retrying={query.isRefetching}
        />
      ) : query.data.length === 0 ? (
        <EmptyState title={t`No live stations`}>
          <Trans>Apple Music lists no live radio for your storefront right now.</Trans>
        </EmptyState>
      ) : (
        <OnAirBoard stations={query.data} />
      )}
      {children}
    </section>
  );
}

/** A band of the page: a line across the column, then its heading at the page-title size. */
function Band({ id, children }: { id: BandId; children: ReactNode }) {
  const heading = useId();
  return (
    <section aria-labelledby={heading} className="mt-14 border-t border-line pt-6">
      <h2 id={heading} className="mb-5 text-page text-ink">
        {bandTitle(id)}
      </h2>
      {children}
    </section>
  );
}

/**
 * The frame of Apple's radio page while it loads: the local broadcasters'
 * heading and rows, then the first band's line and heading and its shelves.
 */
function BandsSkeleton() {
  const { t } = useLingui();
  return (
    <div role="group" aria-busy aria-label={t`Loading`}>
      <div className="mt-10">
        <div className="mb-3 flex h-7 items-center">
          <div className="h-4 w-40 rounded-sm bg-hover" />
        </div>
        <div className={STATION_GRID}>
          {Array.from({ length: 8 }, (_, i) => (
            <div key={i} className="flex h-16 items-center gap-3 px-2">
              <div className="size-12 rounded-sm bg-hover" />
              <div className="h-3.5 w-28 rounded-sm bg-hover" />
            </div>
          ))}
        </div>
      </div>
      <div className="mt-14 border-t border-line pt-6">
        <div className="mb-5 flex h-[30px] items-center">
          <div className="h-5 w-28 rounded-sm bg-hover" />
        </div>
        <div className="flex flex-col gap-10">
          {Array.from({ length: 2 }, (_, s) => (
            <div key={s}>
              <div className="mb-3 flex h-7 items-center">
                <div className="h-4 w-40 rounded-sm bg-hover" />
              </div>
              <div className="grid [grid-auto-rows:0] grid-cols-[repeat(auto-fill,minmax(160px,1fr))] [grid-template-rows:auto] gap-x-6 overflow-hidden">
                {Array.from({ length: 10 }, (_, i) => (
                  <div key={i}>
                    <div className="aspect-square rounded-sm bg-hover" />
                    <div className="mt-2 h-3.5 w-3/4 rounded-sm bg-hover" />
                    <div className="mt-1.5 h-3 w-1/2 rounded-sm bg-hover" />
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
