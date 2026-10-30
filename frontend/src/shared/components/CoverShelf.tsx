import { useState } from "react";

import type { CoverItem } from "@/shared/components/CoverGrid";
import { CoverRow } from "@/shared/components/CoverRow";
import { Shelf, ShelfToggle } from "@/shared/components/Shelf";

interface CoverShelfProps {
  title: string;
  items: CoverItem[];
  aspect?: "square" | "video";
  /** "See all" page; without one, "Show all" opens the rest in place. */
  more?: Parameters<typeof Shelf>[0]["more"];
}

/**
 * A titled shelf of cover tiles: one row that fits the width, with "See
 * all" to the section's page or "Show all" in place when more were sent
 * than fit. Renders nothing when there are no items.
 */
export function CoverShelf({ title, items, aspect = "square", more }: CoverShelfProps) {
  const [open, setOpen] = useState(false);
  const [cols, setCols] = useState(Infinity);
  if (!items.length) return null;
  return (
    <Shelf
      title={title}
      more={more}
      actions={
        !more &&
        items.length > cols && (
          <ShelfToggle open={open} onToggle={() => setOpen(!open)}>
            {open ? "Show fewer" : "Show all"}
          </ShelfToggle>
        )
      }
    >
      <CoverRow
        items={items}
        label={title}
        aspect={aspect}
        rows={open ? Infinity : 1}
        min={aspect === "video" ? 240 : 160}
        onColumns={setCols}
      />
    </Shelf>
  );
}
