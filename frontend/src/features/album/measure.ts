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

/**
 * Whether an element, held `margin` px below the top of the scrolling
 * `main`, fits in it whole: a sticky panel taller than the pane would hide
 * its foot until the page's end. `rendered` re-reads it when the element
 * comes and goes with the layout.
 */
export function useFitsInPane(
  ref: RefObject<HTMLElement | null>,
  margin: number,
  rendered: boolean,
): boolean {
  const [fits, setFits] = useState(false);
  useLayoutEffect(() => {
    const el = ref.current;
    const main = el?.closest("main");
    if (!rendered || !el || !main) return;
    const check = () => setFits(el.offsetHeight + margin * 2 <= main.clientHeight);
    const ro = new ResizeObserver(check);
    ro.observe(el);
    ro.observe(main);
    check();
    return () => ro.disconnect();
  }, [ref, margin, rendered]);
  return fits;
}
