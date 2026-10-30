import { t } from "@lingui/core/macro";
import type { CreditGroup } from "@bindings/services";

/** Sona's words for Apple's credit categories (Apple sends upper-case titles). */
const titles: Record<string, () => string> = {
  performer: () => t`Performers`,
  "composer-and-lyrics": () => t`Composition and lyrics`,
  "production-and-engineering": () => t`Production and engineering`,
};

/** A credit group's heading: Sona's title for known kinds, else Apple's in sentence case. */
export function creditTitle(g: Pick<CreditGroup, "kind" | "title">): string {
  const known = titles[g.kind];
  if (known) return known();
  const s = g.title.trim().toLowerCase().replaceAll("&", "and");
  return s ? s[0].toUpperCase() + s.slice(1) : t`Credits`;
}
