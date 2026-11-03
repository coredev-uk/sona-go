import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { AVATAR_TONES, avatarTone, initialsOf } from "./avatar";

describe("initialsOf", () => {
  it("takes the first letter of the first and last word", () => {
    expect(initialsOf("Thom Yorke")).toBe("TY");
    expect(initialsOf("Nigel Godrich")).toBe("NG");
    expect(initialsOf("Ludwig van Beethoven")).toBe("LB");
    expect(initialsOf("Dan Grech-Marguerat")).toBe("DG");
    expect(initialsOf("Simon & Garfunkel")).toBe("SG");
  });

  it("is never more than two letters, and one for a single word", () => {
    expect(initialsOf("Madonna")).toBe("M");
    expect(initialsOf("Juan Carlos de la Cruz Pérez")).toBe("JP");
    expect(initialsOf("A")).toBe("A");
  });

  it("upper-cases and skips punctuation before a letter", () => {
    expect(initialsOf("j-hope")).toBe("J");
    expect(initialsOf("lil' kim")).toBe("LK");
    expect(initialsOf("  (Strings)  ")).toBe("S");
    expect(initialsOf("Edward O'Brien")).toBe("EO");
    expect(initialsOf("'Cause I Said")).toBe("CS");
  });

  it("counts numbers, drops a leading The and ignores bare symbols", () => {
    expect(initialsOf("50 Cent")).toBe("5C");
    expect(initialsOf("The Bad Plus")).toBe("BP");
    expect(initialsOf("The Weeknd")).toBe("W");
    expect(initialsOf("The")).toBe("T");
    expect(initialsOf("Mike & The Mechanics")).toBe("MM");
  });

  it("handles names in other scripts as whole characters", () => {
    expect(initialsOf("米津玄師")).toBe("米");
    expect(initialsOf("堀 正輝")).toBe("堀正");
    expect(initialsOf("봉준호")).toBe("봉");
    expect(initialsOf("فيروز")).toBe("ف");
    expect(initialsOf("Владимир Высоцкий")).toBe("ВВ");
    expect(initialsOf("ヨーヨー・マ")).toBe("ヨ");
    // An accented capital stays one letter, a combining sequence is not split,
    // and a character outside the BMP is not cut into surrogates.
    expect(initialsOf("Émile Zola")).toBe("ÉZ");
    expect(initialsOf("e\u0301mile")).toBe("\u00c9");
    expect(initialsOf("𠮷野 家")).toBe("𠮷家");
  });

  it("keeps a letter that would upper-case to several", () => {
    expect(initialsOf("ßo")).toBe("ß");
  });

  it("falls back to the first character of a name with no letters, or nothing", () => {
    expect(initialsOf("$$$")).toBe("$");
    expect(initialsOf("🎻 !!!")).toBe("🎻");
    expect(initialsOf("   ")).toBe("");
    expect(initialsOf("")).toBe("");
  });
});

describe("avatarTone", () => {
  it("is deterministic and ignores case, spacing and Unicode form", () => {
    expect(avatarTone("Thom Yorke")).toBe(avatarTone("Thom Yorke"));
    expect(avatarTone("Thom Yorke")).toBe(avatarTone("  thom yorke "));
    expect(avatarTone("Émile")).toBe(avatarTone("E\u0301mile"));
  });

  it("stays in range and uses every tint across a crowd of names", () => {
    const seen = new Set<number>();
    for (let i = 0; i < 400; i++) {
      const t = avatarTone(`Person ${i}`);
      expect(t).toBeGreaterThanOrEqual(0);
      expect(t).toBeLessThan(AVATAR_TONES);
      seen.add(t);
    }
    expect(seen.size).toBe(AVATAR_TONES);
    expect(avatarTone("米津玄師")).toBeLessThan(AVATAR_TONES);
  });

  it("keeps the names of one record apart", () => {
    const record = [
      "Thom Yorke",
      "Jonny Greenwood",
      "Colin Greenwood",
      "Philip Selway",
      "Nigel Godrich",
    ];
    expect(new Set(record.map(avatarTone)).size).toBeGreaterThanOrEqual(3);
  });
});

/** WCAG relative luminance and contrast of #rrggbb colours. */
function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255;
    return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe("the tints in index.css", () => {
  const css = readFileSync(
    path.resolve(import.meta.dirname, "../../shared/styles/index.css"),
    "utf8",
  );
  const block = (selector: RegExp) => {
    const m = selector.exec(css);
    expect(m).not.toBeNull();
    return css.slice(m!.index, css.indexOf("}", m!.index));
  };
  const themes = {
    light: block(/:root,\s*\.theme-light\s*\{/),
    dark: block(/\n\.dark\s*\{/),
  };

  for (const [theme, body] of Object.entries(themes)) {
    it(`gives every tint its colours in the ${theme} theme, text ≥ 4.5:1 on its ground`, () => {
      const tokens = [...body.matchAll(/--avatar-(\d+)-(bg|fg):\s*(#[0-9a-f]{6});/gi)];
      expect(tokens).toHaveLength(AVATAR_TONES * 2);
      for (let n = 0; n < AVATAR_TONES; n++) {
        const get = (k: string) => tokens.find((m) => Number(m[1]) === n && m[2] === k)?.[3] ?? "";
        expect(get("bg"), `tint ${n} bg`).not.toBe("");
        expect(contrast(get("fg"), get("bg")), `${theme} tint ${n}`).toBeGreaterThanOrEqual(4.5);
      }
    });
  }

  it("keeps the tints apart from each other in each theme", () => {
    for (const body of Object.values(themes)) {
      const bgs = [...body.matchAll(/--avatar-\d+-bg:\s*(#[0-9a-f]{6})/gi)].map((m) => m[1]);
      expect(new Set(bgs).size).toBe(AVATAR_TONES);
    }
  });
});
