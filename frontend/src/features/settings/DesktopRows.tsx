import type { Settings } from "@bindings/config";
import { Trans, useLingui } from "@lingui/react/macro";

import { useCapabilities } from "@/shared/lib/capabilities";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { useSettings, useUpdateSettings } from "./queries";
import { SELECT_WIDTH, SettingRow } from "./SettingRow";

function useDesktop() {
  const settings = useSettings();
  const update = useUpdateSettings();
  const s = settings.data;
  const save = (patch: Partial<Settings["desktop"]>) => {
    if (s) update.mutate({ ...s, desktop: { ...s.desktop, ...patch } });
  };
  return { d: s?.desktop, save };
}

/**
 * Start at login, and start in the tray once that is on (Settings →
 * General, the first-run setup), where the platform can do them.
 */
export function StartupRows() {
  const { t } = useLingui();
  const caps = useCapabilities();
  const { d, save } = useDesktop();
  if (!d || !caps?.autostart) return null;
  return (
    <>
      <SettingRow
        id="autostart"
        title={t`Start Sona when you log in`}
        description={t`Adds Sona to your desktop's startup applications.`}
      >
        <Switch
          id="autostart"
          aria-describedby="autostart-desc"
          checked={d.autostart}
          onCheckedChange={(autostart) => save({ autostart })}
        />
      </SettingRow>
      {/* Starting without a window needs the tray to open it from, and only applies at login. */}
      {caps.tray && d.autostart && (
        <SettingRow
          id="start-hidden"
          title={t`Start in the tray`}
          description={t`At login, Sona starts without a window, ready to play from the tray or your media keys.`}
        >
          <Switch
            id="start-hidden"
            aria-describedby="start-hidden-desc"
            checked={d.startHidden}
            onCheckedChange={(startHidden) => save({ startHidden })}
          />
        </SettingRow>
      )}
    </>
  );
}

/** What closing the window does (Settings → General, the first-run setup); needs the tray. */
export function CloseActionRow() {
  const { t } = useLingui();
  const tray = useCapabilities()?.tray ?? false;
  const { d, save } = useDesktop();
  if (!d || !tray) return null;
  return (
    <SettingRow
      id="close-action"
      title={t`When the window is closed`}
      description={t`In the background, music keeps playing and Sona stays in the system tray. Open it again from the tray, your media keys' player or the app launcher.`}
    >
      <Select value={d.closeAction} onValueChange={(closeAction) => save({ closeAction })}>
        <SelectTrigger
          id="close-action"
          aria-describedby="close-action-desc"
          className={SELECT_WIDTH}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="background">
            <Trans>Keep playing in the background</Trans>
          </SelectItem>
          <SelectItem value="quit">
            <Trans>Quit Sona</Trans>
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingRow>
  );
}
