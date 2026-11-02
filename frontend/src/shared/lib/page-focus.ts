/** Layers that take focus over the page: menus, dialogs and popovers. */
const LAYER = "[role=menu], [role=dialog], [role=alertdialog], [data-radix-popper-content-wrapper]";

let last: HTMLElement | null = null;

/**
 * Remembers the last element focused on the page itself (outside menus and
 * dialogs), so a dialog opened from a menu item can return focus there:
 * the menu item it would go back to is gone by the time the dialog closes.
 * Focus entering a menu counts as its trigger's (a row trigger is often
 * remounted as it opens, so the button that had focus is gone); a context
 * menu's hidden anchor does not count, the row before it stays.
 */
export function trackPageFocus(doc: Document = document): () => void {
  const onFocus = (e: FocusEvent) => {
    if (!(e.target instanceof HTMLElement)) return;
    const menu = e.target.closest("[role=menu]");
    if (menu) {
      const trigger = menu.id && doc.querySelector(`[aria-controls="${CSS.escape(menu.id)}"]`);
      if (trigger instanceof HTMLElement && trigger.getAttribute("aria-hidden") !== "true") {
        last = trigger;
      }
    } else if (!e.target.closest(LAYER)) {
      last = e.target;
    }
  };
  doc.addEventListener("focusin", onFocus, true);
  return () => doc.removeEventListener("focusin", onFocus, true);
}

/**
 * Where a closing dialog should send focus: `opener` (what was focused when
 * it opened) while it is still on the page or in the dialog under it, else
 * the last page element focused before it. Null when neither is focusable
 * any more.
 */
export function returnTarget(opener: Element | null): HTMLElement | null {
  const live = (el: Element | null): el is HTMLElement =>
    el instanceof HTMLElement && el.isConnected && !el.closest("[inert]");
  // A dialog opened from another dialog (a confirm over the setup) goes back
  // into it; a menu item is gone or closing, so it never counts.
  const POPUP = "[role=menu], [data-radix-popper-content-wrapper]";
  if (live(opener) && !opener.closest(POPUP)) return opener;
  return live(last) && !last.closest(LAYER) ? last : null;
}
