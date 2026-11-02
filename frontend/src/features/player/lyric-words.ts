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
 * Words sung at least this long are held notes: they fill as the voice
 * holds them. Shorter words light whole as they begin, where a fill
 * would be a blur of frames nobody can follow (each drawn frame costs a
 * composite of the whole window: docs/frontend-architecture.md,
 * "Word-timed lyrics").
 */
export const HELD_MS = 600;

/** A held note's fill is redrawn at most this often (12.5 fps). */
export const FILL_STEP_MS = 80;

/**
 * A word's state at `t`. A held note (HELD_MS) is part-sung while the
 * voice holds it; any other word, and every word without `smooth`
 * (reduced motion), is lit whole from its begin.
 */
export function wordState(w: Span, t: number, smooth: boolean): WordState {
  const p = wordProgress(w, t);
  if (t < w.beginMs) return { sung: "", p: 0 };
  if (p >= 1 || !smooth || w.endMs - w.beginMs < HELD_MS) return { sung: "done", p: 1 };
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

/**
 * A pause at least this long between one line's end and the next one's
 * begin is a break between verses: the sheet leaves a gap there, and
 * nothing is lit while the voice rests.
 */
export const VERSE_BREAK_MS = 3_000;

/** How long a line stays lit after it ends when a break follows it. */
const REST_GRACE_MS = 500;

/**
 * When the voice rests after line `i`: its end plus a short grace when a
 * break follows it (the last line is followed by one), Infinity when the
 * next line follows closely or the line has no end.
 */
export function restFrom(spans: readonly Span[], i: number): number {
  const { beginMs, endMs } = spans[i];
  if (endMs <= beginMs) return Infinity;
  const next = i + 1 < spans.length ? spans[i + 1].beginMs : Infinity;
  return next - endMs >= VERSE_BREAK_MS ? endMs + REST_GRACE_MS : Infinity;
}

/** The sung line and where the sheet should be. */
export interface Sung {
  /** The line sung now, -1 before the first and while the voice rests. */
  active: number;
  /** The line to keep in view: the sung one, or the one coming after a rest. */
  anchor: number;
  /** When `active` or `anchor` next changes (Infinity: never). */
  next: number;
}

/**
 * The sung line at `t`: the last line that has begun, unless the voice
 * is resting in a break after it. `spans` are the lines' spans in order.
 */
export function sungLine(spans: readonly Span[], t: number): Sung {
  const k = activeLine(spans, t);
  if (k < 0) {
    return { active: -1, anchor: spans.length ? 0 : -1, next: spans[0]?.beginMs ?? Infinity };
  }
  const nextBegin = k + 1 < spans.length ? spans[k + 1].beginMs : Infinity;
  const rest = restFrom(spans, k);
  if (t >= rest) {
    return { active: -1, anchor: k + 1 < spans.length ? k + 1 : k, next: nextBegin };
  }
  return { active: k, anchor: k, next: Math.min(rest, nextBegin) };
}

/**
 * Which lines open a verse: after a break (VERSE_BREAK_MS), or where the
 * song part the lyrics name changes ("Verse" to "Chorus"). The first line
 * opens none: it starts the sheet.
 */
export function verseStarts(
  lines: readonly Pick<LyricLine, "part">[],
  spans: readonly Span[],
): boolean[] {
  return lines.map((l, i) => {
    if (i === 0) return false;
    const prev = lines[i - 1];
    if (l.part && prev.part && l.part !== prev.part) return true;
    const end = spans[i - 1].endMs;
    return end > spans[i - 1].beginMs && spans[i].beginMs - end >= VERSE_BREAK_MS;
  });
}

/** How many earlier lines may still be sung under the current one. */
const LOOK_BACK = 4;

/**
 * The lines whose words are lit at `t`, in order: the last line that has
 * begun (it stays lit until the next begins, or until the voice rests
 * after it) and the earlier ones still being sung, such as background
 * vocals running on under the next line. `spans` are the lines' spans, in
 * the lines' order (by begin).
 */
export function singingLines(spans: readonly Span[], t: number): number[] {
  const k = activeLine(spans, t);
  if (k < 0) return [];
  const out = t < restFrom(spans, k) ? [k] : [];
  for (let j = k - 1; j >= 0 && j >= k - LOOK_BACK; j--) {
    if (spans[j].endMs > t) out.unshift(j);
  }
  return out;
}

/**
 * The next time after `t` at which anything lit could change: a word of
 * `words` beginning or ending, the next line beginning, a singing line
 * ending, or the voice resting. Infinity when nothing is left.
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
  if (k >= 0) consider(restFrom(spans, k));
  if (k + 1 < spans.length) consider(spans[k + 1].beginMs);
  return next;
}

/**
 * Whose line it is: "one" for the lead voice, "two" for a duet's other
 * voices, "all" for every voice together.
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
