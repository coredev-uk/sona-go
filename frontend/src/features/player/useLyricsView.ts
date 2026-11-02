import type { LyricLine, Lyrics } from "@bindings/services";
import { useEffect, useMemo, useRef, useState, type RefObject } from "react";

import { useAppearance } from "@/features/settings";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";

import type { LyricLangs } from "./LyricBody";
import { lineSpan, sungLine, verseStarts, type Sung } from "./lyric-words";
import { lyricsClock, useSongOffset } from "./lyrics";
import { livePosition, usePlayback } from "./playback";

/** After the user scrolls the lyrics, they are left alone this long. */
const USER_SCROLL_MS = 4_000;

const NOTHING_SUNG: Sung = { active: -1, anchor: -1, next: Infinity };

/** Where a line stands against the voice; "plain" for lyrics without timing. */
export type LineState = "past" | "active" | "future" | "plain";

/**
 * A line's text colour: the sung line in ink (its words fill with amber)
 * or, timed by line only, amber itself; earlier lines faint, later ones
 * muted, plain lyrics in ink.
 */
export function lineTone(state: LineState, wordTimed: boolean): string {
  if (state === "active") return wordTimed ? "text-ink" : "text-play-ink";
  if (state === "past") return "text-ink-faint";
  return state === "future" ? "text-ink-muted" : "text-ink";
}

/**
 * The line sung now under the song's offset, and the one to keep in view
 * (`lyric-words.ts`, sungLine). It re-renders the caller only when either
 * changes: a timer waits for the next line or rest instead of polling the
 * position.
 */
export function useSungLine(
  songId: string,
  lines: readonly LyricLine[],
  synced: boolean,
): { active: number; anchor: number } {
  const offset = useSongOffset(songId);
  const state = usePlayback((s) => s.state);
  const positionMs = usePlayback((s) => s.positionMs);
  const durationMs = usePlayback((s) => s.durationMs);
  const at = usePlayback((s) => s.at);
  const spans = useMemo(() => lines.map(lineSpan), [lines]);
  const [now, setNow] = useState(() => Date.now());
  const position = livePosition({ state, positionMs, durationMs, at }, Math.max(now, at));
  const t = lyricsClock(position, offset);
  const sung = synced ? sungLine(spans, t) : NOTHING_SUNG;
  const wait = sung.next - t;

  useEffect(() => {
    if (state !== "playing" || !Number.isFinite(wait)) return;
    const id = window.setTimeout(() => setNow(Date.now()), Math.max(wait, 0) + 1);
    return () => window.clearTimeout(id);
  }, [state, wait]);

  const { active, anchor } = sung;
  return useMemo(() => ({ active, anchor }), [active, anchor]);
}

/**
 * The lines' indexes grouped into verses (`lyric-words.ts`, verseStarts);
 * plain lyrics are one verse.
 */
export function useVerses(lines: readonly LyricLine[], synced: boolean): number[][] {
  return useMemo(() => {
    const starts = synced ? verseStarts(lines, lines.map(lineSpan)) : [];
    const verses: number[][] = [];
    lines.forEach((_, i) => {
      if (i === 0 || starts[i]) verses.push([]);
      verses[verses.length - 1].push(i);
    });
    return verses;
  }, [lines, synced]);
}

/** The languages of the lyrics and their localizations, for `lang`. */
export function useLyricLangs(l: Lyrics | undefined): LyricLangs {
  const lead = l?.language ?? "";
  const pronunciation = l?.transliterationLanguage ?? "";
  const translation = l?.translationLanguage ?? "";
  return useMemo(() => ({ lead, pronunciation, translation }), [lead, pronunciation, translation]);
}

/** Reduce effects or the system's reduced motion: no smooth scrolling, words lit whole. */
export function useReducedMotion(): boolean {
  const reduceEffects = useAppearance((s) => s.reduceEffects);
  const system = useMediaQuery("(prefers-reduced-motion: reduce)");
  return reduceEffects || system;
}

/**
 * The reading band, as fractions of the scroller's height: while the
 * anchor line sits inside it the words hold still; once it leaves, the
 * sheet turns so the anchor's verse starts at the band's top.
 */
const BAND_TOP = 0.14;
const BAND_BOTTOM = 0.72;

/**
 * Keeps the anchor line (`[data-line=<anchor>]`) in view inside the scroll
 * container `ref` by turning the sheet a verse at a time instead of
 * following every line: the words stay still while they are read. A turn
 * brings the anchor's verse (`[data-verse]`) to the top of the reading
 * band, or the anchor itself when its verse is too long for the band. A
 * scroll the user made in the last few seconds is left alone. Returns
 * the handler that records one (wheel, touch, keyboard).
 */
export function useTurnPage(ref: RefObject<HTMLElement | null>, anchor: number) {
  const userScrolled = useRef(0);
  const first = useRef(true);
  const reduced = useReducedMotion();

  useEffect(() => {
    const box = ref.current;
    const line = box?.querySelector<HTMLElement>(`[data-line="${anchor}"]`);
    if (!box || !line || Date.now() - userScrolled.current < USER_SCROLL_MS) return;
    const h = box.clientHeight;
    const top = offsetIn(line, box);
    const bottom = top + line.offsetHeight;
    const view = box.scrollTop;
    const jump = first.current || reduced;
    first.current = false;
    if (!jump && top >= view + h * BAND_TOP - 1 && bottom <= view + h * BAND_BOTTOM) return;
    const verse = line.closest<HTMLElement>("[data-verse]");
    const verseTop = verse ? offsetIn(verse, box) : top;
    const from = bottom - verseTop <= h * (BAND_BOTTOM - BAND_TOP) ? verseTop : top;
    box.scrollTo({ top: Math.max(0, from - h * BAND_TOP), behavior: jump ? "auto" : "smooth" });
  }, [ref, anchor, reduced]);

  return () => {
    userScrolled.current = Date.now();
  };
}

/** `el`'s top inside the scroll content of `box` (one of its ancestors). */
function offsetIn(el: HTMLElement, box: HTMLElement): number {
  return el.getBoundingClientRect().top - box.getBoundingClientRect().top + box.scrollTop;
}
