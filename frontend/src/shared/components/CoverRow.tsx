import { useEffect, useRef } from "react";

import { CoverTile, GAP_X, useCoverColumns, type CoverItem } from "@/shared/components/CoverGrid";

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
  const { cols, size } = useCoverColumns(ref, min);
  useEffect(() => onColumns?.(cols), [onColumns, cols]);
  const shown = items.slice(0, cols * rows);
  return (
    <div
      ref={ref}
      role="list"
      aria-label={label}
      className="grid"
      style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, gap: `28px ${GAP_X}px` }}
    >
      {shown.map((item) => (
        <div role="listitem" key={item.key}>
          <CoverTile item={item} size={size} aspect={aspect} />
        </div>
      ))}
    </div>
  );
}
