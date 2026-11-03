/**
 * Initials avatars: the letters and the tint drawn for a person Apple has
 * no picture of. Both come from the name alone, so a person looks the same
 * on every page and in every session.
 */

/** How many tints `index.css` defines (`--avatar-<n>-bg` / `-fg`, per theme). */
export const AVATAR_TONES = 8;

const graphemes =
  typeof Intl.Segmenter === "function"
    ? new Intl.Segmenter(undefined, { granularity: "grapheme" })
    : undefined;

/** The first user-perceived character: a letter with its marks, an emoji sequence, a surrogate pair. */
function firstGrapheme(s: string): string {
  if (graphemes) {
    for (const g of graphemes.segment(s)) return g.segment;
    return "";
  }
  return [...s][0] ?? "";
}

/** Upper case, unless that would make more than one character ("ß" → "SS"). */
function upper(g: string): string {
  const u = g.toUpperCase();
  return firstGrapheme(u) === u ? u : g;
}

/** What may start a word's initial: a letter or a number, in any script. */
const notLetterOrNumber = /^[^\p{L}\p{N}]+/u;

/**
 * The letters that stand for `name`, one or two: the first letter of the
 * first word and of the last ("Thom Yorke" → "TY", "Dan Grech-Marguerat" →
 * "DG"), or of the only word ("Madonna" → "M", "米津玄師" → "米").
 *
 * - Words are split on white space; punctuation before a word's letter
 *   ("O'Brien" keeps its O, "(Strings)" gives S) and words without a letter
 *   or number ("&") are skipped, and a leading "The" is when other words
 *   follow ("The Bad Plus" → "BP").
 * - A letter is a whole user-perceived character in any script, so marks,
 *   Hangul syllables and characters outside the BMP are never cut in half.
 * - A name with no letter or number at all ("$$$") gives its first character;
 *   an empty name gives "".
 */
export function initialsOf(name: string): string {
  const text = name.normalize("NFC").trim();
  const words = text
    .split(/\s+/u)
    .map((w) => w.replace(notLetterOrNumber, ""))
    .filter(Boolean);
  if (words.length > 1 && words[0].toLowerCase() === "the") words.shift();
  if (words.length === 0) return upper(firstGrapheme(text));
  const first = upper(firstGrapheme(words[0]));
  if (words.length === 1) return first;
  return first + upper(firstGrapheme(words[words.length - 1]));
}

/** The tint (0 to AVATAR_TONES - 1) of `name`: FNV-1a of its lower-case NFC text, so it never changes. */
export function avatarTone(name: string): number {
  let h = 0x811c9dc5;
  const text = name.normalize("NFC").trim().toLowerCase();
  for (let i = 0; i < text.length; i++) {
    h = Math.imul(h ^ text.charCodeAt(i), 0x01000193);
  }
  return (h >>> 0) % AVATAR_TONES;
}
