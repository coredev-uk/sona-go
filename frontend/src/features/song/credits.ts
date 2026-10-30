import type { CreditGroup } from "@bindings/services";

/** Sona's words for Apple's credit categories (Apple sends upper-case titles). */
const titles: Record<string, string> = {
  performer: "Performers",
  "composer-and-lyrics": "Composition and lyrics",
  "production-and-engineering": "Production and engineering",
};

/** A credit group's heading: Sona's title for known kinds, else Apple's in sentence case. */
export function creditTitle(g: Pick<CreditGroup, "kind" | "title">): string {
  const known = titles[g.kind];
  if (known) return known;
  const t = g.title.trim().toLowerCase().replaceAll("&", "and");
  return t ? t[0].toUpperCase() + t.slice(1) : "Credits";
}
