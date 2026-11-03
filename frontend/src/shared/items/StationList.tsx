import { PlaybackService, type Station } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Play } from "lucide-react";

import { ItemActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { useNowPlaying } from "@/shared/lib/now-playing";
import { cn } from "@/shared/lib/utils";
import { Equaliser } from "@/shared/tracks/Equaliser";
import { Button } from "@/shared/ui/button";

import { onAirLine, stationUnavailable } from "./station";

/** The station list's grid; loading placeholders share it. */
export const STATION_GRID =
  "grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-x-4 gap-y-1 -mx-2";

interface StationListProps {
  stations: readonly Station[];
  onPlay: (station: Station) => void;
  /** Names the list when no heading labels it. */
  label?: string;
}

/**
 * Live stations as a dense list: cover, name, what is on air (while the
 * station plays and its stream names it) or its tagline or provider, and
 * a play control; the one playing carries the equaliser. Radio's "Live
 * now" and Apple's broadcaster sections share it.
 *
 * A station with a page (`hasPage`) links to it and has a play button. A
 * station without one (a local broadcaster has no schedule) is itself the
 * play control: its name is a button that plays it (and pauses it while
 * it plays), and its menu is beside it.
 */
export function StationList({ stations, onPlay, label }: StationListProps) {
  const { t } = useLingui();
  const { current, playing, onAir } = useNowPlaying();
  return (
    <ul aria-label={label} className={STATION_GRID}>
      {stations.map((s) => {
        const on = current?.id === s.id;
        const name = s.name;
        const line = on && onAir ? onAirLine(onAir) : s.tagline || s.provider;
        const nameClass = cn(
          "block max-w-full truncate text-left text-ui font-medium after:absolute after:inset-0",
          on ? "text-play-ink" : "text-ink",
        );
        return (
          <li
            key={s.id}
            className="group/live group/row relative flex h-16 items-center gap-3 rounded-md px-2 transition-colors duration-100 focus-within:bg-hover hover:bg-hover"
          >
            <Artwork src={s.art.small} size={48} alt="" />
            <div className="min-w-0 flex-1">
              {s.hasPage ? (
                <Link to="/station/$id" params={{ id: s.id }} className={nameClass}>
                  {s.name}
                </Link>
              ) : (
                <button
                  type="button"
                  disabled={!s.playable}
                  title={s.playable ? undefined : stationUnavailable(s.unavailable)}
                  onClick={() => (on ? void PlaybackService.Toggle() : onPlay(s))}
                  className={cn(nameClass, "rounded-sm")}
                >
                  {s.name}
                  {/* Named by its text, with the action after it (docs/accessibility.md). */}{" "}
                  <span className="sr-only">{on && playing ? t`Pause` : t`Play`}</span>
                </button>
              )}
              <p className="truncate text-sub text-ink-muted">{line}</p>
            </div>
            {s.hasPage ? (
              on ? (
                <Equaliser playing={playing} className="mr-2.5" />
              ) : (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  disabled={!s.playable}
                  title={s.playable ? undefined : stationUnavailable(s.unavailable)}
                  aria-label={t`Play ${name}`}
                  onClick={(e) => {
                    // The equaliser replaces this button once the station
                    // plays: focus stays in the row, on the station's link.
                    e.currentTarget.closest("li")?.querySelector("a")?.focus();
                    onPlay(s);
                  }}
                  className="relative z-10 rounded-full text-ink-muted opacity-0 group-focus-within/live:opacity-100 group-hover/live:opacity-100 hover:text-ink focus-visible:opacity-100"
                >
                  <Play fill="currentColor" strokeWidth={0} className="translate-x-px" />
                </Button>
              )
            ) : (
              <>
                {on ? (
                  <Equaliser playing={playing} className="mr-1" />
                ) : (
                  s.playable && (
                    <Play
                      aria-hidden
                      fill="currentColor"
                      strokeWidth={0}
                      className="mr-1 size-4 shrink-0 translate-x-px text-ink-muted opacity-0 group-focus-within/live:opacity-100 group-hover/live:opacity-100"
                    />
                  )
                )}
                <div className="relative z-10 shrink-0">
                  <ItemActionMenu item={{ kind: "station", target: s }} name={s.name} />
                </div>
              </>
            )}
          </li>
        );
      })}
    </ul>
  );
}
