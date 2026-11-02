import { describe, expect, it } from "vitest";

import { languageOptions, nativeName, offersLanguages } from "./languages";

const apple = (...locales: [string, number][]) => ({
  mapped: Math.max(0, ...locales.map(([, c]) => c)),
  locales: locales.map(([tag, coverage]) => ({ tag, coverage, measured: true })),
});
const tags = (o: { tag: string }[]) => o.map((x) => x.tag);

describe("languageOptions", () => {
  it("offers English alone while no locale reaches the selectable share", () => {
    const options = languageOptions(apple(["fr-FR", 0.07], ["de-DE", 0.07]), [], "", "en");
    expect(tags(options)).toEqual(["en"]);
    expect(offersLanguages(options, "")).toBe(false);
    expect(tags(languageOptions(undefined, [], "", "en"))).toEqual(["en"]);
  });

  it("offers Apple's locales from 90%, named in themselves and sorted", () => {
    const options = languageOptions(
      apple(["fr-FR", 0.95], ["de-DE", 0.9], ["ja-JP", 0.89]),
      [],
      "",
      "en",
    );
    expect(tags(options)).toEqual(["de-DE", "en", "fr-FR"]);
    expect(options.map((o) => o.name)).toEqual([
      "Deutsch (Deutschland)",
      "English",
      "Français (France)",
    ]);
    expect(offersLanguages(options, "")).toBe(true);
  });

  it("adds a community catalogue of the same language to Apple's coverage", () => {
    const options = languageOptions(apple(["pt-BR", 0.6]), [{ tag: "pt", share: 0.35 }], "", "en");
    expect(tags(options)).toEqual(["en", "pt-BR"]);
  });

  it("offers a complete community catalogue Apple has no locale for, once", () => {
    const options = languageOptions(
      apple(["fr-FR", 0.95]),
      [
        { tag: "cy", share: 0.92 },
        { tag: "fr", share: 0.95 },
        { tag: "eo", share: 0.5 },
      ],
      "",
      "en",
    );
    expect(tags(options)).toEqual(["cy", "en", "fr-FR"]);
  });

  it("keeps the chosen language listed, so the setting can be read and changed", () => {
    const options = languageOptions(apple(["fr-FR", 0.07]), [], "fr-FR", "en");
    expect(tags(options)).toEqual(["en", "fr-FR"]);
    expect(offersLanguages(options, "fr-FR")).toBe(true);
    expect(tags(languageOptions(apple(["fr-FR", 0.95]), [], "FR-fr", "en"))).toEqual([
      "en",
      "fr-FR",
    ]);
  });
});

describe("nativeName", () => {
  it("names a language in itself, capitalised, and falls back to the tag", () => {
    expect(nativeName("fr-CA")).toBe("Français (Canada)");
    expect(nativeName("en")).toBe("English");
    expect(nativeName("not a tag")).toBe("not a tag");
  });
});
