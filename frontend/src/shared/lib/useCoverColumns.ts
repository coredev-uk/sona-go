import { useLayoutEffect, useState, type RefObject } from "react";

/** The horizontal gap between cover tiles, in px. */
export const GAP_X = 24;

/** The element's width in CSS pixels, kept current as it resizes (0 before layout). */
export function useElementWidth(ref: RefObject<HTMLElement | null>) {
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, [ref]);
  return width;
}

/**
 * The tile columns that fit the element's width: at least `min` wide,
 * GAP_X apart, growing to fill the row. `tile` is the exact tile width,
 * which artwork is requested at (`artUrl`): WebKitGTK repaints the visible
 * page on every scroll step, and art decoded at another size is resampled
 * each time.
 */
export function useCoverColumns(ref: RefObject<HTMLElement | null>, min: number) {
  const width = useElementWidth(ref);
  const cols = Math.max(1, Math.floor((width + GAP_X) / (min + GAP_X)));
  const tile = width ? (width - GAP_X * (cols - 1)) / cols : min;
  return { width, cols, tile };
}
