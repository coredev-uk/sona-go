import { useLayoutEffect, useState, type RefObject } from "react";

/** The horizontal gap between cover tiles, in px. */
export const GAP_X = 24;

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
