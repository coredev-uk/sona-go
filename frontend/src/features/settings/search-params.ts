import { z } from "zod";

export const settingsSections = [
  "account",
  "playback",
  "appearance",
  "desktop",
  "connections",
  "shortcuts",
  "storage",
  "diagnostics",
] as const;
export type SettingsSection = (typeof settingsSections)[number];

/** /settings?section=… — unknown or missing sections fall back to Account. */
export const settingsSearch = z.object({
  section: z.enum(settingsSections).catch("account"),
});
