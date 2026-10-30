import type { Item } from "@bindings/services";

import { CoverGrid } from "@/shared/components/CoverGrid";

import { rowAspect, toCoverItem } from "./cover";

interface ItemGridProps {
  /** `undefined` marks a tile whose page is still loading. */
  items: readonly (Item | undefined)[];
  label: string;
  onRange?: (start: number, end: number) => void;
  onOpen?: (item: Item) => void;
  min?: number;
}

/** Every item as a tile grid (rooms, "See all" pages, charts); virtualised when long. */
export function ItemGrid({ items, label, onRange, onOpen, min }: ItemGridProps) {
  const loaded = items.filter((i): i is Item => !!i);
  return (
    <CoverGrid
      items={items.map((it, i) => (it ? toCoverItem(it, i, onOpen) : undefined))}
      label={label}
      onRange={onRange}
      min={min}
      aspect={rowAspect(loaded)}
    />
  );
}
