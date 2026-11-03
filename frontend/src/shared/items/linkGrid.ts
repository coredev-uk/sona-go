/** The smallest a link button gets, in px: room for "Moods and Activities" on one line, "From Around the World" on two. */
export const LINK_MIN = 184;
/** The gap between link buttons, both ways, in px. */
export const LINK_GAP = 8;

/**
 * How a list of `count` links sets in a row `width` px wide: as many columns
 * as fit at LINK_MIN, filled top to bottom, so an alphabetical list (Apple's
 * genres) reads down each column and Tab follows the eye. `cols` is the
 * tracks the row is cut into (a short list leaves the last ones empty, so
 * its buttons stay as wide as the list's above or below it); `rows` is what
 * the fullest column holds. The columns are filled so the last one holds at
 * least half a column: a lone button stranded at the top of the last column
 * reads as a mistake, so the column is made taller until it has company.
 */
export function linkGrid(width: number, count: number) {
  const cols = Math.max(1, Math.floor((width + LINK_GAP) / (LINK_MIN + LINK_GAP)));
  let rows = Math.max(1, Math.ceil(count / cols));
  while (rows < count) {
    const last = count - (Math.ceil(count / rows) - 1) * rows;
    if (last >= rows / 2) break;
    rows++;
  }
  return { cols, rows };
}
