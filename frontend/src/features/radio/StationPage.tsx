import type { StationDetail, StationEvent } from "@bindings/services";
import { getRouteApi, Link } from "@tanstack/react-router";
import { CircleSlash } from "lucide-react";
import { useEffect, useState } from "react";

import { ActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { useNowPlaying } from "@/shared/lib/now-playing";
import { cn } from "@/shared/lib/utils";

import { playStation } from "./commands";
import { useStation } from "./queries";
import { onAir, progressOf, slotLabel, stationLine } from "./schedule";

const route = getRouteApi("/station/$id");

/**
 * A station: what it is and who runs it, play, and for a live station the
 * schedule from the programme on air. A station Sona cannot play says why
 * where the play control would be.
 */
export function StationPage() {
  const { id } = route.useParams();
  const q = useStation(id);
  if (q.isPending) {
    return (
      <div aria-busy aria-label="Loading" className="flex items-end gap-7 pt-1 pb-7">
        <div className="size-[200px] rounded-sm bg-hover" />
        <div className="flex-1 pb-1">
          <div className="h-8 w-80 rounded-sm bg-hover" />
          <div className="mt-3 h-3 w-48 rounded-sm bg-hover" />
          <div className="mt-6 size-10 rounded-full bg-hover" />
        </div>
      </div>
    );
  }
  if (q.isError) {
    return (
      <ErrorBlock
        title="Couldn't load this station"
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  }
  return <Station detail={q.data} />;
}

function Station({ detail }: { detail: StationDetail }) {
  const s = detail.station;
  const byline = s.showId ? (
    <Link
      to="/curator/$id"
      params={{ id: s.showId }}
      className="hover:underline hover:underline-offset-2"
    >
      {s.showName}
    </Link>
  ) : s.hostName ? (
    s.hostName
  ) : (
    s.tagline || undefined
  );
  return (
    <>
      <CollectionHeader
        title={s.name}
        art={s.art.large}
        byline={byline}
        meta={stationLine(s)}
        description={s.notes || undefined}
        header={detail.header}
        actions={
          <>
            <PlayButtons
              label={s.name}
              disabled={!s.playable}
              onPlay={() => void playStation(s.id)}
            />
            <ActionMenu kind="station" target={s} name={s.name} variant="header" />
          </>
        }
      />
      {!s.playable && (
        <div
          role="note"
          className="mb-8 flex max-w-[72ch] items-start gap-3 rounded-lg border border-dashed border-control-line px-4 py-3.5"
        >
          <CircleSlash
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-ink-muted"
            strokeWidth={1.75}
          />
          <div>
            <p className="text-ui font-medium text-ink">Sona can't play this station</p>
            <p className="mt-0.5 text-sub text-ink-muted">{s.unavailable}</p>
          </div>
        </div>
      )}
      {detail.schedule.length > 0 && <Schedule stationId={s.id} events={detail.schedule} />}
    </>
  );
}

/** Re-renders every `ms` so the on-air marker follows the clock. */
function useNow(ms: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), ms);
    return () => window.clearInterval(t);
  }, [ms]);
  return now;
}

/**
 * The live schedule as a timetable: time on the left, the programme on
 * the right; the one on air shows how far through its slot it is (amber
 * only while Sona is playing this station).
 */
function Schedule({ stationId, events }: { stationId: string; events: StationEvent[] }) {
  const now = useNow(30_000);
  const { current, playing } = useNowPlaying();
  const live = onAir(events, now);
  const upcoming = events.filter((e) => e.endMs > now);
  const tuned = current?.id === stationId && playing;
  return (
    <section aria-labelledby="station-schedule" className="max-w-[880px]">
      <h2 id="station-schedule" className="mb-2 text-heading text-ink">
        Schedule
      </h2>
      <ol>
        {upcoming.map((e) => {
          const isOn = e === live;
          return (
            <li
              key={e.id}
              aria-current={isOn ? "time" : undefined}
              className="grid grid-cols-[112px_48px_minmax(0,1fr)] items-start gap-4 border-t border-line py-3.5 first:border-t-0"
            >
              <div className="pt-0.5">
                <p
                  className={cn("text-meta num", isOn ? "font-medium text-ink" : "text-ink-muted")}
                >
                  {slotLabel(e, now)}
                </p>
                {isOn && <p className="mt-0.5 text-meta text-ink-muted">On air</p>}
              </div>
              <Artwork src={e.art.small} size={48} alt="" />
              <div className="min-w-0">
                <p
                  className={cn(
                    "text-ui font-medium",
                    isOn && tuned ? "text-play-ink" : "text-ink",
                  )}
                >
                  {e.title}
                </p>
                {e.description && (
                  <p className="mt-0.5 line-clamp-2 max-w-[72ch] text-sub text-ink-muted">
                    {e.description}
                  </p>
                )}
                {isOn && (
                  <div
                    role="progressbar"
                    aria-label="Time through this programme"
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={Math.round(progressOf(e, now) * 100)}
                    className="mt-2.5 h-0.5 w-full max-w-80 overflow-hidden rounded-full bg-active"
                  >
                    <div
                      className={cn("h-full", tuned ? "bg-play" : "bg-ink-muted")}
                      style={{ width: `${progressOf(e, now) * 100}%` }}
                    />
                  </div>
                )}
              </div>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
