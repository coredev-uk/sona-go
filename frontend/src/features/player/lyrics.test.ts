import { describe, expect, it } from "vitest";

import {
  activeLine,
  describeOffset,
  wordClock,
  lyricsClock,
  lyricsExtras,
  MAX_OFFSET_MS,
  nudgeOffset,
  seekTarget,
  shownLine,
  useLyricsOffset,
} from "./lyrics";

describe("activeLine", () => {
  const lines = [{ beginMs: 1_000 }, { beginMs: 4_000 }, { beginMs: 4_000 }, { beginMs: 9_500 }];

  it("is -1 before the first line and for no lines", () => {
    expect(activeLine(lines, 0)).toBe(-1);
    expect(activeLine(lines, 999)).toBe(-1);
    expect(activeLine([], 5_000)).toBe(-1);
  });

  it("is the last line that has begun", () => {
    expect(activeLine(lines, 1_000)).toBe(0);
    expect(activeLine(lines, 3_999)).toBe(0);
    // Two lines starting together: the later one is current.
    expect(activeLine(lines, 4_000)).toBe(2);
    expect(activeLine(lines, 60_000)).toBe(3);
  });
});

describe("lyrics offset", () => {
  const lines = [{ beginMs: 10_000 }, { beginMs: 12_000 }];

  it("moves in half-second steps and stops at the limit", () => {
    expect(nudgeOffset(0, 1)).toBe(500);
    expect(nudgeOffset(500, -1)).toBe(0);
    expect(nudgeOffset(0, -1)).toBe(-500);
    // Back on the grid from any value (no float drift after many presses).
    expect(nudgeOffset(740, 1)).toBe(1_000);
    let o = 0;
    for (let i = 0; i < 100; i++) o = nudgeOffset(o, 1);
    expect(o).toBe(MAX_OFFSET_MS);
    for (let i = 0; i < 100; i++) o = nudgeOffset(o, -1);
    expect(o).toBe(-MAX_OFFSET_MS);
  });

  it("shifts which line is sung", () => {
    // At 9.6 s the first line (10 s) is not yet shown; the 250 ms lead
    // shows it from 9.75 s.
    expect(activeLine(lines, lyricsClock(9_600, 0))).toBe(-1);
    expect(activeLine(lines, lyricsClock(9_750, 0))).toBe(0);
    // Lyrics half a second earlier: it shows from 9.25 s.
    expect(activeLine(lines, lyricsClock(9_300, -500))).toBe(0);
    // Lyrics a second later: the first line holds until 11.25 s.
    expect(activeLine(lines, lyricsClock(12_000, 1_000))).toBe(0);
    expect(activeLine(lines, lyricsClock(12_750, 1_000))).toBe(1);
  });

  it("lights words without the lines' lead, under the offset", () => {
    expect(wordClock(10_000, 0)).toBe(10_000);
    expect(wordClock(10_000, 500)).toBe(9_500);
    expect(wordClock(10_000, -500)).toBe(10_500);
  });

  it("seeks to where a line is sung under the offset", () => {
    // Clicking a line lands where the view makes it the sung line.
    for (const offset of [0, 500, -1_500]) {
      const at = seekTarget(12_000, offset);
      expect(activeLine(lines, lyricsClock(at, offset))).toBe(1);
    }
    expect(seekTarget(200, -1_000)).toBe(0);
  });

  it("describes the offset in words", () => {
    expect(describeOffset(0)).toBe("");
    expect(describeOffset(-500)).toBe("0.5 s earlier");
    expect(describeOffset(2_000)).toBe("2 s later");
  });

  it("keeps an offset per song for the session", () => {
    const s = useLyricsOffset.getState();
    s.nudge("a", 1);
    s.nudge("a", 1);
    s.nudge("b", -1);
    expect(useLyricsOffset.getState().offsets).toMatchObject({ a: 1_000, b: -500 });
    useLyricsOffset.getState().reset("a");
    expect(useLyricsOffset.getState().offsets.a).toBe(0);
    expect(useLyricsOffset.getState().offsets.b).toBe(-500);
  });
});

describe("shownLine", () => {
  const word = (text: string, background = false) => ({ beginMs: 0, endMs: 1, text, background });
  const line = {
    beginMs: 0,
    endMs: 1,
    text: "lorem ipsum",
    backgroundText: "(dolor)",
    part: "",
    agent: "v1",
    agentType: "person",
    words: [word("lorem "), word("ipsum"), word("(dolor)", true)],
    translation: { text: "alpha bravo", backgroundText: "(charlie)", words: [] },
    transliteration: {
      text: "lo rem ip sum",
      backgroundText: "(do lor)",
      words: [word("lo "), word("rem ")],
    },
  };

  it("shows the main and background vocals, each with its own words", () => {
    const s = shownLine(line, { translation: false, pronunciation: false });
    expect(s).toEqual({
      lead: { text: "lorem ipsum", words: line.words.slice(0, 2) },
      background: { text: "(dolor)", words: line.words.slice(2) },
      pronunciation: null,
      translation: "",
    });
  });

  it("adds the romanisation and the translation under the line", () => {
    const s = shownLine(line, { translation: true, pronunciation: true });
    expect(s.lead.text).toBe("lorem ipsum");
    expect(s.pronunciation).toEqual({
      text: "lo rem ip sum (do lor)",
      words: line.transliteration.words,
    });
    expect(s.translation).toBe("alpha bravo (charlie)");
  });

  it("adds nothing for lines without localizations", () => {
    const bare = { ...line, translation: null, transliteration: null };
    const s = shownLine(bare, { translation: true, pronunciation: true });
    expect(s.lead.text).toBe("lorem ipsum");
    expect(s.pronunciation).toBeNull();
    expect(s.translation).toBe("");
  });

  it("keeps line-timed text without words", () => {
    const plain = { ...line, words: [], translation: null, transliteration: null };
    const s = shownLine(plain, { translation: false, pronunciation: false });
    expect(s.lead).toEqual({ text: "lorem ipsum", words: [] });
    expect(s.background).toEqual({ text: "(dolor)", words: [] });
  });
});

describe("lyricsExtras", () => {
  it("reports the localizations the lyrics carry", () => {
    expect(lyricsExtras(undefined)).toEqual({ translation: false, pronunciation: false });
    expect(lyricsExtras({ translationLanguage: "en-US", transliterationLanguage: "" })).toEqual({
      translation: true,
      pronunciation: false,
    });
  });
});
