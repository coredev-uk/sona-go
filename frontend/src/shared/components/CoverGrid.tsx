import { useLingui } from "@lingui/react/macro";
import { Link, type LinkProps } from "@tanstack/react-router";
import { Play } from "lucide-react";
import { memo, useEffect, useRef, type KeyboardEvent } from "react";

import { ItemActionMenu } from "@/shared/actions/ActionMenu";
import type { ActionItem } from "@/shared/actions/registry";
import { FluidArtwork } from "@/shared/components/Artwork";
import { ExplicitMark } from "@/shared/components/Badges";
import { TileMotion } from "@/shared/components/TileMotion";
import { useWithheld } from "@/shared/lib/content-policy";
import { useContentVirtualizer } from "@/shared/lib/useContentVirtualizer";
import { GAP_X, useCoverColumns } from "@/shared/lib/useCoverColumns";
import { cn } from "@/shared/lib/utils";
import { SelectionContext, useRowSelected } from "@/shared/selection/store";
import { useItemList } from "@/shared/selection/useItemList";

export interface CoverItem {
  key: string;
  title: string;
  subtitle?: string;
  art?: string;
  shape?: "square" | "round";
  /** Music videos are 16:9 frames; everything else is a square cover. */
  aspect?: "square" | "video";
  /** The item's page; without one the tile calls `onOpen` (web-only items). */
  link?: Pick<LinkProps, "to" | "params" | "search">;
  /** Called when the tile is opened (after navigation starts, or instead of it). */
  onOpen?: () => void;
  /** A play control on the cover, for items that play directly. */
  play?: { label: string; run: () => void };
  /**
   * A short note on the cover's corner, where the play control would be:
   * an upcoming album's release date ("Out 19 Nov"). Also read after the
   * title by assistive technology.
   */
  badge?: string;
  /**
   * What the tile is, for its "more" menu, right-click, selection and drag
   * (songs, albums, playlists, artists, stations). Tiles without one (music
   * videos, web-only items) have no menu and cannot be selected.
   */
  target?: ActionItem;
  /** Marks the title explicit; restricted accounts see the tile dimmed and cannot play it. */
  explicit?: boolean;
  /** Go's square motion artwork URL, played on hover or focus ("" or unset: none). */
  motion?: string;
}

interface CoverGridProps {
  /** `undefined` marks a tile whose page is still loading. */
  items: readonly (CoverItem | undefined)[];
  label: string;
  /** Tiles that are on screen, for paged loading. */
  onRange?: (start: number, end: number) => void;
  /** Smallest tile width; tiles grow to fill the row. */
  min?: number;
  /** The tiles' art shape, for row heights and loading tiles (default square). */
  aspect?: "square" | "video";
}

const GAP_Y = 28;
/** Cover-to-title gap plus the 14/20 title and 13/18 subtitle lines. */
const TEXT_H = 8 + 20 + 18;
/** Grids longer than this are virtualised by row. */
const VIRTUAL_FROM = 200;

/**
 * One cover tile: the art, its title and a muted subtitle, linking to the
 * item. Memoised: a grid re-renders as its visible range moves, and tiles
 * whose item is unchanged skip that render. A selected tile sits on an ink
 * tinted plate, the same fill as a selected track row.
 */
export const CoverTile = memo(function CoverTile({
  item,
  size,
  aspect = "square",
  selected = false,
}: {
  item: CoverItem | undefined;
  size: number;
  aspect?: "square" | "video";
  selected?: boolean;
}) {
  const { t } = useLingui();
  const withheld = useWithheld(item?.explicit);
  if (!item) {
    return (
      <div aria-hidden className="flex flex-col">
        <div
          className={cn(
            "w-full rounded-sm bg-hover",
            aspect === "video" ? "aspect-video" : "aspect-square",
          )}
        />
        <div className="mt-2 h-3.5 w-3/4 rounded-sm bg-hover" />
        <div className="mt-1.5 h-3 w-1/2 rounded-sm bg-hover" />
      </div>
    );
  }
  const round = item.shape === "round";
  const playLabel = item.play?.label;
  const title = (
    <span className="truncate group-hover/tile:underline group-hover/tile:underline-offset-2">
      {item.title}
      {item.badge && <span className="sr-only">, {item.badge}</span>}
    </span>
  );
  // The title is the tile's one link; its ::after stretches over the whole
  // tile so the cover and subtitle open it too, under the play and menu
  // buttons.
  const opener = cn(
    "min-w-0 truncate rounded-sm text-left text-ui font-medium text-ink outline-offset-4 after:absolute after:inset-0 after:content-['']",
    // An explicit title keeps its width so the mark sits right after it.
    !item.explicit && "flex-1",
    round && "text-center",
  );
  return (
    <div
      className={cn(
        "group/tile group/row relative isolate flex min-w-0 flex-col",
        selected &&
          "before:absolute before:-inset-2 before:-z-10 before:rounded-lg before:bg-ink/12 before:content-['']",
        withheld && "opacity-50",
      )}
      title={withheld ? t`Explicit content is restricted on this account` : undefined}
      data-cover-tile
    >
      <div className="relative">
        <FluidArtwork
          src={item.art}
          size={size}
          alt=""
          shape={item.shape}
          aspect={item.aspect ?? aspect}
        />
        {item.motion && !withheld && <TileMotion motion={item.motion} round={round} />}
        {item.play && !withheld && (
          <button
            type="button"
            aria-label={t`Play ${playLabel}`}
            onClick={item.play.run}
            className="absolute bottom-2 left-2 z-10 grid size-9 place-items-center rounded-full bg-play text-on-play opacity-0 transition-opacity duration-100 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100 focus-visible:opacity-100"
          >
            <Play fill="currentColor" strokeWidth={0} className="size-4 translate-x-px" />
          </button>
        )}
        {item.badge && !item.play && (
          <span
            aria-hidden
            className="absolute bottom-2 left-2 rounded-md bg-raised px-1.5 py-0.5 text-meta font-medium text-ink num ring-1 ring-ink/10"
          >
            {item.badge}
          </span>
        )}
      </div>
      <div className="mt-2 flex h-5 min-w-0 items-center gap-1">
        {item.link ? (
          <Link
            {...item.link}
            onClick={item.onOpen}
            draggable={false}
            data-opener
            className={opener}
          >
            {title}
          </Link>
        ) : (
          <button type="button" onClick={item.onOpen} data-opener className={opener}>
            {title}
          </button>
        )}
        {item.explicit && <ExplicitMark />}
        {item.target && (
          <div className="relative z-10 -my-1 ml-auto shrink-0">
            <ItemActionMenu item={item.target} name={item.title} selectionKey={item.key} />
          </div>
        )}
      </div>
      {item.subtitle ? (
        <span className={`truncate text-sub text-ink-muted ${round ? "text-center" : ""}`}>
          {item.subtitle}
        </span>
      ) : (
        <span className="h-[18px]" />
      )}
    </div>
  );
});

/** A grid cell: the tile, its position for the grid's delegated handlers, and its selection. */
export const CoverCell = memo(function CoverCell({
  item,
  index,
  size,
  aspect,
}: {
  item: CoverItem | undefined;
  index: number;
  size: number;
  aspect: "square" | "video";
}) {
  const selected = useRowSelected(item?.target && item.key);
  return (
    <div role="gridcell" data-row={index} aria-selected={item?.target ? selected : undefined}>
      <CoverTile item={item} size={size} aspect={aspect} selected={selected} />
    </div>
  );
});

/**
 * Album, playlist and artist tiles: covers from 160px, 24px across and 28px
 * down. Long grids are virtualised by row against `main#content`.
 *
 * A click opens a tile; Ctrl+click and Shift+click (or Shift+arrows, Ctrl+A)
 * select tiles; arrows move between tiles; right-click or Shift+F10 opens
 * the tile's menu, which acts on the whole selection when the tile is in
 * it; tiles drag onto playlists, pins and the queue.
 */
export function CoverGrid({ items, label, onRange, min = 160, aspect = "square" }: CoverGridProps) {
  const ref = useRef<HTMLDivElement>(null);
  const { width, cols, tile } = useCoverColumns(ref, min);
  const rowH = (aspect === "video" ? (tile * 9) / 16 : tile) + TEXT_H + GAP_Y;
  const rows = Math.ceil(items.length / cols);
  const virtual = items.length > VIRTUAL_FROM && width > 0;
  const {
    virtualizer,
    items: vRows,
    totalSize,
    margin,
  } = useContentVirtualizer(ref, virtual ? rows : 0, rowH, 3);
  const list = useItemList({
    root: ref,
    keys: items.map((it) => (it?.target ? it.key : undefined)),
    itemAt: (i) => items[i]?.target,
    plainClick: "open",
  });

  const first = virtual ? (vRows[0]?.index ?? 0) * cols : 0;
  const last = virtual
    ? Math.min(items.length, ((vRows[vRows.length - 1]?.index ?? -1) + 1) * cols) - 1
    : items.length - 1;
  useEffect(() => {
    if (onRange && last >= first) onRange(first, last);
  }, [onRange, first, last]);

  const focusTile = (i: number) => {
    if (virtual) virtualizer.scrollToIndex(Math.floor(i / cols), { align: "auto" });
    // A virtual row may need a few frames to render after the scroll.
    const go = (tries: number) => {
      const el = ref.current?.querySelector<HTMLElement>(`[data-row="${i}"] [data-opener]`);
      if (el) el.focus({ preventScroll: virtual });
      else if (tries < 10) requestAnimationFrame(() => go(tries + 1));
    };
    requestAnimationFrame(() => go(0));
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const cell = e.target instanceof Element ? e.target.closest<HTMLElement>("[data-row]") : null;
    if (!cell || !(e.target instanceof HTMLElement) || !e.target.hasAttribute("data-opener"))
      return;
    const at = Number(cell.dataset.row);
    if (list.selectionKey(e, at)) return;
    const step = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -cols, ArrowDown: cols }[e.key];
    if (step === undefined || e.altKey) return;
    const to = at + step;
    if (to < 0 || to >= items.length) return;
    e.preventDefault();
    focusTile(to);
    list.arrowTo(e, to);
  };

  const cell = (item: CoverItem | undefined, i: number, fallbackKey: string) => (
    <CoverCell key={item?.key ?? fallbackKey} item={item} index={i} size={tile} aspect={aspect} />
  );

  return (
    <SelectionContext value={list.scope}>
      <div
        ref={ref}
        role="grid"
        aria-label={label}
        aria-multiselectable
        onKeyDown={onKeyDown}
        {...list.rootProps}
      >
        {virtual ? (
          <div className="relative" style={{ height: totalSize }}>
            {vRows.map((r) => (
              <div
                key={r.key}
                role="row"
                className="absolute inset-x-0 top-0 grid"
                style={{
                  gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
                  columnGap: GAP_X,
                  transform: `translateY(${r.start - margin}px)`,
                }}
              >
                {items
                  .slice(r.index * cols, r.index * cols + cols)
                  .map((item, i) => cell(item, r.index * cols + i, `loading-${r.index}-${i}`))}
              </div>
            ))}
          </div>
        ) : (
          <div
            className="grid"
            style={{
              gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
              gap: `${GAP_Y}px ${GAP_X}px`,
            }}
          >
            {Array.from({ length: rows }, (_, r) => (
              // Rows only group the cells for assistive tech; the CSS grid lays them out.
              <div key={r} role="row" className="contents">
                {items
                  .slice(r * cols, r * cols + cols)
                  .map((item, i) => cell(item, r * cols + i, `loading-${r * cols + i}`))}
              </div>
            ))}
          </div>
        )}
        {list.menu}
      </div>
    </SelectionContext>
  );
}
