import { useLayoutEffect } from "react";

/**
 * Returns focus to whatever had it when `open` turned true, once `open`
 * turns false again (or the caller unmounts). For panels and overlays that
 * are not Radix dialogs (those restore focus themselves): the now-playing
 * view, full-screen lyrics, the queue.
 *
 * Focus goes back only if it was lost with the panel (it fell to <body>), so
 * a click elsewhere while the panel closes keeps its target. It waits a task,
 * so the panel's DOM is gone and the page is no longer `inert`.
 */
export function useReturnFocus(open: boolean): void {
  useLayoutEffect(() => {
    if (!open) return;
    // Layout effects run before the panel's own (passive) autofocus effects.
    const active = document.activeElement;
    const opener = active instanceof HTMLElement && active !== document.body ? active : null;
    if (!opener) return;
    return () => {
      setTimeout(() => {
        const now = document.activeElement;
        if ((now === null || now === document.body) && opener.isConnected) {
          opener.focus({ preventScroll: true });
        }
      }, 0);
    };
  }, [open]);
}
