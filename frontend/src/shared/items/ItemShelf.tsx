import type { Item } from "@bindings/services";
import { useState } from "react";

import { CoverRow } from "@/shared/components/CoverRow";
import { Shelf } from "@/shared/components/Shelf";

import { rowAspect, toCoverItem } from "./cover";
import type { ItemLinkProps } from "./links";

interface ItemShelfProps {
  title: string;
  items: readonly Item[];
  /** The section's own "See all" page; without one, extra items open in place. */
  more?: ItemLinkProps;
  /** Smallest tile width (default 160; feature shelves use larger tiles). */
  min?: number;
  /** Rows shown before "Show all" (default 1). */
  rows?: number;
  onOpen?: (item: Item) => void;
  className?: string;
}

/**
 * A titled row of item tiles: as many as fit, never scrolling sideways.
 * A section with its own page links "See all" there; otherwise the rest
 * of its items open in place.
 */
export function ItemShelf({
  title,
  items,
  more,
  min,
  rows = 1,
  onOpen,
  className,
}: ItemShelfProps) {
  const [cols, setCols] = useState(0);
  const [open, setOpen] = useState(false);
  const hidden = cols > 0 && items.length > cols * rows;
  return (
    <Shelf
      title={title}
      more={more}
      className={className}
      actions={
        !more && (hidden || open) ? (
          <button
            type="button"
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
            className="-mr-1 inline-flex h-7 items-center rounded-md px-1.5 text-sub text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink"
          >
            {open ? "Show fewer" : `Show all ${items.length}`}
          </button>
        ) : undefined
      }
    >
      <CoverRow
        items={items.map((it, i) => toCoverItem(it, i, onOpen))}
        label={title || "Items"}
        min={min}
        aspect={rowAspect(items)}
        rows={open ? Math.ceil(items.length / Math.max(cols, 1)) : rows}
        onColumns={setCols}
      />
    </Shelf>
  );
}
