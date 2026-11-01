import { i18n, type Messages } from "@lingui/core";

import { messages as sourceMessages } from "./locales/en.po";
import {
  catalogueLayers,
  chooseLocale,
  type Completeness,
  formatLocales,
  selectableLocales,
  sourceLocale,
} from "./locale";

export { I18nRoot } from "./I18nRoot";
export { pseudoLocale, sourceLocale } from "./locale";

type Catalogue = { messages: Messages };
type Loaders = Record<string, () => Promise<Catalogue>>;

/** Loaders by locale tag, from `import.meta.glob` paths ("./apple/fr-FR.po"). */
function byTag(loaders: Loaders): Map<string, Loaders[string]> {
  return new Map(
    Object.entries(loaders).map(([path, load]) => [
      path.slice(path.lastIndexOf("/") + 1, -3),
      load,
    ]),
  );
}

/**
 * Message catalogues (docs/frontend-architecture.md, "Strings and
 * locales"). English is bundled; every other catalogue is its own chunk
 * loaded on demand. A translated locale layers Apple Music's own
 * translations (`apple/`, from `cmd/i18ntool import`) over the community
 * catalogue (`community/`) over English, and is offered once
 * `completeness.json` says it is complete enough.
 */
const pseudo = byTag(import.meta.glob<Catalogue>(["./locales/*.po", "!./locales/en.po"]));
const apple = byTag(import.meta.glob<Catalogue>("./apple/*.po"));
const community = byTag(import.meta.glob<Catalogue>("./community/*.po"));
const completeness = Object.values(
  import.meta.glob<Completeness>("./completeness.json", { eager: true, import: "default" }),
)[0];

/** The translated locales offered (for the Language setting). */
export const translatedLocales: readonly string[] = selectableLocales(completeness);

const catalogues = [sourceLocale, ...pseudo.keys(), ...translatedLocales];

async function loadMessages(locale: string): Promise<Messages> {
  const load = pseudo.get(locale);
  if (load) return (await load()).messages;
  const layers = catalogueLayers(locale, [...apple.keys()], [...community.keys()]);
  const [own, shared] = await Promise.all([
    layers.apple ? apple.get(layers.apple)!() : undefined,
    layers.community ? community.get(layers.community)!() : undefined,
  ]);
  return { ...sourceMessages, ...shared?.messages, ...own?.messages };
}

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
      messages = await loadMessages(locale);
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
