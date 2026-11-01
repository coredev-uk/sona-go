/**
 * The queue list's roving focus: one row is the list's Tab stop, the arrows
 * move it. Rows are the queue entries from `start` (the current song) to the
 * end; queue indices shift as songs are moved, removed or played, so the
 * focused row is remembered by its entry's uid, with its last index for when
 * the entry is gone.
 */
export interface QueueFocus {
  uid: number;
  qi: number;
}

/**
 * The queue index of the focused row: the remembered entry where it still
 * is, else the row now at its old place (the next one after a removal),
 * else the current song. -1 when the list has no rows.
 */
export function focusedRow(
  entries: readonly { uid: number }[],
  start: number,
  focus: QueueFocus | null,
): number {
  if (start >= entries.length) return -1;
  if (focus) {
    const at = entries.findIndex((e) => e.uid === focus.uid);
    if (at >= start) return at;
  }
  return Math.max(start, Math.min(entries.length - 1, focus?.qi ?? start));
}

/** Where Up, Down, Home and End move focus from row `qi`; null for other keys. */
export function rowForKey(key: string, qi: number, start: number, end: number): number | null {
  switch (key) {
    case "ArrowDown":
      return Math.min(end - 1, qi + 1);
    case "ArrowUp":
      return Math.max(start, qi - 1);
    case "Home":
      return start;
    case "End":
      return end - 1;
    default:
      return null;
  }
}

/**
 * Where Alt+Up or Alt+Down moves the user's song at `qi`: one step, staying
 * before the station's or AutoPlay's part (which starts at `userEnd`); null
 * when it can't move that way or for other keys.
 */
export function moveForKey(key: string, qi: number, userEnd: number): number | null {
  const to = key === "ArrowUp" ? qi - 1 : key === "ArrowDown" ? qi + 1 : -1;
  return to >= 0 && to < userEnd ? to : null;
}
