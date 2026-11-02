import { describe, expect, it } from "vitest";

import {
  chooseTranslation,
  formatLocales,
  matchLocale,
  selectableShare,
  type Translation,
  translatedShare,
  translationCandidates,
} from "./locale";

describe("matchLocale", () => {
  const available = ["en", "pt-PT", "pt-BR", "zh-Hant"];

  it("prefers an exact tag, then the bare language, then a sibling region", () => {
    expect(matchLocale(["pt-br"], available)).toBe("pt-BR");
    expect(matchLocale(["en-GB"], available)).toBe("en");
    expect(matchLocale(["pt-AO"], available)).toBe("pt-PT");
    expect(matchLocale(["zh-Hant-TW"], available)).toBe("zh-Hant");
  });

  it("takes the first preference that has any catalogue", () => {
    expect(matchLocale(["", undefined, "de-DE", "pt-BR", "en"], available)).toBe("pt-BR");
    expect(matchLocale(["de-DE", "fr"], available)).toBeUndefined();
  });
});

describe("translationCandidates", () => {
  it("takes the account's then the system's tags up to the first English one", () => {
    expect(translationCandidates(["fr-FR", "de-DE", "en-GB", "ja"])).toEqual(["fr-FR", "de-DE"]);
    expect(translationCandidates(["", "fr-FR", "fr-fr", "fr"])).toEqual(["fr-FR", "fr"]);
    expect(translationCandidates(["en-GB", "fr-FR"])).toEqual([]);
  });
});

describe("translatedShare", () => {
  const source = ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j"];

  it("adds the community messages Apple's layer lacks to Apple's coverage", () => {
    expect(
      translatedShare(0.2, { a: "A", b: "B" }, { b: "b", c: "c", z: "z" }, source),
    ).toBeCloseTo(0.3);
    expect(translatedShare(0.2, { a: "A", b: "B" }, undefined, source)).toBe(0.2);
    expect(translatedShare(0, {}, { a: "a" }, [])).toBe(0);
  });
});

describe("chooseTranslation", () => {
  const translation = (locale: string, share: number): Translation => ({
    locale,
    share,
    messages: {},
  });

  it("shows a locale only from the selectable share", async () => {
    expect(selectableShare).toBe(0.9);
    const shares: Record<string, number> = { "fr-FR": 0.89, "de-DE": 0.9, ja: 1 };
    const load = (tag: string) => Promise.resolve(translation(tag, shares[tag] ?? 0));
    expect((await chooseTranslation(["fr-FR", "de-DE", "ja"], load))?.locale).toBe("de-DE");
    expect(await chooseTranslation(["fr-FR"], load)).toBeUndefined();
  });

  it("stops at English: an English preference before a translation keeps English", async () => {
    const asked: string[] = [];
    const load = (tag: string) => {
      asked.push(tag);
      return Promise.resolve(translation(tag, 1));
    };
    expect(await chooseTranslation(["", "en-GB", "fr-FR"], load)).toBeUndefined();
    expect(asked).toEqual([]);
  });

  it("asks for today's typical coverage and keeps English", async () => {
    const load = (tag: string) => Promise.resolve(translation(tag, 98 / 1415));
    expect(await chooseTranslation(["fr-FR", "fr"], load)).toBeUndefined();
  });

  it("shows a chosen language however complete, once it translates anything", async () => {
    const shares: Record<string, number> = { "fr-FR": 98 / 1415, "de-DE": 0 };
    const load = (tag: string) => Promise.resolve(translation(tag, shares[tag] ?? 0));
    expect((await chooseTranslation(["fr-FR"], load, 0))?.locale).toBe("fr-FR");
    expect(await chooseTranslation(["de-DE"], load, 0)).toBeUndefined();
    expect(await chooseTranslation(["en"], load, 0)).toBeUndefined();
  });
});

describe("formatLocales", () => {
  it("formats with the system's and the account's regional tags of the shown language", () => {
    expect(formatLocales("en", ["en-GB", "de-DE", "en-US", "en-GB"])).toEqual([
      "en-GB",
      "en-US",
      "en",
    ]);
    expect(formatLocales("en-XA", ["en-GB"])).toEqual(["en-GB", "en-XA"]);
    expect(formatLocales("fr", ["en-GB", undefined, ""])).toEqual(["fr"]);
  });
});
