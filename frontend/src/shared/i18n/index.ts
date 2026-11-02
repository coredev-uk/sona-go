import { type AppleTranslations, TranslationsService } from "@bindings/services";
import { i18n, type Messages } from "@lingui/core";
import { Events } from "@wailsio/runtime";

import { messages as sourceMessages } from "./locales/en.po";
import {
  chooseTranslation,
  formatLocales,
  matchLocale,
  selectableShare,
  sourceLocale,
  type Translation,
  translatedShare,
} from "./locale";
import { bumpRevision } from "./revision";

export { I18nRoot } from "./I18nRoot";
export { type LanguageOption, languageOptions, nativeName, offersLanguages } from "./languages";
export { pseudoLocale, selectableShare, sourceLocale } from "./locale";

type Catalogue = { messages: Messages };
type Loaders = Record<string, () => Promise<Catalogue>>;

/** Loaders by locale tag, from `import.meta.glob` paths ("./community/fr.po"). */
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
 * locales"). English is bundled; the pseudo-locale and the community
 * catalogues (`community/`) are chunks loaded on demand. Apple Music's own
 * translations are not in the bundle: Go fetches them from Apple at run
 * time and serves them through `TranslationsService.Apple`.
 */
const pseudo = byTag(import.meta.glob<Catalogue>(["./locales/*.po", "!./locales/en.po"]));
const community = byTag(import.meta.glob<Catalogue>("./community/*.po"));
const sourceIds = Object.keys(sourceMessages);

/** `VITE_SONA_LOCALE` (`pnpm dev:pseudo`, `pnpm build:pseudo`) forces the pseudo-locale. */
const forced = import.meta.env.VITE_SONA_LOCALE;
const forcedPseudo = forced && pseudo.has(forced) ? forced : undefined;
let account = "";
/** The Language setting: "" (Automatic) or the tag the user chose. */
let setting = "";
let request = 0;
let started = false;
let shown: Messages = sourceMessages;

/** Shows `locale`'s messages; `Intl` formats with the regional tags of its language. */
function show(locale: string, messages: Messages): void {
  const locales = formatLocales(locale, [setting, ...navigator.languages, account]);
  const sameLocale = locale === i18n.locale && locales.join() === i18n.locales?.toString();
  if (sameLocale && messages === shown) return;
  shown = messages;
  i18n.loadAndActivate({ locale, locales, messages });
  document.documentElement.lang = locales[0];
  // Same locale, new catalogue (Apple's update): remount like a switch.
  if (sameLocale) bumpRevision();
}

/** Apple's ICU messages compiled for Lingui; one that fails to compile is left out. */
async function compileApple(apple: AppleTranslations): Promise<Messages> {
  const entries = Object.entries(apple.messages);
  if (!entries.length) return {};
  // Loaded only when a translation arrives: English never pays for the parser.
  const { compileMessageOrThrow } = await import("@lingui/message-utils/compileMessage");
  const out: Messages = {};
  for (const [id, text] of entries) {
    if (text === undefined) continue;
    try {
      out[id] = compileMessageOrThrow(text);
    } catch {
      // Go checked the placeholders; anything Lingui cannot read stays English.
    }
  }
  return out;
}

/** A preferred tag's layers: Apple's translation, then the community catalogue, over English. */
async function loadTranslation(tag: string): Promise<Translation | undefined> {
  const communityTag = matchLocale([tag], [...community.keys()]);
  const [apple, shared] = await Promise.all([
    TranslationsService.Apple(tag).catch(() => undefined),
    communityTag ? community.get(communityTag)!().then((c) => c.messages) : undefined,
  ]);
  const own = apple?.locale ? await compileApple(apple) : {};
  const locale = apple?.locale || communityTag;
  if (!locale) return undefined;
  return {
    locale,
    share: translatedShare(apple?.coverage ?? 0, own, shared, sourceIds),
    messages: { ...sourceMessages, ...shared, ...own },
  };
}

/**
 * Shows the language chosen in Settings (however complete), else, under
 * Automatic, the preferred translation that is complete enough, else
 * English.
 */
async function retranslate(): Promise<void> {
  const ticket = ++request;
  let next: Translation | undefined;
  try {
    next = setting
      ? await chooseTranslation([setting], loadTranslation, 0)
      : await chooseTranslation([account, ...navigator.languages], loadTranslation);
  } catch {
    next = undefined;
  }
  // A later choice (the setting, the account's language, a download) supersedes this one.
  if (ticket !== request) return;
  if (next) show(next.locale, next.messages);
  else show(sourceLocale, sourceMessages);
}

/**
 * The share of Sona's messages each community catalogue translates, for
 * the Language setting's options (loads the catalogues' chunks).
 */
export async function communityShares(): Promise<{ tag: string; share: number }[]> {
  return Promise.all(
    [...community].map(async ([tag, load]) => {
      const { messages } = await load();
      const share = sourceIds.filter((id) => id in messages).length / (sourceIds.length || 1);
      return { tag, share };
    }),
  );
}

/**
 * The share of Sona's messages Apple's layer alone must reach for a locale
 * to be selectable: all of `selectableShare` without community catalogues,
 * else none (one may make up the rest), so Go knows when to list Apple's
 * locales at all (`TranslationsService.Languages`).
 */
export function appleShareNeeded(): number {
  return community.size ? 0 : selectableShare;
}

/**
 * Activates the locale before the first render: English, bundled, at
 * once. The forced pseudo-locale loads first, and `I18nRoot` renders
 * nothing until it is active.
 */
export function initI18n(): void {
  if (forcedPseudo) {
    void pseudo.get(forcedPseudo)!().then(
      (c) => show(forcedPseudo, c.messages),
      () => show(sourceLocale, sourceMessages),
    );
    return;
  }
  show(sourceLocale, sourceMessages);
}

/**
 * After the first paint, looks for a translation of the preferred
 * languages (Apple's from Go's cache, plus the community catalogue) and
 * follows Go's downloads ("translations:apple"). A locale is shown once
 * its layers translate `selectableShare` of Sona's messages.
 */
export function startTranslations(): void {
  if (forcedPseudo || started) return;
  started = true;
  let done = false;
  const start = () => {
    if (done) return;
    done = true;
    Events.On("translations:apple", () => void retranslate());
    void retranslate();
  };
  // Two frames: the first runs before paint, the second after it; a timer
  // covers a window that starts hidden (WebKit throttles its frames).
  requestAnimationFrame(() => requestAnimationFrame(start));
  window.setTimeout(start, 300);
}

/**
 * Follows the account's storefront language (`AuthStatus.language`, e.g.
 * "en-GB"), preferred over the system's when a complete enough
 * translation exists; "" falls back to the system's. The account's tag
 * also refines formatting when the system has no tag of the shown
 * language.
 */
export function followAccountLanguage(tag: string): void {
  if (tag === account || forcedPseudo) return;
  account = tag;
  if (started) void retranslate();
  else show(i18n.locale || sourceLocale, shown);
}

/**
 * Follows the Language setting (Settings → General): "" is Automatic (the
 * account's language, then the system's); a tag shows that language
 * whenever Sona has any of it, English otherwise.
 */
export function followLanguageSetting(tag: string): void {
  if (tag === setting || forcedPseudo) return;
  setting = tag;
  if (started) void retranslate();
}
