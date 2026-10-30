/** BCP 47 tag of the source catalogue: message IDs come from its text. */
export const sourceLocale = "en";

/**
 * The pseudo-locale (accented, stretched English). It is used only when
 * forced through `VITE_SONA_LOCALE`, never matched from a preference.
 */
export const pseudoLocale = "en-XA";

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
 * The locale to show: a forced one when it has a catalogue, else the
 * account's storefront language, else the system's languages, else English.
 */
export function chooseLocale(
  { forced, account, system }: { forced?: string; account?: string; system: readonly string[] },
  catalogues: readonly string[],
): string {
  if (forced && catalogues.includes(forced)) return forced;
  const selectable = catalogues.filter((l) => l !== pseudoLocale);
  return matchLocale([account], selectable) ?? matchLocale(system, selectable) ?? sourceLocale;
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
