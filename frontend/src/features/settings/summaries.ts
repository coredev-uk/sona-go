import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";

import { useCapabilities } from "@/shared/lib/capabilities";
import { conflictingIds, registeredShortcuts, resolveBindings } from "@/shared/shortcuts/registry";

import { useSettings } from "./queries";
import { PROVIDER_NAMES, useScrobbleStatus } from "./scrobble";
import type { SettingsSection } from "./search-params";
import { usePlatformName } from "./platform";

/** The pages Sona can open on, in sidebar order (Go's config.StartPages). */
export function startPages(): { route: string; label: string }[] {
  return [
    { route: "/", label: t`For you` },
    { route: "/new", label: t`Discover` },
    { route: "/charts", label: t`Charts` },
    { route: "/radio", label: t`Radio` },
    { route: "/library/recently-added", label: t`Recently added` },
    { route: "/library/artists", label: t`Artists` },
    { route: "/library/albums", label: t`Albums` },
    { route: "/library/songs", label: t`Songs` },
    { route: "/library/playlists", label: t`Playlists` },
  ];
}

/**
 * Each section's state in one catalogue line ("High quality · Crossfade
 * 6 s"), for the section list: what a section is set to reads without
 * opening it. Only cheap data: the settings, the capabilities and the
 * scrobblers' status; Diagnostics, whose checks cost a run, says what it
 * holds instead. Empty while the settings load.
 */
export function useSectionSummaries(): Partial<Record<SettingsSection, string>> {
  const { i18n } = useLingui();
  const s = useSettings().data;
  const caps = useCapabilities();
  const scrobble = useScrobbleStatus().data;
  const platform = usePlatformName();
  if (!s || !caps) return {};
  const line = (parts: (string | false | undefined)[]) => parts.filter(Boolean).join(" · ");
  const d = s.desktop;

  const page = startPages().find((p) => p.route === d.startPage)?.label ?? d.startPage;
  const general = line([
    d.resumeLastRoute ? t`Opens where you left off` : t`Opens on ${page}`,
    caps.tray && (d.closeAction === "quit" ? t`Quits when closed` : t`Keeps playing when closed`),
  ]);

  const seconds = s.crossfade.seconds;
  const playback = caps.playback.available
    ? line([
        s.audioQuality === 64 ? t`Data saver` : t`High quality`,
        caps.playback.soundCheck && s.audio.soundCheck && t`Sound Check`,
        caps.playback.crossfade && s.crossfade.enabled && t`Crossfade ${seconds} s`,
      ])
    : t`Not available on ${platform} yet`;

  const appearance = line([
    s.theme === "light" ? t`Light theme` : s.theme === "dark" ? t`Dark theme` : t`System theme`,
    s.reduceEffects ? t`Reduced effects` : !s.motionArtwork.enabled && t`Motion artwork off`,
  ]);

  const connected = [
    scrobble?.lastfm.connected && PROVIDER_NAMES.lastfm,
    scrobble?.listenbrainz.connected && PROVIDER_NAMES.listenbrainz,
    // eslint-disable-next-line lingui/no-unlocalized-strings -- brand name
    caps.discordPresence && s.integrations.discordPresence && "Discord",
  ].filter((x): x is string => !!x);
  const connections = connected.length
    ? new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(connected)
    : t`Nothing connected`;

  const overrides = s.shortcuts ?? {};
  const all = registeredShortcuts();
  const conflicts = conflictingIds(all, resolveBindings(all, overrides)).size;
  const changed = Object.keys(overrides).length;
  const shortcuts = line([
    conflicts > 0 && plural(conflicts, { one: "# conflict", other: "# conflicts" }),
    changed > 0 ? plural(changed, { one: "# changed", other: "# changed" }) : t`Default keys`,
  ]);

  const mib = s.storage.artCacheMiB;
  const gb = mib / 1024;
  const limit = mib >= 1024 ? t`${gb} GB` : t`${mib} MB`;

  return {
    general,
    playback,
    appearance,
    connections,
    shortcuts,
    storage: t`Artwork up to ${limit}`,
    diagnostics: t`Checks, logs and bug reports`,
  };
}
