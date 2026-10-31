import { useLingui } from "@lingui/react/macro";
import type { Settings } from "@bindings/config";

import { usePrivate } from "@/features/private";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Switch } from "@/shared/ui/switch";

import { useIntegrationsStatus } from "./integrations";
import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow } from "./SettingRow";

function useIntegrations() {
  const settings = useSettings();
  const update = useUpdateSettings();
  const status = useIntegrationsStatus();
  const s = settings.data;
  const save = (patch: Partial<Settings["integrations"]>) => {
    if (s) update.mutate({ ...s, integrations: { ...s.integrations, ...patch } });
  };
  return { s: s?.integrations, status: status.data, save };
}

/** Settings → Desktop: track notifications and the suspend inhibitor, where the platform has them. */
export function IntegrationsDesktopRows() {
  const { t } = useLingui();
  const { s, status, save } = useIntegrations();
  const caps = useCapabilities();
  const notificationsDescription = (on: boolean) => {
    return on && status?.notifications === "unavailable"
      ? t`Shows the song, artist and artwork when a new track starts, with a Next button. Not shown while Sona's window is in front. No notification service is running on this desktop.`
      : t`Shows the song, artist and artwork when a new track starts, with a Next button. Not shown while Sona's window is in front.`;
  };

  const inhibitDescription = (on: boolean) => {
    return on && status?.inhibit === "unavailable"
      ? t`Stops the computer from suspending on its own while music plays. The screen can still lock and turn off. Neither logind nor the desktop portal answered, so sleep can't be blocked.`
      : t`Stops the computer from suspending on its own while music plays. The screen can still lock and turn off.`;
  };
  return (
    <>
      {caps?.notifications && (
        <SettingRow
          id="track-notifications"
          title={t`Track notifications`}
          description={notificationsDescription(!!s?.notifications)}
        >
          <Switch
            id="track-notifications"
            aria-describedby="track-notifications-desc"
            checked={s?.notifications ?? false}
            disabled={!s}
            onCheckedChange={(notifications) => save({ notifications })}
          />
        </SettingRow>
      )}
      {caps?.sleepInhibit && (
        <SettingRow
          id="inhibit-sleep"
          title={t`Stay awake while playing`}
          description={inhibitDescription(!!s?.inhibitSleep)}
        >
          <Switch
            id="inhibit-sleep"
            aria-describedby="inhibit-sleep-desc"
            checked={s?.inhibitSleep ?? false}
            disabled={!s}
            onCheckedChange={(inhibitSleep) => save({ inhibitSleep })}
          />
        </SettingRow>
      )}
    </>
  );
}

/** Settings → Connections: Discord presence (opt-in). */
export function IntegrationsConnectionsRows() {
  const { s, status, save } = useIntegrations();
  const { t } = useLingui();
  const privateOn = usePrivate((p) => p.state?.on ?? false);
  const configured = status?.discord.configured ?? false;
  const discordDescription = (on: boolean) => {
    if (status && !status.discord.configured) {
      return t`Not available in this build: it was made without a Discord application ID.`;
    }
    if (!on || !status) {
      return t`Shows the song you're playing, with its artwork and time, as your Discord activity. Cleared when you pause.`;
    }
    if (privateOn) {
      return t`Shows the song you're playing, with its artwork and time, as your Discord activity. Nothing is shown while private listening is on.`;
    }
    return status.discord.connected
      ? t`Shows the song you're playing, with its artwork and time, as your Discord activity. Cleared when you pause. Connected to Discord.`
      : t`Shows the song you're playing, with its artwork and time, as your Discord activity. Cleared when you pause. Discord isn't reachable yet; Sona keeps trying while music plays.`;
  };
  return (
    <SettingRow
      id="discord-presence"
      title={t`Discord status`}
      description={discordDescription(!!s?.discordPresence)}
    >
      <Switch
        id="discord-presence"
        aria-describedby="discord-presence-desc"
        checked={configured && (s?.discordPresence ?? false)}
        disabled={!s || !configured}
        onCheckedChange={(discordPresence) => save({ discordPresence })}
      />
    </SettingRow>
  );
}
