/**
 * Keyboard movement through the search field's suggestion list. -1 means
 * no option is active (the caret is in the field): ↓ from there goes to the
 * first option, ↑ to the last, and moving past either end returns to the
 * field, so the list never traps the keyboard.
 */
export function moveActive(active: number, delta: 1 | -1, count: number): number {
  if (count <= 0) return -1;
  if (active < 0) return delta > 0 ? 0 : count - 1;
  const next = active + delta;
  return next < 0 || next >= count ? -1 : next;
}

/**
 * Splits `text` around the first case-insensitive occurrence of `typed`, so
 * a completion can set what the user typed apart from what it adds.
 */
export function splitMatch(
  text: string,
  typed: string,
): [before: string, match: string, after: string] {
  const t = typed.trim();
  const i = t ? text.toLowerCase().indexOf(t.toLowerCase()) : -1;
  if (i < 0) return ["", "", text];
  return [text.slice(0, i), text.slice(i, i + t.length), text.slice(i + t.length)];
}
