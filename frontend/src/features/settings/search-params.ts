import { z } from "zod";

/** Settings' sections, in the order the page lists them. */
export const settingsSections = [
  "general",
  "playback",
  "appearance",
  "connections",
  "shortcuts",
  "storage",
  "diagnostics",
] as const;
export type SettingsSection = (typeof settingsSections)[number];

/**
 * /settings?section=…. Without one, or with one that doesn't exist, the
 * page opens the section shown last in this run (at first, the first one).
 */
export const settingsSearch = z.object({
  section: z.enum(settingsSections).optional().catch(undefined),
});

/**
 * Whether a /settings link names the retired Account section (a saved
 * "last page", an old link): the account lives in the top bar's account
 * menu now, so the route opens the account dialog instead.
 */
export function namesAccountSection(search: unknown): boolean {
  return (
    typeof search === "object" &&
    search !== null &&
    "section" in search &&
    search.section === "account"
  );
}
