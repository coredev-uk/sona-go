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
 */
export function isCompilation(songs: readonly Song[], albumArtist: string): boolean {
  const others = songs.filter((s) => s.artistName !== albumArtist).length;
  return others * 2 > songs.length;
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
