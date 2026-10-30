import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useLayoutEffect, useState, type RefObject } from "react";

/**
 * A virtualizer over rows of fixed height inside `main#content`, the one
 * scroll element of the shell. `ref` is the list body; its offset from the
 * top of the scroll content is the virtualizer's scroll margin, re-measured
 * when the layout above it changes size. Pass `count` 0 to switch it off.
 *
 * Render from `items` and `totalSize`, never from `virtualizer` getters: the
 * virtualizer object keeps its identity while it scrolls, so React Compiler
 * would cache what a caller rendered from it. `items` changes identity only
 * when the visible range or the row positions change. Use `virtualizer` in
 * event handlers and effects (`scrollToIndex`).
 */
export function useContentVirtualizer(
  ref: RefObject<HTMLElement | null>,
  count: number,
  rowHeight: number,
  overscan = 8,
) {
  const [scrollEl, setScrollEl] = useState<HTMLElement | null>(null);
  const [margin, setMargin] = useState(0);

  useLayoutEffect(() => {
    const content = document.getElementById("content");
    const body = ref.current;
    setScrollEl(content);
    if (!content || !body) return;
    const measure = () =>
      setMargin(
        body.getBoundingClientRect().top - content.getBoundingClientRect().top + content.scrollTop,
      );
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(content.firstElementChild ?? content);
    return () => ro.disconnect();
  }, [ref]);

  // eslint-disable-next-line react-hooks/incompatible-library -- useVirtualizer is isolated in this hook on purpose; the compiler skips it
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => scrollEl,
    estimateSize: () => rowHeight,
    overscan,
    scrollMargin: margin,
  });
  // Row height can follow the window width (grids); re-measure on change.
  useEffect(() => virtualizer.measure(), [virtualizer, rowHeight]);

  return {
    virtualizer,
    items: virtualizer.getVirtualItems(),
    totalSize: virtualizer.getTotalSize(),
    margin,
  };
}
