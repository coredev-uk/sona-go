import { describe, expect, it } from "vitest";

import {
  hasWordTiming,
  lineSpan,
  lineVoices,
  nextChange,
  singingLines,
  wordProgress,
  wordState,
} from "./lyric-words";

const word = (beginMs: number, endMs: number, background = false) => ({
  beginMs,
  endMs,
  text: "lorem ",
  background,
});

describe("wordProgress", () => {
  const w = { beginMs: 1_000, endMs: 1_400 };

  it("is 0 before the word and 1 from its end", () => {
    expect(wordProgress(w, 0)).toBe(0);
    expect(wordProgress(w, 999)).toBe(0);
    expect(wordProgress(w, 1_400)).toBe(1);
    expect(wordProgress(w, 9_000)).toBe(1);
  });

  it("runs linearly through the word, starting at its begin", () => {
    expect(wordProgress(w, 1_000)).toBe(0);
    expect(wordProgress(w, 1_100)).toBe(0.25);
    expect(wordProgress(w, 1_399)).toBeCloseTo(0.9975);
  });

  it("sings a word with no length the moment it begins", () => {
    const z = { beginMs: 500, endMs: 500 };
    expect(wordProgress(z, 499)).toBe(0);
    expect(wordProgress(z, 500)).toBe(1);
    expect(wordProgress({ beginMs: 500, endMs: 400 }, 500)).toBe(1);
  });
});

describe("wordState", () => {
  const w = { beginMs: 1_000, endMs: 2_000 };

  it("is unsung, part-sung, then sung", () => {
    expect(wordState(w, 999, true)).toEqual({ sung: "", p: 0 });
    expect(wordState(w, 1_000, true)).toEqual({ sung: "part", p: 0 });
    expect(wordState(w, 1_500, true)).toEqual({ sung: "part", p: 0.5 });
    expect(wordState(w, 2_000, true)).toEqual({ sung: "done", p: 1 });
  });

  it("lights whole words at their begin under reduced motion", () => {
    expect(wordState(w, 999, false).sung).toBe("");
    expect(wordState(w, 1_000, false)).toEqual({ sung: "done", p: 1 });
    expect(wordState(w, 1_500, false)).toEqual({ sung: "done", p: 1 });
  });

  it("follows the clock back after a seek", () => {
    expect(wordState(w, 1_900, true).sung).toBe("part");
    // Seeking back before the word unlights it; a pause holds the state.
    expect(wordState(w, 200, true).sung).toBe("");
    expect(wordState(w, 1_250, true)).toEqual({ sung: "part", p: 0.25 });
  });
});

describe("lineSpan", () => {
  it("runs from the line's begin to its last word's end", () => {
    const line = { beginMs: 1_000, endMs: 3_000, words: [word(1_000, 2_000), word(2_000, 3_000)] };
    expect(lineSpan(line)).toEqual({ beginMs: 1_000, endMs: 3_000 });
  });

  it("reaches past the line's end for background vocals", () => {
    const line = {
      beginMs: 1_000,
      endMs: 3_000,
      words: [word(1_000, 3_000), word(2_500, 4_200, true)],
    };
    expect(lineSpan(line)).toEqual({ beginMs: 1_000, endMs: 4_200 });
  });

  it("starts at a word sung before the line's own begin", () => {
    expect(lineSpan({ beginMs: 1_000, endMs: 2_000, words: [word(900, 2_000)] })).toEqual({
      beginMs: 900,
      endMs: 2_000,
    });
  });

  it("is the line's own time without words", () => {
    expect(lineSpan({ beginMs: 5, endMs: 9, words: [] })).toEqual({ beginMs: 5, endMs: 9 });
  });
});

describe("singingLines", () => {
  // Line 1's background vocals run until 4.5 s, under line 2.
  const spans = [
    { beginMs: 0, endMs: 2_000 },
    { beginMs: 2_000, endMs: 4_500 },
    { beginMs: 4_000, endMs: 6_000 },
    { beginMs: 8_000, endMs: 9_000 },
  ];

  it("is nothing before the first line", () => {
    expect(singingLines(spans, -1)).toEqual([]);
    expect(singingLines([], 100)).toEqual([]);
  });

  it("is the last line begun", () => {
    expect(singingLines(spans, 0)).toEqual([0]);
    expect(singingLines(spans, 1_999)).toEqual([0]);
    expect(singingLines(spans, 2_000)).toEqual([1]);
  });

  it("keeps an earlier line while its background vocals run on", () => {
    expect(singingLines(spans, 4_000)).toEqual([1, 2]);
    expect(singingLines(spans, 4_499)).toEqual([1, 2]);
    expect(singingLines(spans, 4_500)).toEqual([2]);
  });

  it("keeps the last line lit through a gap until the next begins", () => {
    expect(singingLines(spans, 7_000)).toEqual([2]);
    expect(singingLines(spans, 8_000)).toEqual([3]);
    expect(singingLines(spans, 60_000)).toEqual([3]);
  });

  it("follows a seek backwards and forwards", () => {
    expect(singingLines(spans, 8_500)).toEqual([3]);
    expect(singingLines(spans, 500)).toEqual([0]);
    expect(singingLines(spans, 4_200)).toEqual([1, 2]);
  });
});

describe("nextChange", () => {
  const spans = [
    { beginMs: 0, endMs: 2_000 },
    { beginMs: 5_000, endMs: 6_000 },
  ];

  it("is the next word boundary", () => {
    const words = [word(0, 800), word(1_000, 2_000)];
    expect(nextChange(spans, [0], words, 100)).toBe(800);
    expect(nextChange(spans, [0], words, 800)).toBe(1_000);
    expect(nextChange(spans, [0], words, 1_000)).toBe(2_000);
  });

  it("sleeps through a gap until the next line begins", () => {
    expect(nextChange(spans, [0], [word(0, 800)], 2_500)).toBe(5_000);
  });

  it("is Infinity once everything is sung", () => {
    expect(nextChange(spans, [1], [word(5_000, 6_000)], 7_000)).toBe(Infinity);
  });
});

describe("lineVoices", () => {
  const l = (agent: string, agentType = "person") => ({ agent, agentType });

  it("puts a duet's second voice on the other side and groups in the middle", () => {
    expect(lineVoices([l("v1"), l("v2", "other"), l("v1000", "group"), l("v1")])).toEqual([
      "one",
      "two",
      "all",
      "one",
    ]);
  });

  it("takes the first voice to sing as the lead", () => {
    expect(lineVoices([l("v2"), l("v1"), l("v3")])).toEqual(["one", "two", "two"]);
  });

  it("keeps a single voice on one side", () => {
    expect(lineVoices([l("v1"), l(""), l("v1")])).toEqual(["one", "one", "one"]);
    expect(lineVoices([l("v1000", "group"), l("v1")])).toEqual(["all", "one"]);
    expect(lineVoices([])).toEqual([]);
  });
});

describe("hasWordTiming", () => {
  it("needs word timing and words", () => {
    expect(hasWordTiming(undefined)).toBe(false);
    expect(hasWordTiming({ timing: "Line", lines: [{ words: [] }] })).toBe(false);
    expect(hasWordTiming({ timing: "Word", lines: [{ words: [] }] })).toBe(false);
    expect(hasWordTiming({ timing: "Word", lines: [{ words: [word(0, 1)] }] })).toBe(true);
  });
});
