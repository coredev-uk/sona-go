/**
 * The queue list's lines: the current song and the user's songs, then the
 * station's or AutoPlay's part under its own heading, then (while AutoPlay
 * is still on its way) a note that it will continue.
 */
export type Line = { kind: "row"; qi: number } | { kind: "head" } | { kind: "note" };

export interface QueueLines {
  lines: Line[];
  /** The user's songs after the current one. */
  mine: number;
  /** The station's or AutoPlay's songs after the current one. */
  auto: number;
  /** Where the user's part ends: the first upcoming station slot, else the end. */
  userEnd: number;
  /** The heading's line, -1 without one; the rows after it are the station's part. */
  head: number;
}

/**
 * Lines for `entries` from `start` (the current song). The station's part
 * gets a heading when it follows songs the user queued, and always for
 * AutoPlay (it is not what the user chose). `note` adds the AutoPlay note.
 */
export function queueLines(
  entries: readonly { auto: boolean }[],
  start: number,
  station: { autoPlay: boolean } | null,
  note: boolean,
): QueueLines {
  const lines: Line[] = [];
  let mine = 0;
  let auto = 0;
  let userEnd = entries.length;
  let head = -1;
  for (let qi = start; qi < entries.length; qi++) {
    const upcoming = qi > start;
    if (upcoming && entries[qi].auto && auto === 0) {
      userEnd = qi;
      if (station && (station.autoPlay || mine > 0)) {
        head = lines.length;
        lines.push({ kind: "head" });
      }
    }
    if (upcoming) {
      if (entries[qi].auto) auto++;
      else mine++;
    }
    lines.push({ kind: "row", qi });
  }
  if (note) lines.push({ kind: "note" });
  return { lines, mine, auto, userEnd, head };
}
