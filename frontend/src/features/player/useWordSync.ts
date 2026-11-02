import type { LyricLine } from "@bindings/services";
import { useEffect, type RefObject } from "react";

import { lineSpan, nextChange, singingLines, wordState, type Span } from "./lyric-words";
import { useLyricsOffset, wordClock } from "./lyrics";
import { livePosition, usePlayback } from "./playback";
import { useReducedMotion } from "./useLyricsView";

interface WordEl extends Span {
  el: HTMLElement;
  sung: string;
  /** The --p last written, -1 when none is set. */
  p: number;
}

interface LitLine {
  el: HTMLElement;
  words: WordEl[];
}

function paint(w: WordEl, sung: string, p: number) {
  if (w.sung !== sung) {
    if (sung) w.el.dataset.sung = sung;
    else delete w.el.dataset.sung;
    w.sung = sung;
  }
  // Three decimals is under a pixel for any word.
  const q = sung === "part" ? Math.round(p * 1000) / 1000 : -1;
  if (q === w.p) return;
  if (q < 0) w.el.style.removeProperty("--p");
  else w.el.style.setProperty("--p", String(q));
  w.p = q;
}

/**
 * Lights word-timed lyrics word by word inside `ref`, without rendering
 * React: each line is a `[data-line=<index>]` element, each timed word a
 * `[data-wb][data-we]` span (its begin and end in ms). Words already sung
 * get `data-sung="done"`; the word being sung gets `data-sung="part"` and
 * `--p`, how far through it the voice is (0–1), which the stylesheet
 * turns into a wipe. Only the lines being sung are touched.
 *
 * The clock is the playback position, extrapolated between Go's
 * snapshots. One requestAnimationFrame loop runs only while a word is
 * being sung, the song plays and the page is visible; between words it
 * sleeps on a timer until the next one begins. Under reduced motion words
 * light whole at their begin, on timers only. `layout` changes whenever
 * the words rendered change (the pronunciation toggle), so the loop finds
 * them again.
 */
export function useWordSync(
  ref: RefObject<HTMLElement | null>,
  songId: string,
  lines: readonly LyricLine[],
  enabled: boolean,
  layout: unknown,
) {
  const reduced = useReducedMotion();

  useEffect(() => {
    const root = ref.current;
    if (!enabled || !root) return;
    const smooth = !reduced;
    const spans = lines.map(lineSpan);
    const lit = new Map<number, LitLine>();
    let frame = 0;
    let timer = 0;

    const find = (i: number): LitLine | null => {
      const el = root.querySelector<HTMLElement>(`[data-line="${i}"]`);
      if (!el) return null;
      const words = Array.from(el.querySelectorAll<HTMLElement>("[data-wb]"), (w) => ({
        el: w,
        beginMs: Number(w.dataset.wb),
        endMs: Number(w.dataset.we),
        sung: w.dataset.sung ?? "",
        p: -1,
      }));
      return { el, words };
    };
    const clear = (l: LitLine) => {
      for (const w of l.words) paint(w, "", 0);
    };
    const stop = () => {
      cancelAnimationFrame(frame);
      window.clearTimeout(timer);
      frame = 0;
      timer = 0;
    };

    const tick = () => {
      stop();
      const pb = usePlayback.getState();
      const offset = useLyricsOffset.getState().offsets[songId] ?? 0;
      const t = wordClock(livePosition(pb, Date.now()), offset);
      const singing = singingLines(spans, t);

      for (const [i, l] of lit) {
        if (!singing.includes(i) || !l.el.isConnected) {
          clear(l);
          lit.delete(i);
        }
      }
      let partial = false;
      const words: Span[] = [];
      for (const i of singing) {
        let l = lit.get(i);
        if (!l) {
          l = find(i) ?? undefined;
          if (!l) continue;
          lit.set(i, l);
        }
        for (const w of l.words) {
          const s = wordState(w, t, smooth);
          paint(w, s.sung, s.p);
          if (s.sung === "part") partial = true;
          words.push(w);
        }
      }

      if (pb.state !== "playing" || document.hidden) return;
      if (partial) {
        frame = requestAnimationFrame(tick);
        return;
      }
      const next = nextChange(spans, singing, words, t);
      if (next < Infinity) timer = window.setTimeout(tick, next - t + 1);
    };

    // A new snapshot from Go (play, pause, seek), a new offset or the page
    // coming back into view: start over from the new clock.
    const unsubPlayback = usePlayback.subscribe((s, prev) => {
      if (s.state !== prev.state || s.positionMs !== prev.positionMs || s.at !== prev.at) tick();
    });
    const unsubOffset = useLyricsOffset.subscribe(tick);
    document.addEventListener("visibilitychange", tick);
    tick();

    return () => {
      stop();
      unsubPlayback();
      unsubOffset();
      document.removeEventListener("visibilitychange", tick);
      for (const l of lit.values()) clear(l);
    };
  }, [ref, songId, lines, enabled, reduced, layout]);
}
