import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";

import { hasAccount, useAuth } from "@/features/auth";
import { useCdm } from "@/features/cdm";
import { usePrivate } from "@/features/private";
import { useScrobbleStatus, useSettings } from "@/features/settings";
import { useCapabilities } from "@/shared/lib/capabilities";

import { widevineItem } from "./status";
import { setupSteps, widevineView, type SetupStep } from "./steps";

/** Brand names, never translated. */
const LASTFM = "Last.fm";
const LISTENBRAINZ = "ListenBrainz";
const DISCORD = "Discord";

/**
 * Each step's line under its name, set like the catalogue line (middle
 * dots, the meta size): where that step stands now, read live, so the
 * list doubles as the summary of the setup. Steps with nothing to say have
 * no line.
 */
function useStepLines(): Partial<Record<SetupStep, string>> {
  const { t } = useLingui();
  const state = useAuth().data?.state;
  const caps = useCapabilities();
  const cdm = useCdm((s) => s.status);
  const priv = usePrivate((s) => s.state);
  const s = useSettings().data;
  const scrobble = useScrobbleStatus().data;

  const lines: Partial<Record<SetupStep, string>> = {};
  if (state) {
    lines.account =
      state === "validating" ? t`Checking…` : hasAccount(state) ? t`Signed in` : t`Not signed in`;
  }
  if (caps && s) {
    const playback = caps.playback.available
      ? [
          widevineItem(widevineView(caps, cdm), cdm),
          s.audioQuality === 64 ? t`Data saver` : t`High quality`,
          priv?.on ? t`Private` : "",
        ]
      : [t`No playback here`];
    lines.playback = playback.filter(Boolean).join(" · ");

    const connected = [
      scrobble?.lastfm.connected ? LASTFM : "",
      scrobble?.listenbrainz.connected ? LISTENBRAINZ : "",
      caps.discordPresence && s.integrations.discordPresence ? DISCORD : "",
    ].filter(Boolean);
    lines.connections = connected.length > 0 ? connected.join(" · ") : t`Optional`;

    const themes: Record<string, string> = { light: t`Light theme`, dark: t`Dark theme` };
    lines.appearance = [
      themes[s.theme] ?? t`System theme`,
      s.reduceEffects ? t`Reduced effects` : "",
    ]
      .filter(Boolean)
      .join(" · ");

    if (caps.autostart && s.desktop.autostart) lines.desktop = t`Starts at login`;
  }
  return lines;
}

/**
 * The setup's steps down the window's leading side: every step is a link
 * (back, or ahead: each step can be skipped), the current one marked by
 * the router (aria-current="page": each step has its own URL), each with
 * its live line.
 */
export function StepRail() {
  const { t } = useLingui();
  const lines = useStepLines();
  const names: Record<SetupStep, string> = {
    welcome: t`Welcome`,
    account: t`Apple Music`,
    playback: t`Playback`,
    connections: t`Connections`,
    appearance: t`Appearance`,
    desktop: t`Desktop`,
    done: t`Done`,
  };
  return (
    <>
      <h2 id="setup-steps" className="mb-1.5 px-2.5 text-meta text-ink-faint">
        <Trans>Set up Sona</Trans>
      </h2>
      <ol aria-labelledby="setup-steps" className="flex flex-col gap-0.5">
        {setupSteps.map((step) => (
          <li key={step}>
            <Link
              to="/welcome"
              search={{ step }}
              className="group flex min-h-8 flex-col justify-center rounded-md px-2.5 py-1.5 text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink aria-[current=page]:bg-active aria-[current=page]:text-ink"
            >
              <span className="text-ui group-aria-[current=page]:font-medium">{names[step]}</span>
              {lines[step] && (
                <>
                  {" "}
                  <span className="text-meta text-ink-faint num">{lines[step]}</span>
                </>
              )}
            </Link>
          </li>
        ))}
      </ol>
    </>
  );
}
