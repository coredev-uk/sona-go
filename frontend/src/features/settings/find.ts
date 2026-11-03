import { t } from "@lingui/core/macro";
import type { Settings } from "@bindings/config";
import type { Capabilities } from "@bindings/services";

import { groupTitle } from "@/shared/shortcuts/groups";
import { registeredShortcuts } from "@/shared/shortcuts/registry";

import { settingsSections, type SettingsSection } from "./search-params";

/** A section's name, in the section list, its heading and Find a setting's results. */
export function sectionTitle(section: SettingsSection): string {
  switch (section) {
    case "general":
      return t`General`;
    case "playback":
      return t`Playback`;
    case "appearance":
      return t`Appearance`;
    case "connections":
      return t`Connections`;
    case "shortcuts":
      return t`Shortcuts`;
    case "storage":
      return t`Storage`;
    case "diagnostics":
      return t`Diagnostics`;
  }
}

/** What decides whether a row is on its section at all. */
export interface FindContext {
  caps: Capabilities | undefined;
  settings: Settings | undefined;
  /** Settings → General shows its Language row (a second language is selectable). */
  languageRow: boolean;
}

/**
 * One setting Find a setting can land on. `id` is the row's `data-setting`
 * (the control's id for a SettingRow); `title` is the row's own title, so
 * a result reads as the row it opens.
 */
export interface SettingEntry {
  id: string;
  section: SettingsSection;
  group: string;
  title: string;
  /** Other words people search for it by. */
  keywords?: string;
  /** Rows a section shows only in some states (not for capabilities: those rows say why they're unavailable). */
  when?: (c: FindContext) => boolean;
}

const playable = (c: FindContext) => c.caps?.playback.available ?? false;
const cdmEngine = (c: FindContext) => playable(c) && c.caps?.playback.engine === "cdm";

/**
 * Every setting on the Settings page, in page order, then every shortcut.
 * A test renders each section and checks that each entry is there.
 */
export function settingIndex(): SettingEntry[] {
  const general = sectionTitle("general");
  const starting = t`Starting Sona`;
  const window = t`Window`;
  const playing = t`While music plays`;
  const updates = t`Updates`;
  const sound = t`Sound`;
  const listening = t`Listening`;
  const lyrics = t`Lyrics`;
  const advanced = t`Advanced`;
  const appearance = sectionTitle("appearance");
  const scrobbling = t`Scrobbling`;
  const discord = t`Discord`;
  const storage = sectionTitle("storage");
  const diagnostics = sectionTitle("diagnostics");
  const rendering = t`Rendering`;

  const entries: SettingEntry[] = [
    {
      id: "language",
      section: "general",
      group: general,
      title: t`Language`,
      when: (c) => c.languageRow,
    },
    {
      id: "autostart",
      section: "general",
      group: starting,
      title: t`Start Sona when you log in`,
      keywords: t`login, startup, autostart`,
    },
    {
      id: "start-hidden",
      section: "general",
      group: starting,
      title: t`Start in the tray`,
      when: (c) => !!c.caps?.autostart && !!c.caps.tray && (c.settings?.desktop.autostart ?? false),
    },
    {
      id: "start-page",
      section: "general",
      group: starting,
      title: t`Open on`,
      keywords: t`start page, resume`,
    },
    {
      id: "setup-again",
      section: "general",
      group: starting,
      title: t`First-run setup`,
      keywords: t`Set up Sona again, welcome`,
    },
    {
      id: "close-action",
      section: "general",
      group: window,
      title: t`When the window is closed`,
      keywords: t`tray, background, quit`,
    },
    {
      id: "title-bar",
      section: "general",
      group: window,
      title: t`Title bar`,
      keywords: t`window buttons, decorations`,
      when: (c) => c.caps?.windowControls.native === "frame",
    },
    { id: "window-size", section: "general", group: window, title: t`Window size` },
    {
      id: "track-notifications",
      section: "general",
      group: playing,
      title: t`Track notifications`,
    },
    {
      id: "inhibit-sleep",
      section: "general",
      group: playing,
      title: t`Stay awake while playing`,
      keywords: t`sleep, suspend`,
    },
    {
      id: "updates-check",
      section: "general",
      group: updates,
      title: t`Check for updates when Sona starts`,
    },
    {
      id: "cdm",
      section: "playback",
      group: sectionTitle("playback"),
      title: t`Widevine`,
      when: cdmEngine,
    },
    {
      id: "quality",
      section: "playback",
      group: sound,
      title: t`Audio quality`,
      keywords: t`bitrate, data saver, lossless`,
      when: playable,
    },
    {
      id: "output",
      section: "playback",
      group: sound,
      title: t`Output`,
      keywords: t`speakers, headphones, device`,
      when: playable,
    },
    {
      id: "sound-check",
      section: "playback",
      group: sound,
      title: t`Sound Check`,
      keywords: t`loudness, volume`,
      when: playable,
    },
    { id: "equalizer", section: "playback", group: sound, title: t`Equaliser`, when: playable },
    { id: "crossfade", section: "playback", group: sound, title: t`Crossfade`, when: playable },
    {
      id: "crossfade-seconds",
      section: "playback",
      group: sound,
      title: t`Crossfade length`,
      when: (c) => playable(c) && !!c.caps?.playback.crossfade && !!c.settings?.crossfade.enabled,
    },
    {
      id: "private-listening",
      section: "playback",
      group: listening,
      title: t({
        message: "Private listening",
        comment:
          "Apple Music's own name for the mode: plays aren't reported to Apple, so they don't shape recommendations or Replay.",
      }),
      when: playable,
    },
    {
      id: "replace-queue",
      section: "playback",
      group: listening,
      title: t`Replacing your queue`,
      when: playable,
    },
    {
      id: "lyrics-sources",
      section: "playback",
      group: lyrics,
      title: t`Lyrics sources`,
      keywords: t`Apple Music, Musixmatch, LRCLIB, NetEase`,
    },
    {
      id: "lyrics-word-timing",
      section: "playback",
      group: lyrics,
      title: t`Prefer word-by-word lyrics`,
    },
    {
      id: "lyrics-translation",
      section: "playback",
      group: lyrics,
      title: t`Lyrics translation`,
      keywords: t`language`,
    },
    {
      id: "idle",
      section: "playback",
      group: advanced,
      title: t`Free memory when idle`,
      when: cdmEngine,
    },
    {
      id: "theme",
      section: "appearance",
      group: appearance,
      title: t`Theme`,
      keywords: t`dark, light`,
    },
    {
      id: "reduce-effects",
      section: "appearance",
      group: appearance,
      title: t`Reduce effects`,
      keywords: t`animation, motion, blur`,
    },
    {
      id: "motion-artwork",
      section: "appearance",
      group: appearance,
      title: t`Motion artwork`,
      keywords: t`video, animated covers`,
    },
    { id: "scrobble-lastfm", section: "connections", group: scrobbling, title: t`Last.fm` },
    {
      id: "scrobble-listenbrainz",
      section: "connections",
      group: scrobbling,
      title: t`ListenBrainz`,
    },
    { id: "discord-presence", section: "connections", group: discord, title: t`Discord status` },
    {
      id: "art-cache",
      section: "storage",
      group: storage,
      title: t`Artwork cache`,
      keywords: t`disk, space`,
    },
    {
      id: "clear-cache",
      section: "storage",
      group: storage,
      title: t`Catalogue cache`,
      keywords: t`Clear caches, offline`,
    },
    {
      id: "compact",
      section: "storage",
      group: storage,
      title: t`Library database`,
      keywords: t`Compact now`,
    },
    {
      id: "settings-file",
      section: "storage",
      group: storage,
      title: t`Settings file`,
      keywords: t`export, import, backup`,
    },
    {
      id: "diagnostics-copy",
      section: "diagnostics",
      group: diagnostics,
      title: t`Copy diagnostics`,
      keywords: t`bug report`,
    },
    {
      id: "diagnostics-logs",
      section: "diagnostics",
      group: diagnostics,
      title: t`Open log folder`,
      keywords: t`logs`,
    },
    {
      id: "disableDmabufRenderer",
      section: "diagnostics",
      group: rendering,
      title: t`Disable the DMA-BUF renderer`,
      keywords: t`blank window, flicker`,
      when: (c) => c.caps?.webkitGtk ?? false,
    },
    {
      id: "disableCompositingMode",
      section: "diagnostics",
      group: rendering,
      title: t`Disable compositing`,
      keywords: t`software rendering`,
      when: (c) => c.caps?.webkitGtk ?? false,
    },
  ];
  for (const s of registeredShortcuts()) {
    entries.push({
      id: `shortcut-${s.id}`,
      section: "shortcuts",
      group: groupTitle(s.group),
      title: s.label(),
    });
  }
  return entries;
}

/** Lower case without accents, so "equaliser" finds "Égaliseur" and the other way round. */
function fold(s: string, locale: string): string {
  return s.toLocaleLowerCase(locale).normalize("NFD").replace(/\p{M}/gu, "");
}

/**
 * The entries a query finds: every word of it somewhere in the title, the
 * group, the section or the keywords; titles that start with it first,
 * then titles holding it, then the rest, each in page order.
 */
export function findSettings(
  entries: readonly SettingEntry[],
  query: string,
  context: FindContext,
  locale: string,
): SettingEntry[] {
  const q = fold(query.trim(), locale);
  if (!q) return [];
  const words = q.split(/\s+/);
  const rank = (e: SettingEntry) => {
    const title = fold(e.title, locale);
    if (title.startsWith(q)) return 0;
    if (title.split(/\s+/).some((w) => w.startsWith(words[0]))) return 1;
    if (title.includes(q)) return 2;
    return 3;
  };
  const order = (e: SettingEntry) => settingsSections.indexOf(e.section);
  return entries
    .filter((e) => !e.when || e.when(context))
    .filter((e) => {
      const hay = fold(
        [e.title, e.group, sectionTitle(e.section), e.keywords ?? ""].join(" "),
        locale,
      );
      return words.every((w) => hay.includes(w));
    })
    .map((e, i) => ({ e, i, r: rank(e) }))
    .sort((a, b) => a.r - b.r || order(a.e) - order(b.e) || a.i - b.i)
    .map((x) => x.e);
}

const FOCUSABLE =
  'button:not(:disabled), [role="combobox"]:not(:disabled), input:not(:disabled), a[href], [tabindex="0"]';

/** How long Find a setting waits for a section to render the row (its data may still be loading). */
const LAND_MS = 3000;
/** How long the row keeps its wash after Find a setting lands on it. */
const FOUND_MS = 2000;

let pending: { id: string; until: number } | undefined;
let landing = 0;

/**
 * Moves focus to a setting's control (or the row, when it has none to
 * take it), scrolls it into view and washes the row for a moment. The row
 * may not be on screen yet (the section is changing, or loading its data),
 * so this waits for it, frame by frame, for up to three seconds.
 */
export function focusSetting(id: string): void {
  pending = { id, until: Date.now() + LAND_MS };
  cancelAnimationFrame(landing);
  const step = () => {
    if (!pending) return;
    const row = document.querySelector<HTMLElement>(`[data-setting="${CSS.escape(pending.id)}"]`);
    if (!row) {
      if (Date.now() < pending.until) landing = requestAnimationFrame(step);
      else pending = undefined;
      return;
    }
    pending = undefined;
    let target = row.matches(FOCUSABLE) ? row : row.querySelector<HTMLElement>(FOCUSABLE);
    if (!target) {
      if (!row.hasAttribute("tabindex")) row.tabIndex = -1;
      target = row;
    }
    row.scrollIntoView({ block: "center" });
    target.focus({ preventScroll: true });
    row.setAttribute("data-found", "");
    setTimeout(() => row.removeAttribute("data-found"), FOUND_MS);
  };
  step();
}
