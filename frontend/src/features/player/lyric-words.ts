import type { LyricLine } from "@bindings/services";

import { activeLine } from "./lyrics";

/** A stretch of the song in ms: a word, or the time a line is sung. */
export interface Span {
  beginMs: number;
  endMs: number;
}

/**
 * How far the voice is through `w` at `t`: 0 before it begins, 1 once it
 * ends, linear in between. A word with no length is sung the moment it
 * begins.
 */
export function wordProgress(w: Span, t: number): number {
  if (t < w.beginMs) return 0;
  if (t >= w.endMs) return 1;
  return (t - w.beginMs) / (w.endMs - w.beginMs);
}

/** What a word shows: not sung yet (""), being sung ("part", `p` of it) or sung ("done"). */
export interface WordState {
  sung: "" | "part" | "done";
  p: number;
}

/**
 * A word's state at `t`. Without `smooth` (reduced motion) a word is lit
 * whole from its begin and never shows a part.
 */
export function wordState(w: Span, t: number, smooth: boolean): WordState {
  const p = wordProgress(w, t);
  if (t < w.beginMs) return { sung: "", p: 0 };
  if (p >= 1 || !smooth) return { sung: "done", p: 1 };
  return { sung: "part", p };
}

/**
 * The time a line is sung: from its begin (or an earlier first word) to
 * its end or its last word's end, whichever is later. Background vocals
 * often run on past the line's own end, under the next line.
 */
export function lineSpan(line: Pick<LyricLine, "beginMs" | "endMs" | "words">): Span {
  let beginMs = line.beginMs;
  let endMs = line.endMs;
  for (const w of line.words) {
    if (w.beginMs < beginMs) beginMs = w.beginMs;
    if (w.endMs > endMs) endMs = w.endMs;
  }
  return { beginMs, endMs };
}

/** How many earlier lines may still be sung under the current one. */
const LOOK_BACK = 4;

/**
 * The lines whose words are lit at `t`, in order: the last line that has
 * begun (it stays lit until the next begins) and the earlier ones still
 * being sung, such as background vocals running on under the next line.
 * `spans` are the lines' spans, in the lines' order (by begin).
 */
export function singingLines(spans: readonly Span[], t: number): number[] {
  const k = activeLine(spans, t);
  if (k < 0) return [];
  const out = [k];
  for (let j = k - 1; j >= 0 && j >= k - LOOK_BACK; j--) {
    if (spans[j].endMs > t) out.unshift(j);
  }
  return out;
}

/**
 * The next time after `t` at which anything lit could change: a word of
 * `words` beginning or ending, the next line beginning, or a singing line
 * ending. Infinity when nothing is left.
 */
export function nextChange(
  spans: readonly Span[],
  singing: readonly number[],
  words: readonly Span[],
  t: number,
): number {
  let next = Infinity;
  const consider = (ms: number) => {
    if (ms > t && ms < next) next = ms;
  };
  for (const w of words) {
    consider(w.beginMs);
    consider(w.endMs);
  }
  for (const i of singing) consider(spans[i].endMs);
  const k = activeLine(spans, t);
  if (k + 1 < spans.length) consider(spans[k + 1].beginMs);
  return next;
}

/**
 * Where a line sits: "one" for the lead voice, "two" for a duet's other
 * voices (the other side), "all" for every voice together.
 */
export type Voice = "one" | "two" | "all";

/**
 * Each line's voice. Lines sung by a group are "all"; when two or more
 * other voices take lines, the first to sing is "one" and the rest "two".
 * A song with one voice is "one" throughout.
 */
export function lineVoices(lines: readonly Pick<LyricLine, "agent" | "agentType">[]): Voice[] {
  let lead = "";
  let duet = false;
  for (const l of lines) {
    if (!l.agent || l.agentType === "group") continue;
    if (!lead) lead = l.agent;
    else if (l.agent !== lead) duet = true;
  }
  return lines.map((l) => {
    if (l.agentType === "group") return "all";
    return duet && l.agent && l.agent !== lead ? "two" : "one";
  });
}

/** Whether the lyrics carry word timing to light word by word. */
export function hasWordTiming(
  lyrics: { timing: string; lines: readonly Pick<LyricLine, "words">[] } | undefined,
): boolean {
  return lyrics?.timing === "Word" && lyrics.lines.some((l) => l.words.length > 0);
}
