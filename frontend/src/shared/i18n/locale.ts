import type { Messages } from "@lingui/core";

/** BCP 47 tag of the source catalogue: message IDs come from its text. */
export const sourceLocale = "en";

/**
 * The pseudo-locale (accented, stretched English). It is used only when
 * forced through `VITE_SONA_LOCALE`, never matched from a preference.
 */
export const pseudoLocale = "en-XA";

/**
 * How much of Sona's messages a locale's layers must translate before Sona
 * shows it. Less complete ones would show mostly English.
 */
export const selectableShare = 0.9;

const language = (tag: string) => tag.split("-")[0];

/**
 * The catalogue for the first preferred tag that has one: an exact match
 * ("pt-BR"), else one for the same language ("en-GB" → "en", "pt-BR" →
 * "pt-PT"). Matching ignores case; empty preferences are skipped.
 */
export function matchLocale(
  preferred: readonly (string | undefined)[],
  available: readonly string[],
): string | undefined {
  const lower = available.map((a) => a.toLowerCase());
  for (const tag of preferred) {
    if (!tag) continue;
    const want = tag.toLowerCase();
    let i = lower.indexOf(want);
    if (i < 0) i = lower.indexOf(language(want));
    if (i < 0) i = lower.findIndex((a) => language(a) === language(want));
    if (i >= 0) return available[i];
  }
  return undefined;
}

/**
 * The preferred tags (the account's storefront language, then the
 * system's) to look for a translation of, in order: up to the first
 * English one, since English, the source, answers it. Empty and repeated
 * tags are skipped.
 */
export function translationCandidates(preferred: readonly (string | undefined)[]): string[] {
  const out: string[] = [];
  for (const tag of preferred) {
    if (!tag) continue;
    const lower = tag.toLowerCase();
    if (language(lower) === sourceLocale) break;
    if (!out.some((t) => t.toLowerCase() === lower)) out.push(tag);
  }
  return out;
}

/**
 * The share of Sona's messages a locale translates: Apple's layer's
 * coverage (counted by Go against the same catalogue) plus the community
 * catalogue's messages Apple's layer lacks.
 */
export function translatedShare(
  appleCoverage: number,
  apple: Messages,
  community: Messages | undefined,
  source: readonly string[],
): number {
  if (!source.length) return 0;
  const extra = community ? source.filter((id) => id in community && !(id in apple)).length : 0;
  return appleCoverage + extra / source.length;
}

/** A locale's catalogue: its layers merged over English. */
export interface Translation {
  locale: string;
  share: number;
  messages: Messages;
}

/**
 * The translation to show: the first preferred locale (translationCandidates)
 * whose layers translate something and reach `minShare` (`selectableShare`
 * when Sona chooses; 0 for the language the user chose in Settings, which
 * is shown however complete it is); undefined shows English.
 */
export async function chooseTranslation(
  preferred: readonly (string | undefined)[],
  load: (tag: string) => Promise<Translation | undefined>,
  minShare = selectableShare,
): Promise<Translation | undefined> {
  for (const tag of translationCandidates(preferred)) {
    const t = await load(tag);
    if (t && t.share > 0 && t.share >= minShare) return t;
  }
  return undefined;
}

/**
 * The locales `Intl` formats with while `locale`'s catalogue is shown: the
 * system's and then the account's tags of the same language, so an English
 * UI on an en-GB system keeps 24-hour clocks and day-month dates, then the
 * catalogue's own tag.
 */
export function formatLocales(
  locale: string,
  preferred: readonly (string | undefined)[],
): string[] {
  const lang = language(locale.toLowerCase());
  const same = preferred.filter(
    (tag): tag is string => !!tag && language(tag.toLowerCase()) === lang,
  );
  return [...new Set([...same, locale])];
}
