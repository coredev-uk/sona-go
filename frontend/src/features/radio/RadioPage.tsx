import type { Station } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import type { UseQueryResult } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Play } from "lucide-react";

import { hasAccount, useAuth } from "@/features/auth";
import { Artwork } from "@/shared/components/Artwork";
import type { CoverItem } from "@/shared/components/CoverGrid";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { EditorialSections } from "@/shared/items/EditorialSections";
import { useNowPlaying } from "@/shared/lib/now-playing";
import { Equaliser } from "@/shared/tracks/Equaliser";
import { Button } from "@/shared/ui/button";

import { playStation } from "./commands";
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
    play: s.playable ? { label: playLabel(s.name), run: () => void playStation(s.id) } : undefined,
    target: { kind: "station", target: s },
  };
}

function playLabel(name: string): string {
  return t`Play ${name}`;
}

/**
 * Radio: Apple Music's live stations first (they are on air now), the
 * user's own recent stations, then Apple's editorial radio page (shows,
 * episodes, genre stations).
 */
export function RadioPage() {
  const { t } = useLingui();
  const auth = useAuth();
  const signedIn = hasAccount(auth.data?.state);
  const live = useLiveStations();
  const recent = useRecentStations(signedIn);
  const grouping = useRadioGrouping();

  // The editorial page repeats the live stations in its own shelf; the
  // list above already shows them.
  const sections = (grouping.data?.sections ?? []).filter(
    (s) =>
      !(
        live.data?.length &&
        s.items.length > 0 &&
        s.items.every((i) => i.station?.kind === "live")
      ),
  );

  return (
    <>
      <PageHeader title={t`Radio`} />
      <OnAir query={live} />
      {signedIn && recent.data && recent.data.length > 0 && (
        <div className="mt-10">
          <CoverShelf title={t`Your stations`} items={recent.data.map(stationCover)} />
        </div>
      )}
      <div className="mt-10 flex flex-col gap-10">
        {grouping.isPending ? (
          <div aria-busy aria-label={t`Loading`} className="flex flex-col gap-3">
            <div className="h-4 w-40 rounded-sm bg-hover" />
            <div className="grid grid-cols-[repeat(auto-fill,minmax(160px,1fr))] gap-x-6">
              {Array.from({ length: 6 }, (_, i) => (
                <div key={i} className="aspect-square rounded-sm bg-hover" />
              ))}
            </div>
          </div>
        ) : grouping.isError ? (
          <ErrorBlock
            title={t`Couldn't load Apple Music's radio page`}
            error={grouping.error}
            onRetry={() => void grouping.refetch()}
            retrying={grouping.isRefetching}
          />
        ) : (
          <EditorialSections sections={sections} />
        )}
      </div>
    </>
  );
}

/**
 * Apple Music's live stations as a dense list: cover, name, tagline and a
 * play control; the one playing carries the equaliser.
 */
function OnAir({ query }: { query: UseQueryResult<Station[]> }) {
  const { t } = useLingui();
  const { current, playing } = useNowPlaying();
  return (
    <section aria-labelledby="radio-live">
      <h2 id="radio-live" className="mb-3 text-heading text-ink">
        <Trans>Live now</Trans>
      </h2>
      {query.isPending ? (
        <ul aria-busy aria-label={t`Loading`} className={LIVE_GRID}>
          {Array.from({ length: 6 }, (_, i) => (
            <li key={i} className="flex h-16 items-center gap-3 px-2">
              <div className="size-12 rounded-sm bg-hover" />
              <div className="flex-1">
                <div className="h-3.5 w-1/2 rounded-sm bg-hover" />
                <div className="mt-1.5 h-3 w-2/3 rounded-sm bg-hover" />
              </div>
            </li>
          ))}
        </ul>
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
        <ul className={LIVE_GRID}>
          {query.data.map((s) => {
            const on = current?.id === s.id;
            return (
              <li
                key={s.id}
                className="group/live relative flex h-16 items-center gap-3 rounded-md px-2 transition-colors duration-100 hover:bg-hover"
              >
                <Artwork src={s.art.small} size={48} alt="" />
                <div className="min-w-0 flex-1">
                  <Link
                    to="/station/$id"
                    params={{ id: s.id }}
                    className={
                      "block truncate text-ui font-medium after:absolute after:inset-0 " +
                      (on ? "text-play-ink" : "text-ink")
                    }
                  >
                    {s.name}
                  </Link>
                  <p className="truncate text-sub text-ink-muted">{s.tagline || s.provider}</p>
                </div>
                {on ? (
                  <Equaliser playing={playing} className="mr-2.5" />
                ) : (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    disabled={!s.playable}
                    title={s.playable ? undefined : s.unavailable}
                    aria-label={playLabel(s.name)}
                    onClick={() => void playStation(s.id)}
                    className="relative z-10 rounded-full text-ink-muted opacity-0 group-focus-within/live:opacity-100 group-hover/live:opacity-100 hover:text-ink focus-visible:opacity-100"
                  >
                    <Play fill="currentColor" strokeWidth={0} className="translate-x-px" />
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

const LIVE_GRID = "grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-x-4 gap-y-1 -mx-2";
