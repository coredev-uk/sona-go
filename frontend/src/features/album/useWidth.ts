import { useLayoutEffect, useState, type RefObject } from "react";

/**
 * An element's width in CSS pixels, read before the first paint and kept
 * current as the window, the sidebar or the queue panel change it. The
 * album page lays out by the content's width, not the window's.
 */
export function useWidth(ref: RefObject<HTMLElement | null>): number {
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.clientWidth);
    const ro = new ResizeObserver(([entry]) => setWidth(Math.round(entry.contentRect.width)));
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return width;
}
