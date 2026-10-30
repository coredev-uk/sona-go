import { Link } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Clock3, Play } from "lucide-react";
import {
  memo,
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";

import { Artwork } from "@/shared/components/Artwork";
import { ExplicitMark } from "@/shared/components/Badges";
import type { Song } from "@bindings/services";
import { formatTime } from "@/shared/lib/format";
import { useContentVirtualizer } from "@/shared/lib/useContentVirtualizer";
import { cn } from "@/shared/lib/utils";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import type { ActionContainer } from "@/shared/actions/registry";
import { isCurrent, useNowPlaying } from "@/shared/lib/now-playing";
import { useContentPolicy, useWithheld } from "@/shared/lib/content-policy";

import { Equaliser } from "./Equaliser";

export type SongSort = "title" | "artist" | "album" | "dateAdded";

export interface TrackColumns {
  /** "track": the album's track numbers; "row": 1…n in list order. */
  number: "track" | "row";
  artwork?: boolean;
  artist?: boolean;
  album?: boolean;
}

interface TrackListProps {
  /** Every row of the list; `undefined` marks a row whose page is still loading. */
  songs: readonly (Song | undefined)[];
  columns: TrackColumns;
  /** Accessible name of the list, e.g. "Songs in OK Computer". */
  label: string;
  /** Plays from row `index`, with the whole list as the queue. */
  onPlay: (index: number) => void;
  sort?: { key: SongSort; desc: boolean; onSort: (key: SongSort) => void };
  /** Group album tracks under "Disc n" when there is more than one disc. */
  discs?: boolean;
  /** A song to mark and scroll to (from a shared link). */
  highlight?: string;
  /** Catalogue IDs of the album's most played songs, marked "Popular" after the title. */
  popular?: ReadonlySet<string>;
  /** Rows that are on screen, for paged loading. */
  onRange?: (start: number, end: number) => void;
  /** The collection the songs belong to, for row actions (see ActionContainer). */
  container?: Omit<ActionContainer, "index">;
}

const ROW = 44;
/** Lists longer than this are virtualised. */
const VIRTUAL_FROM = 200;

function gridTemplate(c: TrackColumns): string {
  return [
    "40px",
    "minmax(0,2fr)",
    c.artist && "minmax(0,1fr)",
    c.album && "minmax(0,1fr)",
    "56px",
    "32px",
  ]
    .filter(Boolean)
    .join(" ");
}

function HeaderCell({
  label,
  sortKey,
  sort,
  className,
}: {
  label: string;
  sortKey?: SongSort;
  sort?: TrackListProps["sort"];
  className?: string;
}) {
  if (!sortKey || !sort) {
    return (
      <span role="columnheader" className={className}>
        {label}
      </span>
    );
  }
  const active = sort.key === sortKey;
  const Arrow = sort.desc ? ArrowDown : ArrowUp;
  return (
    <span
      role="columnheader"
      aria-sort={active ? (sort.desc ? "descending" : "ascending") : "none"}
      className={className}
    >
      <button
        type="button"
        onClick={() => sort.onSort(sortKey)}
        className={cn(
          "-mx-1 inline-flex h-6 items-center gap-1 rounded-sm px-1 transition-colors duration-100 hover:text-ink",
          active && "text-ink",
        )}
      >
        {label}
        {active && <Arrow aria-hidden className="size-3" strokeWidth={2} />}
      </button>
    </span>
  );
}

interface RowProps {
  song: Song | undefined;
  index: number;
  columns: TrackColumns;
  template: string;
  current: boolean;
  playing: boolean;
  highlighted: boolean;
  focused: boolean;
  popular: boolean;
  onPlay: (index: number) => void;
  onFocusRow: (index: number) => void;
  container?: Omit<ActionContainer, "index">;
}

const TrackRow = memo(function TrackRow({
  song,
  index,
  columns,
  template,
  current,
  playing,
  highlighted,
  popular,
  focused,
  onPlay,
  onFocusRow,
  container,
}: RowProps) {
  // Restricted accounts see explicit songs greyed out and cannot play them.
  const withheld = useWithheld(song?.explicit);
  if (!song) {
    return (
      <div
        role="row"
        aria-rowindex={index + 2}
        aria-busy
        className="grid h-11 items-center gap-4 px-2"
        style={{ gridTemplateColumns: template }}
      >
        <span />
        <span className="flex items-center gap-3">
          {columns.artwork && <span className="size-8 shrink-0 rounded-sm bg-hover" />}
          <span className="h-3.5 w-2/5 rounded-sm bg-hover" />
        </span>
        {columns.artist && <span className="h-3 w-1/2 rounded-sm bg-hover" />}
        {columns.album && <span className="h-3 w-1/2 rounded-sm bg-hover" />}
        <span />
        <span />
      </div>
    );
  }
  const number = columns.number === "track" ? song.trackNumber || index + 1 : index + 1;
  const disabled = !song.playable || withheld;
  return (
    <div
      role="row"
      aria-rowindex={index + 2}
      aria-selected={current || undefined}
      aria-disabled={disabled || undefined}
      data-row={index}
      tabIndex={focused ? 0 : -1}
      title={withheld ? "Explicit content is restricted on this account" : undefined}
      onFocus={(e) => e.target === e.currentTarget && onFocusRow(index)}
      onDoubleClick={() => !disabled && onPlay(index)}
      className={cn(
        "group/row grid h-11 cursor-default items-center gap-4 rounded-md px-2 transition-colors duration-100 select-none hover:bg-hover focus-visible:bg-hover focus-visible:outline-offset-[-2px]",
        highlighted && "bg-active",
        disabled && "text-ink-faint",
      )}
      style={{ gridTemplateColumns: template }}
    >
      <span role="gridcell" className="relative flex h-full items-center justify-end">
        {current ? (
          <Equaliser playing={playing} className="mr-1" />
        ) : (
          <span className="text-sub text-ink-faint num group-focus-within/row:invisible group-hover/row:invisible">
            {number}
          </span>
        )}
        {!disabled && !current && (
          <button
            type="button"
            tabIndex={-1}
            aria-label={`Play ${song.name}`}
            onClick={() => onPlay(index)}
            className="invisible absolute inset-y-0 right-0 grid w-6 place-items-center text-ink group-focus-within/row:visible group-hover/row:visible"
          >
            <Play aria-hidden className="size-3.5" fill="currentColor" strokeWidth={0} />
          </button>
        )}
      </span>
      <span role="gridcell" className="flex min-w-0 items-center gap-3">
        {columns.artwork && <Artwork src={song.art.small} size={32} alt="" />}
        <span className="min-w-0">
          <span className="flex min-w-0 items-center gap-1.5">
            <span
              className={cn(
                "truncate text-ui",
                current ? "font-medium text-play-ink" : disabled ? "text-ink-faint" : "text-ink",
              )}
            >
              {song.name}
            </span>
            {song.explicit && <ExplicitMark />}
            {popular && <span className="shrink-0 text-meta text-ink-faint">Popular</span>}
          </span>
          {/* Without an artist column the artist sits under the title. */}
          {!columns.artist && columns.number === "row" && (
            <span className="block truncate text-sub text-ink-muted">{song.artistName}</span>
          )}
        </span>
      </span>
      {columns.artist && (
        <span role="gridcell" className="truncate text-sub text-ink-muted">
          <EntityLink id={song.artistId} kind="artist">
            {song.artistName}
          </EntityLink>
        </span>
      )}
      {columns.album && (
        <span role="gridcell" className="truncate text-sub text-ink-muted">
          <EntityLink id={song.albumId} kind="album">
            {song.albumName}
          </EntityLink>
        </span>
      )}
      <span role="gridcell" className="text-right text-sub text-ink-faint num">
        {song.durationMs ? formatTime(song.durationMs) : "—"}
      </span>
      <span role="gridcell" className="flex justify-end">
        <ActionMenu
          kind="song"
          target={song}
          name={song.name}
          container={container && { ...container, index }}
        />
      </span>
    </div>
  );
});

function EntityLink({
  id,
  kind,
  children,
}: {
  id: string;
  kind: "artist" | "album";
  children: ReactNode;
}) {
  if (!id) return <>{children}</>;
  return (
    <Link
      to={kind === "artist" ? "/artist/$id" : "/album/$id"}
      params={{ id }}
      tabIndex={-1}
      className="underline-offset-2 hover:text-ink hover:underline"
    >
      {children}
    </Link>
  );
}

/**
 * A track table: fixed 44px rows, no dividers, the playing track marked in
 * place. Double-click or Enter plays from that row with the list as the
 * queue. Lists over 200 rows are virtualised against `main#content`.
 */
export function TrackList({
  songs,
  columns,
  label,
  onPlay,
  sort,
  discs,
  highlight,
  onRange,
  popular,
  container,
}: TrackListProps) {
  const { current, playing } = useNowPlaying();
  const restricted = useContentPolicy((s) => s.restricted);
  const template = gridTemplate(columns);
  // A shared link's song is marked and takes the initial keyboard focus.
  const highlightIndex = highlight
    ? songs.findIndex((s) => s && (s.id === highlight || s.catalogId === highlight))
    : -1;
  const [focusState, setFocus] = useState<number | null>(null);
  const focus = focusState ?? Math.max(0, highlightIndex);
  const bodyRef = useRef<HTMLDivElement>(null);
  const virtual = songs.length > VIRTUAL_FROM;
  const { virtualizer: v, margin } = useContentVirtualizer(
    bodyRef,
    virtual ? songs.length : 0,
    ROW,
    12,
  );
  const virtualizer = virtual ? v : undefined;

  const multiDisc =
    discs && songs.some((s, i) => i > 0 && s && songs[0] && s.discNumber !== songs[0].discNumber);

  const play = useCallback((i: number) => onPlay(i), [onPlay]);

  const moveFocus = (next: number) => {
    const i = Math.max(0, Math.min(songs.length - 1, next));
    setFocus(i);
    virtualizer?.scrollToIndex(i, { align: "auto" });
    // A virtual row may need a few frames to render after the scroll.
    let tries = 0;
    const focusRow = () => {
      const el = bodyRef.current?.querySelector<HTMLElement>(`[data-row="${i}"]`);
      if (el) el.focus({ preventScroll: !!virtualizer });
      else if (tries++ < 10) requestAnimationFrame(focusRow);
    };
    requestAnimationFrame(focusRow);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.target instanceof HTMLElement && e.target.closest("button, a, input")) {
      if (e.target.getAttribute("role") !== "row") return;
    }
    const page = Math.max(1, Math.floor(window.innerHeight / ROW) - 2);
    switch (e.key) {
      case "ArrowDown":
        moveFocus(focus + 1);
        break;
      case "ArrowUp":
        moveFocus(focus - 1);
        break;
      case "PageDown":
        moveFocus(focus + page);
        break;
      case "PageUp":
        moveFocus(focus - page);
        break;
      case "Home":
        moveFocus(0);
        break;
      case "End":
        moveFocus(songs.length - 1);
        break;
      case "Enter": {
        const s = songs[focus];
        if (s?.playable && !(restricted && s.explicit)) onPlay(focus);
        break;
      }
      default:
        return;
    }
    e.preventDefault();
  };

  const items = virtualizer?.getVirtualItems() ?? [];
  const first = items[0]?.index ?? 0;
  const last = items[items.length - 1]?.index ?? -1;

  useEffect(() => {
    if (!onRange) return;
    if (virtual) {
      if (last >= 0) onRange(first, last);
    } else if (songs.length) onRange(0, songs.length - 1);
  }, [onRange, virtual, first, last, songs.length]);

  // Scroll the shared link's song into view once it is loaded.
  useEffect(() => {
    if (highlightIndex < 0) return;
    if (virtual) v.scrollToIndex(highlightIndex, { align: "center" });
    else
      bodyRef.current
        ?.querySelector(`[data-row="${highlightIndex}"]`)
        ?.scrollIntoView({ block: "center" });
  }, [highlightIndex, virtual, v]);

  const row = (song: Song | undefined, i: number) => (
    <TrackRow
      song={song}
      index={i}
      columns={columns}
      template={template}
      current={!!song && isCurrent(song, current)}
      playing={playing}
      highlighted={i === highlightIndex}
      popular={!!song?.catalogId && !!popular?.has(song.catalogId)}
      focused={i === focus}
      onPlay={play}
      onFocusRow={setFocus}
      container={container}
    />
  );

  return (
    <div role="grid" aria-label={label} aria-rowcount={songs.length + 1} onKeyDown={onKeyDown}>
      <div
        role="row"
        aria-rowindex={1}
        className="grid h-8 items-center gap-4 border-b border-line px-2 text-meta text-ink-faint"
        style={{ gridTemplateColumns: template }}
      >
        <span role="columnheader" className="text-right">
          #
        </span>
        <HeaderCell label="Title" sortKey={sort && "title"} sort={sort} />
        {columns.artist && <HeaderCell label="Artist" sortKey={sort && "artist"} sort={sort} />}
        {columns.album && <HeaderCell label="Album" sortKey={sort && "album"} sort={sort} />}
        <span role="columnheader" className="flex justify-end">
          <Clock3 aria-label="Duration" className="size-3.5" strokeWidth={1.75} />
        </span>
        <span role="columnheader" aria-label="Actions" />
      </div>
      <div ref={bodyRef} className="pt-1">
        {virtualizer ? (
          <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
            {items.map((it) => (
              <div
                key={it.key}
                className="absolute inset-x-0 top-0"
                style={{ transform: `translateY(${it.start - margin}px)` }}
              >
                {row(songs[it.index], it.index)}
              </div>
            ))}
          </div>
        ) : (
          songs.map((song, i) => (
            <div key={song?.id ?? `loading-${i}`}>
              {multiDisc && song && (i === 0 || songs[i - 1]?.discNumber !== song.discNumber) && (
                <h3 className="px-2 pt-5 pb-2 text-sub font-semibold text-ink-muted first:pt-2">
                  Disc {song.discNumber}
                </h3>
              )}
              {row(song, i)}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
