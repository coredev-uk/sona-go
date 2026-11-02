import type { LyricsProvider } from "@bindings/config";

/** The list with the provider at `from` moved one place up (-1) or down (1); unchanged at an end. */
export function moveProvider(list: readonly LyricsProvider[], from: number, dir: -1 | 1) {
  const to = from + dir;
  if (to < 0 || to >= list.length) return [...list];
  const next = [...list];
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}
