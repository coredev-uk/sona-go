import type { LyricLine, LyricWord, Lyrics } from "@bindings/services";
import { create } from "zustand";

/** Index of the line sung at `ms`: the last line that has begun, or -1 before the first. */
export function activeLine(lines: readonly { beginMs: number }[], ms: number): number {
  let lo = 0;
  let hi = lines.length - 1;
  let found = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (lines[mid].beginMs <= ms) {
      found = mid;
      lo = mid + 1;
    } else hi = mid - 1;
  }
  return found;
}

/** What the lyrics view adds to the sung words. */
export interface LyricsPrefs {
  /** The line translated into the account's language, under each line. */
  translation: boolean;
  /** The words in Latin script (romanization) instead of the original script. */
  pronunciation: boolean;
}

const PREFS_KEY = "sona.lyrics";

function loadPrefs(): LyricsPrefs {
  try {
    const p = JSON.parse(localStorage.getItem(PREFS_KEY) ?? "{}") as Partial<LyricsPrefs>;
    return { translation: p.translation === true, pronunciation: p.pronunciation === true };
  } catch {
    return { translation: false, pronunciation: false };
  }
}

interface LyricsPrefsStore extends LyricsPrefs {
  set: (change: Partial<LyricsPrefs>) => void;
}

/** The lyrics view's choices, kept across songs and restarts. */
export const useLyricsPrefs = create<LyricsPrefsStore>((set, get) => ({
  ...loadPrefs(),
  set: (change) => {
    set(change);
    const { translation, pronunciation } = get();
    localStorage.setItem(PREFS_KEY, JSON.stringify({ translation, pronunciation }));
  },
}));

/** Which localizations these lyrics carry (the toggles shown for them). */
export function lyricsExtras(
  l: Pick<Lyrics, "translationLanguage" | "transliterationLanguage"> | undefined,
) {
  return { translation: !!l?.translationLanguage, pronunciation: !!l?.transliterationLanguage };
}

/** One line as the view shows it under the current choices. */
export interface ShownLine {
  text: string;
  background: string;
  words: LyricWord[];
  /** The translated line, "" when not shown or missing. */
  translation: string;
}

/**
 * A line under `prefs`: its romanization in place of the original when
 * pronunciation is on and the line has one, and its translation beneath
 * when translation is on.
 */
export function shownLine(line: LyricLine, prefs: LyricsPrefs): ShownLine {
  const tl = prefs.pronunciation ? line.transliteration : null;
  const tr = prefs.translation ? line.translation : null;
  return {
    text: tl?.text || line.text,
    background: tl ? tl.backgroundText : line.backgroundText,
    words: tl ? tl.words : line.words,
    translation: tr ? [tr.text, tr.backgroundText].filter(Boolean).join(" ") : "",
  };
}
