import type { AppleTranslations } from "@bindings/services";
import { generateMessageId } from "@lingui/message-utils/generateMessageId";
import { afterEach, describe, expect, it, vi } from "vitest";

const add = generateMessageId("Add");
const about = generateMessageId("About {artistName}");
const albums = generateMessageId("Albums");

function apple(
  locale: string,
  coverage: number,
  messages: Record<string, string>,
): AppleTranslations {
  return { locale, coverage, messages, version: "translations~x.js", fetchedAt: 1 };
}

/**
 * A fresh i18n module with Go's TranslationsService and events faked;
 * `serve` answers TranslationsService.Apple, `emit` sends
 * "translations:apple".
 */
async function boot(serve: (tag: string) => AppleTranslations, languages = ["fr-FR", "en-US"]) {
  vi.resetModules();
  const handlers: (() => void)[] = [];
  const asked: string[] = [];
  vi.doMock("@bindings/services", () => ({
    TranslationsService: {
      Apple: (tag: string) => {
        asked.push(tag);
        return Promise.resolve(serve(tag));
      },
    },
  }));
  vi.doMock("@wailsio/runtime", () => ({
    Events: {
      On: (_name: string, fn: () => void) => {
        handlers.push(fn);
        return () => {};
      },
    },
  }));
  vi.spyOn(navigator, "languages", "get").mockReturnValue(languages);
  const { i18n } = await import("@lingui/core");
  const mod = await import("./index");
  const revision = await import("./revision");
  mod.initI18n();
  mod.startTranslations();
  return { i18n, mod, revision, asked, emit: () => handlers.forEach((h) => h()) };
}

afterEach(() => {
  vi.doUnmock("@bindings/services");
  vi.doUnmock("@wailsio/runtime");
  vi.restoreAllMocks();
});

describe("Apple's runtime translations", () => {
  it("layers Apple's messages over English once the locale is complete enough", async () => {
    const { i18n } = await boot(() =>
      apple("fr-FR", 0.95, { [add]: "Ajouter", [about]: "À propos de {artistName}" }),
    );
    expect(i18n.locale).toBe("en");
    await vi.waitFor(() => expect(i18n.locale).toBe("fr-FR"));
    expect(i18n._(add)).toBe("Ajouter");
    expect(i18n._(about, { artistName: "Björk" })).toBe("À propos de Björk");
    expect(i18n._(albums)).toBe("Albums"); // English fills in
    expect(document.documentElement.lang).toBe("fr-FR");
  });

  it("keeps English below the threshold, then follows a download that completes it", async () => {
    let coverage = 98 / 1415;
    const { i18n, asked, emit } = await boot(() => apple("fr-FR", coverage, { [add]: "Ajouter" }));
    await vi.waitFor(() => expect(asked).toEqual(["fr-FR"]));
    expect(i18n.locale).toBe("en");
    expect(i18n._(add)).toBe("Add");

    coverage = 0.9;
    emit();
    await vi.waitFor(() => expect(i18n.locale).toBe("fr-FR"));
    expect(i18n._(add)).toBe("Ajouter");
  });

  it("never asks for English preferences", async () => {
    vi.useFakeTimers();
    try {
      const { asked, i18n } = await boot(() => apple("fr-FR", 1, {}), ["en-GB", "fr-FR"]);
      await vi.advanceTimersByTimeAsync(300); // past startTranslations' first-paint wait
      expect(asked).toEqual([]);
      expect(i18n.locale).toBe("en");
    } finally {
      vi.useRealTimers();
    }
  });

  it("remounts on an update of the shown locale and drops what Lingui cannot compile", async () => {
    let text = "Ajouter";
    const { i18n, revision, emit } = await boot(() =>
      apple("fr-FR", 0.95, { [add]: text, [albums]: "{broken" }),
    );
    await vi.waitFor(() => expect(i18n._(add)).toBe("Ajouter"));
    expect(i18n._(albums)).toBe("Albums");
    const before = revision.getRevision();
    text = "Ajouter à";
    emit();
    await vi.waitFor(() => expect(i18n._(add)).toBe("Ajouter à"));
    expect(revision.getRevision()).toBe(before + 1);
  });
});
