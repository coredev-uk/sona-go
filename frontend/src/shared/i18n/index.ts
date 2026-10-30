import { i18n, type Messages } from "@lingui/core";

import { messages as sourceMessages } from "./locales/en.po";
import { chooseLocale, formatLocales, sourceLocale } from "./locale";

export { I18nRoot } from "./I18nRoot";
export { pseudoLocale, sourceLocale } from "./locale";

/**
 * Message catalogues (docs/frontend-architecture.md, "Strings and
 * locales"). English is bundled; every other catalogue, the pseudo-locale
 * included, is its own chunk loaded on demand.
 */
const loaders = import.meta.glob<{ messages: Messages }>(["./locales/*.po", "!./locales/en.po"]);
const lazy = new Map(
  Object.entries(loaders).map(([path, load]) => [path.slice("./locales/".length, -3), load]),
);
const catalogues = [sourceLocale, ...lazy.keys()];

/** `VITE_SONA_LOCALE` (`pnpm dev:pseudo`, `pnpm build:pseudo`) overrides the choice. */
const forced = import.meta.env.VITE_SONA_LOCALE;
let account = "";
let request = 0;

function wanted(): string {
  return chooseLocale({ forced, account, system: navigator.languages }, catalogues);
}

/** Shows `locale`'s messages; `Intl` formats with the regional tags of its language. */
function show(locale: string, messages: Messages): void {
  const locales = formatLocales(locale, [...navigator.languages, account]);
  i18n.loadAndActivate({ locale, locales, messages });
  document.documentElement.lang = locales[0];
}

async function activate(locale: string): Promise<void> {
  const ticket = ++request;
  let messages = sourceMessages;
  if (locale !== sourceLocale) {
    try {
      messages = (await lazy.get(locale)!()).messages;
    } catch {
      locale = sourceLocale;
    }
  }
  // A later choice (the account's language arriving) supersedes this one.
  if (ticket === request) show(locale, messages);
}

/**
 * Activates the locale before the first render. English, the common case,
 * is bundled and activates synchronously; another catalogue loads first,
 * and `I18nRoot` renders nothing until it is active.
 */
export function initI18n(): void {
  const locale = wanted();
  if (locale === sourceLocale) show(locale, sourceMessages);
  else void activate(locale);
}

/**
 * Follows the account's storefront language (`AuthStatus.language`, e.g.
 * "en-GB") when a catalogue exists for it; "" falls back to the system's.
 * The account's tag also refines formatting when the system has no tag of
 * the shown language.
 */
export function followAccountLanguage(tag: string): void {
  if (tag === account) return;
  account = tag;
  const locale = wanted();
  const locales = formatLocales(locale, [...navigator.languages, account]);
  if (locale !== i18n.locale || locales.join() !== i18n.locales?.toString()) {
    void activate(locale);
  }
}
