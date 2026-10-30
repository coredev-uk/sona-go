import type { Item, Song } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Play } from "lucide-react";
import { useRef } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { Shelf } from "@/shared/components/Shelf";
import { useWithheld } from "@/shared/lib/content-policy";
import { isCurrent, useNowPlaying } from "@/shared/lib/now-playing";
import { useCoverColumns } from "@/shared/lib/useCoverColumns";
import { cn } from "@/shared/lib/utils";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { Equaliser } from "@/shared/tracks/Equaliser";
import { ExplicitMark } from "@/shared/components/Badges";

import type { ItemLinkProps } from "./links";
import { playSongList } from "./play";

interface TrackShelfProps {
  title: string;
  items: readonly Item[];
  more?: ItemLinkProps;
  /** Rows per column (default 4). */
  rows?: number;
}

/**
 * Songs as a compact block of rows that fills the width in columns of
 * `rows` songs, reading down each column. Double-click, Enter or the cover's
 * play button plays from that song with the shelf as the queue.
 */
export function TrackShelf({ title, items, more, rows = 4 }: TrackShelfProps) {
  const ref = useRef<HTMLDivElement>(null);
  const { cols } = useCoverColumns(ref, 280);
  const songs = items.flatMap((i) => (i.song ? [i.song] : []));
  const shown = songs.slice(0, cols * rows);
  const now = useNowPlaying();
  const play = (i: number) => playSongList(songs, i, title);
  return (
    <Shelf title={title} more={more}>
      <div
        ref={ref}
        role="list"
        aria-label={title}
        className="grid grid-flow-col gap-x-6"
        style={{
          gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${Math.min(rows, shown.length)}, auto)`,
        }}
      >
        {shown.map((song, i) => (
          <TrackRow
            key={`${i}:${song.id}`}
            song={song}
            current={isCurrent(song, now.current)}
            playing={now.playing}
            onPlay={() => play(i)}
          />
        ))}
      </div>
    </Shelf>
  );
}

function TrackRow({
  song,
  current,
  playing,
  onPlay,
}: {
  song: Song;
  current: boolean;
  playing: boolean;
  onPlay: () => void;
}) {
  const { t } = useLingui();
  const withheld = useWithheld(song.explicit);
  const disabled = !song.playable || withheld;
  const name = song.name;
  return (
    <div
      role="listitem"
      tabIndex={0}
      aria-disabled={disabled || undefined}
      title={withheld ? t`Explicit content is restricted on this account` : undefined}
      onDoubleClick={() => !disabled && onPlay()}
      onKeyDown={(e) => {
        if (e.key === "Enter" && e.target === e.currentTarget && !disabled) onPlay();
      }}
      className={cn(
        "group/row flex h-14 min-w-0 items-center gap-3 rounded-md px-2 outline-offset-[-2px] transition-colors duration-100 hover:bg-hover",
        disabled && "opacity-50",
      )}
    >
      <div className="relative shrink-0">
        <Artwork src={song.art.small} size={40} alt="" />
        {current ? (
          <span className="absolute inset-0 grid place-items-center rounded-sm bg-ground/70">
            <Equaliser playing={playing} />
          </span>
        ) : (
          !disabled && (
            <button
              type="button"
              tabIndex={-1}
              aria-label={t`Play ${name}`}
              onClick={onPlay}
              className="absolute inset-0 grid place-items-center rounded-sm bg-ground/70 text-ink opacity-0 transition-opacity duration-100 group-hover/row:opacity-100"
            >
              <Play fill="currentColor" strokeWidth={0} className="size-4" />
            </button>
          )
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-1.5">
          <span
            className={cn("truncate text-ui font-medium", current ? "text-play-ink" : "text-ink")}
          >
            {song.name}
          </span>
          {song.explicit && <ExplicitMark />}
        </div>
        <div className="truncate text-sub text-ink-muted">
          {song.artistId ? (
            <Link
              to="/artist/$id"
              params={{ id: song.artistId }}
              tabIndex={-1}
              className="underline-offset-2 hover:text-ink hover:underline"
            >
              {song.artistName}
            </Link>
          ) : (
            song.artistName
          )}
        </div>
      </div>
      <ActionMenu kind="song" target={song} name={song.name} />
    </div>
  );
}
