import type { Song } from "@bindings/services";

/** One disc of a multi-disc album: where its songs start in the list, how many, how long. */
export interface Disc {
  number: number;
  /** Index of its first song in the album's list. */
  first: number;
  songs: number;
  durationMs: number;
}

/**
 * The album's discs in list order, or none when the songs sit on one disc
 * (a single disc needs no index).
 */
export function discsOf(songs: readonly Song[]): Disc[] {
  const discs: Disc[] = [];
  songs.forEach((s, i) => {
    const last = discs[discs.length - 1];
    if (last && last.number === s.discNumber) {
      last.songs++;
      last.durationMs += s.durationMs;
    } else {
      discs.push({ number: s.discNumber, first: i, songs: 1, durationMs: s.durationMs });
    }
  });
  return discs.length > 1 ? discs : [];
}

/**
 * Whether the songs are credited to many artists (a soundtrack, a
 * compilation): then the list gets an Artist column. An album with a
 * featured artist on a few songs names them under those titles instead.
 * A song with no credit yet (unpublished, on an album not out) counts for
 * neither.
 */
export function isCompilation(songs: readonly Song[], albumArtist: string): boolean {
  const credited = songs.filter((s) => s.artistName);
  const others = credited.filter((s) => s.artistName !== albumArtist).length;
  return others * 2 > credited.length;
}

/**
 * The people in Apple's composer credits ("Taylor Swift, Jack Antonoff &
 * Zoë Kravitz"), each once, most credited first (ties in album order).
 */
export function writersOf(songs: readonly Song[]): string[] {
  const count = new Map<string, number>();
  for (const s of songs) {
    for (const name of s.composerName.split(/\s*(?:,|&)\s*/)) {
      const n = name.trim();
      if (n) count.set(n, (count.get(n) ?? 0) + 1);
    }
  }
  // Map keeps first-seen order, and the sort is stable.
  return [...count.entries()].sort((a, b) => b[1] - a[1]).map(([n]) => n);
}

/**
 * A classical album: Apple names the work its tracks belong to. Its rows
 * are grouped under their works and read by movement.
 */
export function hasWorks(songs: readonly Song[]): boolean {
  return songs.some((s) => s.workName);
}

/**
 * The heading a work's first track carries on a classical album: the work,
 * led by its composer when the album has more than one ("Johann Sebastian
 * Bach · Goldberg Variations"). A track outside any work gets none.
 */
export function workHeading(
  song: Song,
  prev: Song | undefined,
  composers: number,
): string | undefined {
  if (!song.workName) return undefined;
  if (prev && prev.workName === song.workName && prev.discNumber === song.discNumber)
    return undefined;
  return composers > 1 && song.composerName
    ? `${song.composerName} · ${song.workName}`
    : song.workName;
}

/** A track's title under its work's heading: the movement, when Apple names one. */
export function movementTitle(song: Song): string {
  return song.workName && song.movementName ? song.movementName : song.name;
}

/**
 * The year the album's oldest recording first came out, when that is
 * before the album's own year (a reissue, a compilation); "" otherwise.
 */
export function firstReleased(songs: readonly Song[], albumDate: string): string {
  const album = albumDate.slice(0, 4);
  let first = "";
  for (const s of songs) {
    const y = s.releaseDate.slice(0, 4);
    if (/^\d{4}$/.test(y) && (!first || y < first)) first = y;
  }
  return first && album && first < album ? first : "";
}

/**
 * An album not out yet lists songs Apple hasn't published as placeholders
 * ("Track 3", no duration) credited to an artist it doesn't know yet. Their
 * credit is dropped so the rows don't repeat Apple's stand-in name.
 */
export function withoutPlaceholderCredits(songs: Song[], upcoming: boolean): Song[] {
  if (!upcoming) return songs;
  return songs.map((s) => (!s.playable && !s.durationMs ? { ...s, artistName: "" } : s));
}

/**
 * A library album holding only some of its songs: how many it has of the
 * catalogue album's total. Undefined when it holds them all, or when the
 * catalogue album isn't known.
 */
export function libraryShare(
  have: number,
  catalogue: number | undefined,
): { have: number; total: number } | undefined {
  return catalogue && catalogue > have ? { have, total: catalogue } : undefined;
}
