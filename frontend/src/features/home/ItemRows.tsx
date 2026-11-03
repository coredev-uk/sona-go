import type { Item } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Play } from "lucide-react";
import { memo, useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

import { ItemActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { ExplicitMark } from "@/shared/components/Badges";
import { openWithoutPage, playsWhenOpened } from "@/shared/items/cover";
import { actionItemOf, itemLink, itemShape, itemSubtitle } from "@/shared/items/links";
import { canPlay, playItem } from "@/shared/items/play";
import { useWithheld } from "@/shared/lib/content-policy";
import { useCoverColumns } from "@/shared/lib/useCoverColumns";
import { cn } from "@/shared/lib/utils";
import { SelectionContext, useRowSelected } from "@/shared/selection/store";
import { useItemList } from "@/shared/selection/useItemList";

interface ItemRowsProps {
  items: readonly Item[];
  label: string;
  /** Rows shown until `open`. */
  rows: number;
  /** Show every item. */
  open?: boolean;
  /** Smallest column width (default 264). */
  min?: number;
  /** Reports the columns that fit, e.g. to offer "Show all". */
  onColumns?: (cols: number) => void;
  /** The rows sit on a cover's backdrop: hover tints with ink, not the opaque --hover fill. */
  onBackdrop?: boolean;
}

const rowKey = (item: Item, i: number) => `${i}:${item.kind}:${item.id || item.url}`;

/**
 * Albums, playlists and stations as compact rows (40px art, title, Apple's
 * reason and the artist or curator) in as many columns as fit, reading
 * across. One Tab stop: the arrow keys, Home and End move between rows,
 * Enter opens, Space plays, Shift+F10 or the Menu key opens the row's
 * menu. Rows select, open their menu on right-click and drag like cover
 * tiles.
 */
export function ItemRows({
  items,
  label,
  rows,
  open = false,
  min = 264,
  onColumns,
  onBackdrop = false,
}: ItemRowsProps) {
  const { t } = useLingui();
  const ref = useRef<HTMLDivElement>(null);
  const hint = useId();
  const { cols } = useCoverColumns(ref, min);
  useEffect(() => onColumns?.(cols), [onColumns, cols]);
  const shown = open ? items : items.slice(0, cols * rows);
  const keys = shown.map((it, i) => (actionItemOf(it) ? rowKey(it, i) : undefined));
  const list = useItemList({
    root: ref,
    keys,
    itemAt: (i) => (shown[i] ? actionItemOf(shown[i]) : undefined),
    plainClick: "open",
  });
  // Roving focus: the last row focused is the list's one Tab stop.
  const [active, setActive] = useState(0);
  const current = Math.min(active, shown.length - 1);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const cell = e.target instanceof Element ? e.target.closest<HTMLElement>("[data-row]") : null;
    if (!cell || !(e.target instanceof HTMLElement) || !e.target.hasAttribute("data-opener"))
      return;
    const at = Number(cell.dataset.row);
    if (list.selectionKey(e, at)) return;
    if (e.altKey) return;
    const to =
      e.key === "Home"
        ? 0
        : e.key === "End"
          ? shown.length - 1
          : at + ({ ArrowLeft: -1, ArrowRight: 1, ArrowUp: -cols, ArrowDown: cols }[e.key] ?? NaN);
    if (!(to >= 0 && to < shown.length)) return;
    e.preventDefault();
    ref.current?.querySelector<HTMLElement>(`[data-row="${to}"] [data-opener]`)?.focus();
    list.arrowTo(e, to);
  };

  return (
    <SelectionContext value={list.scope}>
      <div
        ref={ref}
        role="grid"
        aria-label={label}
        aria-multiselectable
        aria-describedby={hint}
        className="grid gap-x-6"
        style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
        onKeyDown={onKeyDown}
        onFocus={(e) => {
          const cell =
            e.target instanceof Element ? e.target.closest<HTMLElement>("[data-row]") : null;
          if (cell) setActive(Number(cell.dataset.row));
        }}
        {...list.rootProps}
      >
        {Array.from({ length: Math.ceil(shown.length / cols) }, (_, r) => (
          // Rows only group the cells for assistive tech; the CSS grid lays them out.
          <div key={r} role="row" className="contents">
            {shown.slice(r * cols, r * cols + cols).map((item, c) => {
              const i = r * cols + c;
              return (
                <ItemRow
                  key={rowKey(item, i)}
                  item={item}
                  index={i}
                  selectionKey={keys[i]}
                  tabbable={i === current}
                  onBackdrop={onBackdrop}
                />
              );
            })}
          </div>
        ))}
        {list.menu}
      </div>
      <p id={hint} hidden>
        {t`Enter opens, Space plays, Shift+F10 shows more actions.`}
      </p>
    </SelectionContext>
  );
}

const ItemRow = memo(function ItemRow({
  item,
  index,
  selectionKey,
  tabbable,
  onBackdrop,
}: {
  item: Item;
  index: number;
  selectionKey: string | undefined;
  tabbable: boolean;
  onBackdrop: boolean;
}) {
  const { t } = useLingui();
  const withheld = useWithheld(item.explicit);
  const selected = useRowSelected(selectionKey);
  const link = itemLink(item);
  const target = actionItemOf(item);
  const playable = canPlay(item) && !withheld;
  // A station with no page plays when it is opened: the name is that
  // button, and the cover's play mark is not a second control.
  const direct = playable && playsWhenOpened(item);
  const round = itemShape(item) === "round";
  const subtitle = itemSubtitle(item);
  const name = item.name;
  // The title is the row's one link; its ::after stretches over the row so
  // the art and the line under it open it too, under the play and menu buttons.
  const opener = {
    "data-opener": true,
    tabIndex: tabbable ? 0 : -1,
    draggable: false,
    onKeyDown: (e: KeyboardEvent) => {
      if (e.key !== " ") return;
      e.preventDefault();
      if (playable) playItem(item);
    },
    className:
      "min-w-0 truncate text-left text-ui font-medium text-ink after:absolute after:inset-0 after:content-[''] focus-visible:outline-none group-hover/row:underline group-hover/row:underline-offset-2",
  };
  return (
    <div
      role="gridcell"
      data-row={index}
      aria-selected={target ? selected : undefined}
      title={withheld ? t`Explicit content is restricted on this account` : undefined}
      className={cn(
        "group/row relative isolate flex h-14 min-w-0 items-center gap-3 rounded-md px-2 outline-offset-[-2px] transition-colors duration-100 has-[[data-opener]:focus-visible]:outline-2 has-[[data-opener]:focus-visible]:outline-ring",
        onBackdrop ? "hover:bg-ink/6" : "hover:bg-hover",
        selected && "bg-ink/12 hover:bg-ink/12",
        withheld && "opacity-50",
      )}
    >
      <div className="relative shrink-0">
        <Artwork src={item.art.small || item.art.medium} size={40} alt="" shape={itemShape(item)} />
        {direct && (
          <span
            aria-hidden
            className={cn(
              "pointer-events-none absolute inset-0 z-10 grid place-items-center bg-ground/70 text-ink opacity-0 transition-opacity duration-100 group-hover/row:opacity-100",
              round ? "rounded-full" : "rounded-sm",
            )}
          >
            <Play fill="currentColor" strokeWidth={0} className="size-4 translate-x-px" />
          </span>
        )}
        {playable && !direct && (
          <button
            type="button"
            tabIndex={-1}
            aria-label={t`Play ${name}`}
            onClick={() => playItem(item)}
            className={cn(
              "absolute inset-0 z-10 grid place-items-center bg-ground/70 text-ink opacity-0 transition-opacity duration-100 group-hover/row:opacity-100",
              round ? "rounded-full" : "rounded-sm",
            )}
          >
            <Play fill="currentColor" strokeWidth={0} className="size-4 translate-x-px" />
          </button>
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-1">
          {link ? (
            <Link {...link} {...opener}>
              {name}
            </Link>
          ) : (
            <button type="button" onClick={() => openWithoutPage(item)} {...opener}>
              {name}
              {direct && (
                <>
                  {" "}
                  <span className="sr-only">{t`Play`}</span>
                </>
              )}
            </button>
          )}
          {item.explicit && <ExplicitMark />}
        </div>
        {subtitle && <p className="truncate text-sub text-ink-muted">{subtitle}</p>}
      </div>
      {target && (
        <div className="relative z-10 shrink-0">
          <ItemActionMenu item={target} name={name} selectionKey={selectionKey} />
        </div>
      )}
    </div>
  );
});
