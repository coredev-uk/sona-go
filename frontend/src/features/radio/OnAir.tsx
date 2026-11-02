import type { Station, StationDetail } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { CircleSlash, Play } from "lucide-react";

import { Artwork } from "@/shared/components/Artwork";
import { formatRunningTime } from "@/shared/lib/format";
import { useNowPlaying } from "@/shared/lib/now-playing";
import { useNow } from "@/shared/lib/useNow";
import { cn } from "@/shared/lib/utils";
import { Equaliser } from "@/shared/tracks/Equaliser";
import { Button } from "@/shared/ui/button";

import { playStation } from "./commands";
import { useStations } from "./queries";
import {
  nextProgramme,
  onAir,
  onAirSpan,
  programmeName,
  progressOf,
  slotLabel,
  startLabel,
} from "./schedule";

/** The board's grid: two columns at 1440, one at Sona's narrowest, three on a wide screen. */
const BOARD_GRID = "-mx-2 grid grid-cols-[repeat(auto-fill,minmax(420px,1fr))] gap-x-4 gap-y-1";
/** Each row's art: the programme's own (Apple's schedule art), else the station's. */
const ART = 80;
const ROW = "grid h-24 grid-cols-[80px_minmax(0,1fr)_32px] gap-x-4 rounded-md p-2";

/**
 * Apple Music's live stations as a now-and-next board: per station, the
 * programme on air with its art, its slot and how far through it is, and
 * the next programme with a name of its own. The schedules are the
 * stations' own pages (shared with them), read every 30 seconds, never a
 * ticking counter.
 */
export function OnAirBoard({ stations }: { stations: readonly Station[] }) {
  const details = useStations(stations.map((s) => s.id));
  const now = useNow(30_000);
  return (
    // Named by the section's "Live now" heading.
    <ul className={BOARD_GRID}>
      {stations.map((s, i) => (
        <OnAirRow key={s.id} station={s} detail={details[i]?.data} now={now} />
      ))}
    </ul>
  );
}

function OnAirRow({
  station: s,
  detail,
  now,
}: {
  station: Station;
  detail: StationDetail | undefined;
  now: number;
}) {
  const { t } = useLingui();
  const { current, playing } = useNowPlaying();
  const tuned = current?.id === s.id;
  const schedule = detail?.schedule ?? [];
  const programme = onAir(schedule, now);
  const what = programme ? programmeName(s, programme) || s.tagline : s.tagline;
  // The programme's stretch (the station's own hours as one run), counted
  // down to the next programme with a name.
  const span = onAirSpan(s, schedule, now);
  const next = nextProgramme(s, schedule, now);
  const nextAt = next ? startLabel(next, now) : "";
  const nextTitle = next?.title ?? "";
  const name = s.name;
  // "1 hr, 18 mins": to the minute, never under one.
  const left = span ? formatRunningTime(Math.max(60_000, span.endMs - now)) : "";
  const progress = span ? progressOf(span, now) : 0;
  return (
    <li
      className={cn(
        ROW,
        "group/live relative transition-colors duration-100 focus-within:bg-hover hover:bg-hover",
      )}
    >
      <Artwork src={programme?.art.medium || s.art.medium} size={ART} alt="" />
      <div className="min-w-0">
        <Link
          to="/station/$id"
          params={{ id: s.id }}
          className={cn(
            "truncate text-ui font-medium after:absolute after:inset-0",
            tuned ? "text-play-ink" : "text-ink",
          )}
        >
          {s.name}
        </Link>
        <p className="truncate text-sub text-ink">{what}</p>
        {!s.playable ? (
          <p className="mt-2 flex min-w-0 items-center gap-1.5 text-meta text-ink-muted">
            <CircleSlash aria-hidden className="size-3.5 shrink-0" strokeWidth={1.75} />
            <span className="truncate" title={s.unavailable}>
              {s.unavailable || <Trans>Sona can't play this station</Trans>}
            </span>
          </p>
        ) : (
          <>
            {span && (
              <div className="mt-2 flex items-center gap-3 text-meta text-ink-muted">
                <span className="shrink-0 num">{slotLabel(span, now)}</span>
                <div
                  role="progressbar"
                  aria-label={t`Time through this programme`}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(progress * 100)}
                  aria-valuetext={t`${left} left`}
                  className="h-0.5 min-w-8 flex-1 overflow-hidden rounded-full bg-active"
                >
                  <div
                    className={cn("h-full", tuned && playing ? "bg-play" : "bg-ink-muted")}
                    style={{ width: `${progress * 100}%` }}
                  />
                </div>
                <span className="shrink-0">
                  <Trans>{left} left</Trans>
                </span>
              </div>
            )}
            {next && (
              <p className={cn("truncate text-meta text-ink-muted", span ? "mt-0.5" : "mt-2")}>
                <Trans>
                  Next at <span className="num">{nextAt}</span>: {nextTitle}
                </Trans>
              </p>
            )}
          </>
        )}
      </div>
      <div className="relative z-10 grid h-8 place-items-center">
        {tuned ? (
          <Equaliser playing={playing} />
        ) : (
          s.playable && (
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t`Play ${name}`}
              onClick={(e) => {
                // The equaliser replaces this button once the station
                // plays: focus stays in the row, on the station's link.
                e.currentTarget.closest("li")?.querySelector("a")?.focus();
                void playStation(s.id);
              }}
              className="rounded-full text-ink-muted hover:text-ink"
            >
              <Play fill="currentColor" strokeWidth={0} className="translate-x-px" />
            </Button>
          )
        )}
      </div>
    </li>
  );
}

/** The board's frame while the live stations load: rows at their real height and art size. */
export function OnAirSkeleton() {
  const { t } = useLingui();
  return (
    <ul aria-busy aria-label={t`Loading`} className={BOARD_GRID}>
      {Array.from({ length: 6 }, (_, i) => (
        <li key={i} className={ROW}>
          <div className="size-20 rounded-sm bg-hover" />
          <div className="flex flex-col">
            <div className="mt-[3px] h-3.5 w-32 rounded-sm bg-hover" />
            <div className="mt-[7px] h-3 w-48 rounded-sm bg-hover" />
            <div className="mt-auto mb-0.5 h-3 w-full max-w-72 rounded-sm bg-hover" />
          </div>
        </li>
      ))}
    </ul>
  );
}
