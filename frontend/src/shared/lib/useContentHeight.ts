import { useLayoutEffect, type RefObject } from "react";

/**
 * Sets `--pane-h` on `ref` to the height of `main#content`, the shell's
 * scroll element, and keeps it current. A page whose list column is
 * `sticky top-0 h-(--pane-h)` stays as tall as the window while the rest
 * of the page scrolls beside it.
 */
export function useContentHeight(ref: RefObject<HTMLElement | null>) {
  useLayoutEffect(() => {
    const el = ref.current;
    const content = document.getElementById("content");
    if (!el || !content) return;
    const set = () => el.style.setProperty("--pane-h", `${content.clientHeight}px`);
    set();
    const ro = new ResizeObserver(set);
    ro.observe(content);
    return () => ro.disconnect();
  }, [ref]);
}
