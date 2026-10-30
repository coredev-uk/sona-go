import { t } from "@lingui/core/macro";

/**
 * The Settings rows a changed setting belongs to, by Go's dotted settings
 * name (SystemService.ImportSettings' `changed`). A name matches its
 * longest listed prefix, so every field of a group (the equaliser) reads as
 * one row.
 */
const rowTitles: Record<string, () => string> = {
  theme: () => t`Theme`,
  reduceEffects: () => t`Reduce effects`,
  "motionArtwork.enabled": () => t`Motion artwork`,
  audioQuality: () => t`Audio quality`,
  engineIdleMinutes: () => t`Stop the playback host when idle`,
  autoPlay: () => t`AutoPlay`,
  "audio.soundCheck": () => t`Sound Check`,
  "audio.equalizer": () => t`Equaliser`,
  crossfade: () => t`Crossfade`,
  "queue.replaceConfirm": () => t`Replacing your queue`,
  "desktop.closeAction": () => t`When the window is closed`,
  "desktop.startHidden": () => t`Start in the tray`,
  "desktop.startPage": () => t`Open on`,
  "desktop.resumeLastRoute": () => t`Resume where you left off`,
  "integrations.notifications": () => t`Track notifications`,
  "integrations.inhibitSleep": () => t`Stay awake while playing`,
  "integrations.discordPresence": () => t`Discord status`,
  shortcuts: () => t`Keyboard shortcuts`,
  "storage.artCacheMiB": () => t`Artwork cache limit`,
  "updates.check": () => t`Check for updates`,
};

function rowTitle(name: string): string {
  for (let key = name; key; key = key.slice(0, Math.max(0, key.lastIndexOf(".")))) {
    const title = rowTitles[key];
    if (title) return title();
  }
  return name;
}

/** The titles of the Settings rows the changed names belong to, once each, in order. */
export function changedRows(changed: readonly string[]): string[] {
  return [...new Set(changed.map(rowTitle))];
}
