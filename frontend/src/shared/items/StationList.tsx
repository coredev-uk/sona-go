import type { Station } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Play } from "lucide-react";

import { Artwork } from "@/shared/components/Artwork";
import { useNowPlaying } from "@/shared/lib/now-playing";
import { cn } from "@/shared/lib/utils";
import { Equaliser } from "@/shared/tracks/Equaliser";
import { Button } from "@/shared/ui/button";

import { stationUnavailable } from "./station";

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
 * Live stations as a dense list: cover, name, tagline or provider and a
 * play control; the one playing carries the equaliser. Radio's "Live now"
 * and Apple's broadcaster sections share it.
 */
export function StationList({ stations, onPlay, label }: StationListProps) {
  const { t } = useLingui();
  const { current, playing } = useNowPlaying();
  return (
    <ul aria-label={label} className={STATION_GRID}>
      {stations.map((s) => {
        const on = current?.id === s.id;
        const name = s.name;
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
                className={cn(
                  "block truncate text-ui font-medium after:absolute after:inset-0",
                  on ? "text-play-ink" : "text-ink",
                )}
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
            )}
          </li>
        );
      })}
    </ul>
  );
}
