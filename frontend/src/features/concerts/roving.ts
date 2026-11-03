import { useCallback, useLayoutEffect, useRef, type FocusEvent, type KeyboardEvent } from "react";

/**
 * Roving focus for a long agenda: every row (`[data-row]`) holds one or
 * two controls (`[data-stop]`: the artist's link, then the tickets), and
 * the whole list is one Tab stop. ↑ and ↓ move to the same control of the
 * row above or below, ← and → between a row's controls, Home and End to the
 * first and last row (a ticket menu's button keeps ↓ for moving: its menu opens with Enter or Space). Tab leaves the list; coming back lands on the control
 * focused last (docs/accessibility.md: long lists are one Tab stop).
 */

interface Position {
  row: number;
  col: number;
}

function layout(container: HTMLElement): HTMLElement[][] {
  return Array.from(container.querySelectorAll<HTMLElement>("[data-row]")).map((row) =>
    Array.from(row.querySelectorAll<HTMLElement>("[data-stop]")),
  );
}

function locate(rows: HTMLElement[][], target: EventTarget | null): Position | null {
  for (let row = 0; row < rows.length; row++) {
    const col = rows[row].indexOf(target as HTMLElement);
    if (col >= 0) return { row, col };
  }
  return null;
}

/** Where `key` moves from `at` among rows of `counts[i]` controls; null for other keys. */
export function moveFrom(key: string, at: Position, counts: readonly number[]): Position | null {
  const last = counts.length - 1;
  const clamp = (row: number, col: number): Position => ({
    row,
    col: Math.min(col, Math.max(counts[row] - 1, 0)),
  });
  switch (key) {
    case "ArrowDown":
      return at.row < last ? clamp(at.row + 1, at.col) : at;
    case "ArrowUp":
      return at.row > 0 ? clamp(at.row - 1, at.col) : at;
    case "ArrowRight":
      return { row: at.row, col: Math.min(at.col + 1, counts[at.row] - 1) };
    case "ArrowLeft":
      return { row: at.row, col: Math.max(at.col - 1, 0) };
    case "Home":
      return clamp(0, at.col);
    case "End":
      return clamp(last, at.col);
    default:
      return null;
  }
}

/**
 * Props for the list's container. `deps` are what changes its rows (the
 * shows listed); tab stops are set again after each.
 */
export function useRoving(deps: readonly unknown[]) {
  const ref = useRef<HTMLDivElement>(null);
  const current = useRef<Position>({ row: 0, col: 0 });

  const apply = useCallback(() => {
    const container = ref.current;
    if (!container) return;
    const rows = layout(container);
    const row = Math.min(current.current.row, rows.length - 1);
    const col = row >= 0 ? Math.min(current.current.col, rows[row].length - 1) : 0;
    rows.forEach((stops, r) =>
      stops.forEach((el, c) => el.setAttribute("tabindex", r === row && c === col ? "0" : "-1")),
    );
  }, []);

  useLayoutEffect(apply, [apply, ...deps]);

  const onFocus = useCallback(
    (e: FocusEvent<HTMLElement>) => {
      const rows = layout(e.currentTarget);
      const at = locate(rows, e.target);
      if (!at) return;
      current.current = at;
      apply();
    },
    [apply],
  );

  const onKeyDown = useCallback((e: KeyboardEvent<HTMLElement>) => {
    if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    const rows = layout(e.currentTarget);
    const at = locate(rows, e.target);
    if (!at) return;
    const to = moveFrom(
      e.key,
      at,
      rows.map((r) => r.length),
    );
    if (!to) return;
    e.preventDefault();
    rows[to.row]?.[to.col]?.focus();
  }, []);

  return { ref, onFocus, onKeyDown };
}
