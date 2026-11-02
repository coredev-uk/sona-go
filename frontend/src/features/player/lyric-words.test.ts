import { describe, expect, it } from "vitest";

import {
  hasWordTiming,
  lineSpan,
  lineVoices,
  nextChange,
  restFrom,
  singingLines,
  sungLine,
  verseStarts,
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

  it("fills only held notes: a short word lights whole as it begins", () => {
    const short = { beginMs: 1_000, endMs: 1_599 };
    expect(wordState(short, 999, true).sung).toBe("");
    expect(wordState(short, 1_000, true)).toEqual({ sung: "done", p: 1 });
    expect(wordState({ beginMs: 1_000, endMs: 1_600 }, 1_300, true)).toEqual({
      sung: "part",
      p: 0.5,
    });
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

  it("keeps the last line lit through a short gap until the next begins", () => {
    expect(singingLines(spans, 7_000)).toEqual([2]);
    expect(singingLines(spans, 8_000)).toEqual([3]);
  });

  it("lets a line go once the voice rests after it", () => {
    // The last line is followed by a rest: lit until its end and a grace.
    expect(singingLines(spans, 9_499)).toEqual([3]);
    expect(singingLines(spans, 9_500)).toEqual([]);
    const brk = [
      { beginMs: 0, endMs: 2_000 },
      { beginMs: 5_000, endMs: 6_000 },
    ];
    expect(singingLines(brk, 2_499)).toEqual([0]);
    expect(singingLines(brk, 2_500)).toEqual([]);
    expect(singingLines(brk, 5_000)).toEqual([1]);
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

  it("wakes when the voice rests after the line", () => {
    expect(nextChange(spans, [0], [word(0, 800)], 2_100)).toBe(2_500);
  });
});

describe("restFrom", () => {
  it("is the line's end plus a grace when a break follows", () => {
    const spans = [
      { beginMs: 0, endMs: 2_000 },
      { beginMs: 4_999, endMs: 6_000 },
      { beginMs: 9_000, endMs: 10_000 },
    ];
    expect(restFrom(spans, 0)).toBe(Infinity); // 2.999 s: no break
    expect(restFrom(spans, 1)).toBe(6_500); // 3 s: a break
    expect(restFrom(spans, 2)).toBe(10_500); // the last line
  });

  it("never rests on a line without an end", () => {
    expect(restFrom([{ beginMs: 1_000, endMs: 0 }], 0)).toBe(Infinity);
  });
});

describe("sungLine", () => {
  const spans = [
    { beginMs: 1_000, endMs: 2_000 },
    { beginMs: 2_100, endMs: 3_000 },
    { beginMs: 8_000, endMs: 9_000 },
  ];

  it("anchors the first line before anything is sung", () => {
    expect(sungLine(spans, 0)).toEqual({ active: -1, anchor: 0, next: 1_000 });
    expect(sungLine([], 0)).toEqual({ active: -1, anchor: -1, next: Infinity });
  });

  it("is the last line begun, waking at the next line", () => {
    expect(sungLine(spans, 1_500)).toEqual({ active: 0, anchor: 0, next: 2_100 });
  });

  it("lights nothing in a break and anchors the line coming", () => {
    expect(sungLine(spans, 3_400)).toEqual({ active: 1, anchor: 1, next: 3_500 });
    expect(sungLine(spans, 3_500)).toEqual({ active: -1, anchor: 2, next: 8_000 });
    expect(sungLine(spans, 8_000)).toEqual({ active: 2, anchor: 2, next: 9_500 });
    expect(sungLine(spans, 9_500)).toEqual({ active: -1, anchor: 2, next: Infinity });
  });
});

describe("verseStarts", () => {
  const l = (part = "") => ({ part });

  it("opens a verse after a break or where the song part changes", () => {
    const spans = [
      { beginMs: 0, endMs: 1_000 },
      { beginMs: 1_200, endMs: 2_000 },
      { beginMs: 5_000, endMs: 6_000 },
      { beginMs: 6_100, endMs: 7_000 },
    ];
    expect(verseStarts([l(), l(), l(), l()], spans)).toEqual([false, false, true, false]);
    expect(verseStarts([l("Verse"), l("Chorus"), l("Chorus"), l("Chorus")], spans)).toEqual([
      false,
      true,
      true,
      false,
    ]);
  });

  it("finds no breaks in lines without ends", () => {
    const spans = [
      { beginMs: 0, endMs: 0 },
      { beginMs: 9_000, endMs: 0 },
    ];
    expect(verseStarts([l(), l()], spans)).toEqual([false, false]);
  });
});

describe("lineVoices", () => {
  const l = (agent: string, agentType = "person") => ({ agent, agentType });

  it("tells a duet's other voices and every voice together from the lead", () => {
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
