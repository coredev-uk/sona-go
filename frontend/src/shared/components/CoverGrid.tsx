import { Link, type LinkProps } from "@tanstack/react-router";
import { Play } from "lucide-react";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";

import { FluidArtwork } from "@/shared/components/Artwork";
import { ExplicitMark } from "@/shared/components/Badges";
import { useWithheld } from "@/shared/lib/content-policy";
import { useContentVirtualizer } from "@/shared/lib/useContentVirtualizer";
import { cn } from "@/shared/lib/utils";

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
  /** The item's "more" menu (an ActionMenu), beside the title. */
  menu?: ReactNode;
  /** Marks the title explicit; restricted accounts see the tile dimmed and cannot play it. */
  explicit?: boolean;
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

export const GAP_X = 24;
const GAP_Y = 28;
/** Cover-to-title gap plus the 14/20 title and 13/18 subtitle lines. */
const TEXT_H = 8 + 20 + 18;
/** Grids longer than this are virtualised by row. */
const VIRTUAL_FROM = 200;

/**
 * The tile columns that fit the element's width: at least `min` wide,
 * GAP_X apart, growing to fill the row. `size` is the tile width rounded
 * up to 40px steps, so resizing the window reuses cached artwork sizes.
 */
export function useCoverColumns(ref: RefObject<HTMLElement | null>, min: number) {
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, [ref]);
  const cols = Math.max(1, Math.floor((width + GAP_X) / (min + GAP_X)));
  const tile = width ? (width - GAP_X * (cols - 1)) / cols : min;
  return { width, cols, tile, size: Math.ceil(tile / 40) * 40 };
}

/** One cover tile: the art, its title and a muted subtitle, linking to the item. */
export function CoverTile({
  item,
  size,
  aspect = "square",
}: {
  item: CoverItem | undefined;
  size: number;
  aspect?: "square" | "video";
}) {
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
  const title = (
    <span className="truncate group-hover/tile:underline group-hover/tile:underline-offset-2">
      {item.title}
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
        "group/tile group/row relative flex min-w-0 flex-col",
        withheld && "opacity-50",
      )}
      title={withheld ? "Explicit content is restricted on this account" : undefined}
    >
      <div className="relative">
        <FluidArtwork
          src={item.art}
          size={size}
          alt=""
          shape={item.shape}
          aspect={item.aspect ?? aspect}
        />
        {item.play && !withheld && (
          <button
            type="button"
            aria-label={`Play ${item.play.label}`}
            onClick={item.play.run}
            className="absolute bottom-2 left-2 z-10 grid size-9 place-items-center rounded-full bg-play text-on-play opacity-0 transition-opacity duration-100 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100 focus-visible:opacity-100"
          >
            <Play fill="currentColor" strokeWidth={0} className="size-4 translate-x-px" />
          </button>
        )}
      </div>
      <div className="mt-2 flex h-5 min-w-0 items-center gap-1">
        {item.link ? (
          <Link {...item.link} onClick={item.onOpen} draggable={false} className={opener}>
            {title}
          </Link>
        ) : (
          <button type="button" onClick={item.onOpen} className={opener}>
            {title}
          </button>
        )}
        {item.explicit && <ExplicitMark />}
        {item.menu && <div className="relative z-10 -my-1 ml-auto shrink-0">{item.menu}</div>}
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
}

/**
 * Album, playlist and artist tiles: covers from 160px, 24px across and 28px
 * down. Long grids are virtualised by row against `main#content`.
 */
export function CoverGrid({ items, label, onRange, min = 160, aspect = "square" }: CoverGridProps) {
  const ref = useRef<HTMLDivElement>(null);
  const { width, cols, tile, size } = useCoverColumns(ref, min);
  const rowH = (aspect === "video" ? (tile * 9) / 16 : tile) + TEXT_H + GAP_Y;
  const rows = Math.ceil(items.length / cols);
  const virtual = items.length > VIRTUAL_FROM && width > 0;
  const { virtualizer: v, margin } = useContentVirtualizer(ref, virtual ? rows : 0, rowH, 3);

  const vRows = virtual ? v.getVirtualItems() : [];
  const first = virtual ? (vRows[0]?.index ?? 0) * cols : 0;
  const last = virtual
    ? Math.min(items.length, ((vRows[vRows.length - 1]?.index ?? -1) + 1) * cols) - 1
    : items.length - 1;
  useEffect(() => {
    if (onRange && last >= first) onRange(first, last);
  }, [onRange, first, last]);

  const template = `repeat(${cols}, minmax(0, 1fr))`;

  return (
    <div ref={ref} role="list" aria-label={label}>
      {virtual ? (
        <div className="relative" style={{ height: v.getTotalSize() }}>
          {vRows.map((r) => (
            <div
              key={r.key}
              className="absolute inset-x-0 top-0 grid"
              style={{
                gridTemplateColumns: template,
                columnGap: GAP_X,
                transform: `translateY(${r.start - margin}px)`,
              }}
            >
              {items.slice(r.index * cols, r.index * cols + cols).map((item, i) => (
                <div role="listitem" key={item?.key ?? `loading-${r.index}-${i}`}>
                  <CoverTile item={item} size={size} aspect={aspect} />
                </div>
              ))}
            </div>
          ))}
        </div>
      ) : (
        <div
          className="grid"
          style={{ gridTemplateColumns: template, gap: `${GAP_Y}px ${GAP_X}px` }}
        >
          {items.map((item, i) => (
            <div role="listitem" key={item?.key ?? `loading-${i}`}>
              <CoverTile item={item} size={size} aspect={aspect} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
