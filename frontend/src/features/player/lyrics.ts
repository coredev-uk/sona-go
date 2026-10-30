import type { LyricLine, LyricWord, Lyrics } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { create } from "zustand";

import { numberFormat } from "@/shared/lib/format";

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

/** Lyrics timed by line or word (others are plain text). */
export function isSynced(l: Pick<Lyrics, "timing"> | undefined): boolean {
  return l?.timing === "Line" || l?.timing === "Word";
}

/** Lines are shown this early so the eye arrives as the voice does. */
export const LYRICS_LEAD_MS = 250;
/** One press of "earlier" or "later". */
export const OFFSET_STEP_MS = 500;
/** The furthest the lyrics can be moved either way. */
export const MAX_OFFSET_MS = 10_000;

/**
 * The offset after one nudge (`dir` -1 = lyrics earlier, 1 = later), on
 * the 0.5 s grid and within ±MAX_OFFSET_MS. A negative offset shows the
 * lyrics earlier (for songs whose lyrics run behind the voice), a
 * positive one later.
 */
export function nudgeOffset(offsetMs: number, dir: 1 | -1): number {
  const steps = Math.round(offsetMs / OFFSET_STEP_MS) + dir;
  const next = steps * OFFSET_STEP_MS;
  return Math.min(MAX_OFFSET_MS, Math.max(-MAX_OFFSET_MS, next));
}

/** The lyrics clock at playback position `positionMs`: what activeLine reads. */
export function lyricsClock(positionMs: number, offsetMs: number): number {
  return positionMs + LYRICS_LEAD_MS - offsetMs;
}

/** Where to seek so that the line starting at `beginMs` is the one sung. */
export function seekTarget(beginMs: number, offsetMs: number): number {
  return Math.max(0, beginMs + offsetMs);
}

/** The offset in words: "0.5 s earlier", "1 s later", "" when there is none. */
export function describeOffset(offsetMs: number): string {
  if (offsetMs === 0) return "";
  const s = numberFormat({ maximumFractionDigits: 1 }).format(Math.abs(offsetMs) / 1000);
  return offsetMs < 0 ? t`${s} s earlier` : t`${s} s later`;
}

interface LyricsOffsetStore {
  /** Offset in ms by catalog song ID; songs not listed are at 0. */
  offsets: Readonly<Record<string, number>>;
  nudge: (songId: string, dir: 1 | -1) => void;
  reset: (songId: string) => void;
}

/**
 * Per-song lyrics offsets for this session. A misaligned lyrics file is a
 * property of the song, so the offset stays with it: a new song starts in
 * sync, and going back to a song keeps what was set for it. Nothing is
 * stored across restarts.
 */
export const useLyricsOffset = create<LyricsOffsetStore>((set) => ({
  offsets: {},
  nudge: (songId, dir) =>
    set((s) => ({ offsets: { ...s.offsets, [songId]: nudgeOffset(s.offsets[songId] ?? 0, dir) } })),
  reset: (songId) => set((s) => ({ offsets: { ...s.offsets, [songId]: 0 } })),
}));

/** The lyrics offset of `songId` (0 when none is set). */
export function useSongOffset(songId: string): number {
  return useLyricsOffset((s) => s.offsets[songId] ?? 0);
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
