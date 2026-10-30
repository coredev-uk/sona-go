import { describe, expect, it } from "vitest";

import { activeLine, lyricsExtras, shownLine } from "./lyrics";

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

describe("shownLine", () => {
  const word = (text: string) => ({ beginMs: 0, endMs: 1, text, background: false });
  const line = {
    beginMs: 0,
    endMs: 1,
    text: "보고 싶다",
    backgroundText: "(보고 싶다)",
    part: "",
    agent: "v1",
    words: [word("보고 "), word("싶다")],
    translation: { text: "I miss you", backgroundText: "(I miss you)", words: [] },
    transliteration: {
      text: "bo go sip da",
      backgroundText: "(bo go sip da)",
      words: [word("bo go ")],
    },
  };

  it("shows the original when nothing is chosen", () => {
    const s = shownLine(line, { translation: false, pronunciation: false });
    expect(s).toEqual({
      text: "보고 싶다",
      background: "(보고 싶다)",
      words: line.words,
      translation: "",
    });
  });

  it("swaps in the romanization and adds the translation", () => {
    const s = shownLine(line, { translation: true, pronunciation: true });
    expect(s.text).toBe("bo go sip da");
    expect(s.background).toBe("(bo go sip da)");
    expect(s.words).toHaveLength(1);
    expect(s.translation).toBe("I miss you (I miss you)");
  });

  it("keeps the original for lines without localizations", () => {
    const bare = { ...line, translation: null, transliteration: null };
    const s = shownLine(bare, { translation: true, pronunciation: true });
    expect(s.text).toBe("보고 싶다");
    expect(s.translation).toBe("");
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
