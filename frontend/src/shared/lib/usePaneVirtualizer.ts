import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect } from "react";

/**
 * A virtualizer over a list that scrolls inside its own box (`scrollEl`, from
 * a callback ref so it follows the box when it mounts later), for
 * panes beside the page's own scroll (the Artists list). Rows have a known
 * height by index (`sizeAt`), so nothing is measured. Pass a new `sizeAt`
 * when the rows change.
 *
 * Render from `items` and `totalSize`, never from `virtualizer` getters, as
 * for `useContentVirtualizer`; use `virtualizer` in handlers and effects
 * (`scrollToIndex`).
 */
export function usePaneVirtualizer(
  scrollEl: HTMLElement | null,
  count: number,
  sizeAt: (index: number) => number,
  overscan = 8,
) {
  // eslint-disable-next-line react-hooks/incompatible-library -- useVirtualizer is isolated in this hook on purpose; the compiler skips it
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => scrollEl,
    estimateSize: sizeAt,
    overscan,
  });
  useEffect(() => virtualizer.measure(), [virtualizer, sizeAt]);

  return {
    virtualizer,
    items: virtualizer.getVirtualItems(),
    totalSize: virtualizer.getTotalSize(),
  };
}
