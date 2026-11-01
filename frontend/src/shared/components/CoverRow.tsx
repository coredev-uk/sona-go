import { useEffect, useRef } from "react";

import { CoverCell, type CoverItem } from "@/shared/components/CoverGrid";
import { GAP_X, useCoverColumns } from "@/shared/lib/useCoverColumns";
import { SelectionContext } from "@/shared/selection/store";
import { useItemList } from "@/shared/selection/useItemList";

interface CoverRowProps {
  items: readonly CoverItem[];
  label: string;
  /** Smallest tile width; tiles grow to fill the row (default 160). */
  min?: number;
  aspect?: "square" | "video";
  /** How many rows of tiles to show (default 1); the rest are left out. */
  rows?: number;
  /** Reports how many tiles fit in one row, e.g. to offer "Show all". */
  onColumns?: (cols: number) => void;
}

/**
 * A shelf's tiles: as many cover tiles as fit in `rows` rows at the grid's
 * tile size, never scrolling sideways. Pair it with a Shelf's "See all".
 * Tiles select, open their menu on right-click and drag like CoverGrid's.
 */
export function CoverRow({
  items,
  label,
  min = 160,
  aspect = "square",
  rows = 1,
  onColumns,
}: CoverRowProps) {
  const ref = useRef<HTMLDivElement>(null);
  const { cols, tile } = useCoverColumns(ref, min);
  useEffect(() => onColumns?.(cols), [onColumns, cols]);
  const shown = items.slice(0, cols * rows);
  const list = useItemList({
    root: ref,
    keys: shown.map((it) => (it.target ? it.key : undefined)),
    itemAt: (i) => shown[i]?.target,
    plainClick: "open",
  });
  return (
    <SelectionContext value={list.scope}>
      <div
        ref={ref}
        role="grid"
        aria-label={label}
        aria-multiselectable
        className="grid"
        style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, gap: `28px ${GAP_X}px` }}
        onKeyDown={(e) => {
          const cell =
            e.target instanceof Element ? e.target.closest<HTMLElement>("[data-row]") : null;
          if (cell) list.selectionKey(e, Number(cell.dataset.row));
        }}
        {...list.rootProps}
      >
        {/* One row for assistive tech; the CSS grid lays the cells out. */}
        <div role="row" className="contents">
          {shown.map((item, i) => (
            <CoverCell key={item.key} item={item} index={i} size={tile} aspect={aspect} />
          ))}
        </div>
        {list.menu}
      </div>
    </SelectionContext>
  );
}
