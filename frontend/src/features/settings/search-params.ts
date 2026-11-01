import { z } from "zod";

/** Settings' sections, in the order the page lists them. */
export const settingsSections = [
  "account",
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
