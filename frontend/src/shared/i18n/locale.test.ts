import { describe, expect, it } from "vitest";

import {
  catalogueLayers,
  chooseLocale,
  type Completeness,
  formatLocales,
  matchLocale,
  selectableLocales,
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

describe("chooseLocale", () => {
  const catalogues = ["en", "en-XA", "fr"];

  it("follows the account's storefront language over the system", () => {
    expect(chooseLocale({ account: "fr-FR", system: ["en-US"] }, catalogues)).toBe("fr");
  });

  it("falls back to the system languages, then English, when the account has no catalogue", () => {
    expect(chooseLocale({ account: "ja-JP", system: ["de-DE", "fr-CA"] }, catalogues)).toBe("fr");
    expect(chooseLocale({ account: "ja-JP", system: ["de-DE"] }, catalogues)).toBe("en");
  });

  it("never picks the pseudo-locale unless forced", () => {
    expect(chooseLocale({ account: "en-XA", system: ["en-XA"] }, ["en-XA", "fr"])).toBe("en");
    expect(chooseLocale({ forced: "en-XA", account: "fr", system: [] }, catalogues)).toBe("en-XA");
  });

  it("ignores a forced locale without a catalogue", () => {
    expect(chooseLocale({ forced: "xx", account: "fr", system: [] }, catalogues)).toBe("fr");
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

describe("selectableLocales", () => {
  const locale = (share: number) => ({ apple: 0, community: 0, share, selectable: share >= 0.9 });

  it("offers only the locales at the threshold or above", () => {
    const completeness: Completeness = {
      messages: 100,
      threshold: 0.9,
      locales: { "fr-FR": locale(0.95), "de-DE": locale(0.9), ja: locale(0.89) },
    };
    expect(selectableLocales(completeness)).toEqual(["de-DE", "fr-FR"]);
  });

  it("offers none without a completeness file", () => {
    expect(selectableLocales(undefined)).toEqual([]);
  });
});

describe("catalogueLayers", () => {
  it("takes Apple's catalogue for the same tag and the community one for the language", () => {
    expect(catalogueLayers("de-DE", ["de-CH", "de-DE"], ["de"])).toEqual({
      apple: "de-DE",
      community: "de",
    });
  });

  it("leaves out a layer the locale has no catalogue in", () => {
    expect(catalogueLayers("eo", ["de-DE"], ["eo"])).toEqual({ apple: undefined, community: "eo" });
    expect(catalogueLayers("pt-BR", ["pt-BR", "pt-PT"], [])).toEqual({
      apple: "pt-BR",
      community: undefined,
    });
  });
});
