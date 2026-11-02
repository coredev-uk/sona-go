import type { KeyboardEvent } from "react";

/**
 * Keeps Tab inside a view that covers the whole window (full-screen
 * lyrics, the video view): the rest of the window is underneath it.
 */
export function trapTab(e: KeyboardEvent<HTMLElement>) {
  if (e.key !== "Tab") return;
  const focusable = [
    ...e.currentTarget.querySelectorAll<HTMLElement>(
      "button:not([disabled]), a[href], [tabindex='0']",
    ),
  ];
  if (!focusable.length) return;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (
    e.shiftKey &&
    (document.activeElement === first || document.activeElement === e.currentTarget)
  ) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault();
    first.focus();
  }
}
