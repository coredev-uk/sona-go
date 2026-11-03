import type { ArtistEntry } from "@bindings/services";
import { plural, t } from "@lingui/core/macro";

/** A library artist ready to list and filter: its folded name and the letter it files under. */
export interface IndexedArtist {
  entry: ArtistEntry;
  /** The name folded (lower case, no accents), what the filter matches. */
  key: string;
  /** `key` without spaces and punctuation, so "acdc" finds "AC/DC". */
  loose: string;
  /** "A"–"Z", or "#" for digits, symbols and other scripts. */
  letter: string;
}

/** Letters NFD doesn't split into a base and a mark. */
const SPLIT: Record<string, string> = {
  ß: "ss",
  æ: "ae",
  œ: "oe",
  ø: "o",
  đ: "d",
  ð: "d",
  ł: "l",
  þ: "th",
  ı: "i",
};

/** `text` in lower case without accents: "Édith Piaf" → "edith piaf". */
export function fold(text: string): string {
  return text
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{M}+/gu, "")
    .replace(/[ßæœøđðłþı]/g, (c) => SPLIT[c])
    .trim();
}

const loosen = (key: string) => key.replace(/[^\p{L}\p{N}]+/gu, "");

const collator = new Intl.Collator(undefined, { numeric: true });

const OTHERS = "#";

function letterOf(key: string): string {
  const first = key[0];
  return first && first >= "a" && first <= "z" ? first.toUpperCase() : OTHERS;
}

/** Letters sort A–Z, then "#" for everything else, as music libraries do. */
const rank = (letter: string) => (letter === OTHERS ? 26 : letter.charCodeAt(0) - 65);

/** Sorts a library's artists by name, accents and case aside, and files each under its letter. */
export function indexArtists(entries: readonly ArtistEntry[]): IndexedArtist[] {
  return entries
    .map((entry) => {
      const key = fold(entry.name);
      return { entry, key, loose: loosen(key), letter: letterOf(key) };
    })
    .sort(
      (a, b) =>
        rank(a.letter) - rank(b.letter) ||
        collator.compare(a.key, b.key) ||
        collator.compare(a.entry.name, b.entry.name) ||
        collator.compare(a.entry.id, b.entry.id),
    );
}

/**
 * The artists matching `query`, best first: a name that starts with it,
 * then one with a word that does, then one that contains it, then one that
 * contains it ignoring spaces and punctuation. Within each, A to Z. An
 * empty query keeps every artist.
 */
export function filterArtists(artists: readonly IndexedArtist[], query: string): IndexedArtist[] {
  const q = fold(query);
  if (!q) return [...artists];
  const loose = loosen(q);
  const hits: { artist: IndexedArtist; score: number }[] = [];
  for (const artist of artists) {
    const { key } = artist;
    let score = -1;
    if (key.startsWith(q)) score = 0;
    else if (key.includes(` ${q}`)) score = 1;
    else if (key.includes(q)) score = 2;
    else if (loose && artist.loose.includes(loose)) score = 3;
    if (score >= 0) hits.push({ artist, score });
  }
  // Array.prototype.sort is stable, so each score keeps A to Z.
  return hits.sort((a, b) => a.score - b.score).map((h) => h.artist);
}

/** One row of the virtual list: a letter heading or an artist. */
export type ArtistListRow =
  { kind: "letter"; letter: string } | { kind: "artist"; artist: IndexedArtist; position: number };

/** Row heights, so the list never measures (docs/design.md, Lists). */
export const LETTER_ROW = 32;
export const ARTIST_ROW = 44;

/** The artists as list rows, with a heading before each letter when `grouped`. */
export function listRows(artists: readonly IndexedArtist[], grouped: boolean): ArtistListRow[] {
  const rows: ArtistListRow[] = [];
  let letter = "";
  artists.forEach((artist, position) => {
    if (grouped && artist.letter !== letter) {
      letter = artist.letter;
      rows.push({ kind: "letter", letter });
    }
    rows.push({ kind: "artist", artist, position });
  });
  return rows;
}

/** What the library holds of an artist: `12 albums, 148 songs`; either part alone when the other is 0. */
export function libraryLine(albums: number, songs: number): string {
  const a = albums ? plural(albums, { one: "# album", other: "# albums" }) : "";
  const s = songs ? plural(songs, { one: "# song", other: "# songs" }) : "";
  return a && s ? t`${a}, ${s}` : a || s;
}
