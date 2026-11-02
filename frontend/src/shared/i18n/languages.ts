import type { AppleLanguages } from "@bindings/services";

import { matchLocale, selectableShare, sourceLocale } from "./locale";

/** One language the Language setting offers (besides Automatic). */
export interface LanguageOption {
  /** The setting's value: a BCP 47 tag ("en" for English). */
  tag: string;
  /** The language's name in itself ("Français (Canada)"), so anyone finds their own. */
  name: string;
}

/** A community catalogue and the share of Sona's messages it translates. */
export interface CommunityShare {
  tag: string;
  share: number;
}

/** A language's name in itself, capitalised as a list item starts; the tag when unknown. */
export function nativeName(tag: string): string {
  let name: string | undefined;
  try {
    name = new Intl.DisplayNames([tag], { type: "language", languageDisplay: "standard" }).of(tag);
  } catch {
    name = undefined;
  }
  if (!name || name === tag) return tag;
  const [first = ""] = name;
  return first.toLocaleUpperCase(tag) + name.slice(first.length);
}

/**
 * The languages the Language setting offers: English, and every locale
 * whose layers reach `selectableShare`: Apple's locales (by their counted
 * coverage, or the mapped share bounding it until one is downloaded) with
 * the community catalogue of the same language added, and community
 * catalogues Apple has no locale for. The chosen tag (`current`) stays
 * listed even when it no longer qualifies, so it can be read and changed.
 * Sorted by name in the shown locale's collation.
 */
export function languageOptions(
  apple: AppleLanguages | undefined,
  community: readonly CommunityShare[],
  current: string,
  collation: string,
): LanguageOption[] {
  const communityTags = community.map((c) => c.tag);
  const tags: string[] = [sourceLocale];
  const appleTags = (apple?.locales ?? []).map((l) => l.tag);
  for (const l of apple?.locales ?? []) {
    const sibling = matchLocale([l.tag], communityTags);
    const extra = community.find((c) => c.tag === sibling)?.share ?? 0;
    if (Math.min(1, l.coverage + extra) >= selectableShare) tags.push(l.tag);
  }
  for (const c of community) {
    if (c.share >= selectableShare && !matchLocale([c.tag], appleTags)) tags.push(c.tag);
  }
  if (current) tags.push(current);

  const seen = new Set<string>();
  const out: LanguageOption[] = [];
  for (const tag of tags) {
    const key = tag.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({ tag, name: nativeName(tag) });
  }
  const collator = new Intl.Collator(collation);
  return out.sort((a, b) => collator.compare(a.name, b.name));
}

/**
 * Whether the Language setting is offered: once a language besides
 * English is selectable, or while one is chosen (so it can be changed back).
 */
export function offersLanguages(options: readonly LanguageOption[], current: string): boolean {
  return current !== "" || options.some((o) => o.tag !== sourceLocale);
}
