import { Trans, useLingui } from "@lingui/react/macro";
import type { Settings } from "@bindings/config";
import { DesktopService } from "@bindings/services";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";
import { run } from "@/shared/lib/run";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { IntegrationsDesktopRows } from "./IntegrationsRows";
import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";

/**
 * Settings → Desktop: what closing the window does, starting at login,
 * where Sona opens, and the remembered window size.
 */
export function DesktopSection() {
  const { t } = useLingui();
  /** The pages Sona can open on, in sidebar order (Go's config.StartPages). */
  const startPages = [
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
  const settings = useSettings();
  const update = useUpdateSettings();
  const caps = useCapabilities();
  if (settings.error) {
    return (
      <ErrorBlock
        title={t`Couldn't read Sona's settings`}
        error={settings.error}
        onRetry={() => void settings.refetch()}
      />
    );
  }
  const s = settings.data;
  if (!s) return <SettingsPanel className="h-96" />;
  const d = s.desktop;
  const { width, height } = s.window;
  const save = (patch: Partial<Settings["desktop"]>) =>
    update.mutate({ ...s, desktop: { ...d, ...patch } });

  return (
    <SettingsPanel>
      {caps?.tray && (
        <SettingRow
          id="close-action"
          title={t`When the window is closed`}
          description={t`In the background, music keeps playing and Sona stays in the system tray. Open it again from the tray, your media keys' player or the app launcher.`}
        >
          <Select value={d.closeAction} onValueChange={(closeAction) => save({ closeAction })}>
            <SelectTrigger
              id="close-action"
              aria-describedby="close-action-desc"
              className="h-8 w-56"
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
      )}
      {caps?.autostart && (
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
      )}
      {/* Starting without a window needs the tray to open it from. */}
      {caps?.autostart && caps.tray && (
        <SettingRow
          id="start-hidden"
          title={t`Start in the tray`}
          description={t`At login, Sona starts without a window, ready to play from the tray or your media keys.`}
        >
          <Switch
            id="start-hidden"
            aria-describedby="start-hidden-desc"
            checked={d.startHidden}
            disabled={!d.autostart}
            onCheckedChange={(startHidden) => save({ startHidden })}
          />
        </SettingRow>
      )}
      <SettingRow
        id="start-page"
        title={t`Open on`}
        description={t`The page Sona shows when it starts.`}
      >
        <Select
          value={d.startPage}
          disabled={d.resumeLastRoute}
          onValueChange={(startPage) => save({ startPage })}
        >
          <SelectTrigger id="start-page" aria-describedby="start-page-desc" className="h-8 w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {startPages.map((p) => (
              <SelectItem key={p.route} value={p.route}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingRow>
      <SettingRow
        id="resume-route"
        title={t`Resume where you left off`}
        description={t`Opens on the page you were last on instead. Your queue is always kept.`}
      >
        <Switch
          id="resume-route"
          aria-describedby="resume-route-desc"
          checked={d.resumeLastRoute}
          onCheckedChange={(resumeLastRoute) =>
            save({ resumeLastRoute, lastRoute: resumeLastRoute ? d.lastRoute : "" })
          }
        />
      </SettingRow>
      <SettingRow
        id="window-size"
        title={t`Window size`}
        description={
          s.window.maximised ? (
            <Trans>
              Sona opens at the size you left it, maximised:{" "}
              <span className="text-ink num">
                {width} × {height}
              </span>
              .
            </Trans>
          ) : (
            <Trans>
              Sona opens at the size you left it:{" "}
              <span className="text-ink num">
                {width} × {height}
              </span>
              .
            </Trans>
          )
        }
      >
        <Button
          id="window-size"
          aria-describedby="window-size-desc"
          variant="outline"
          size="sm"
          onClick={() => void run(t`Couldn't reset the window`, DesktopService.ResetWindowSize())}
        >
          <Trans>Reset size</Trans>
        </Button>
      </SettingRow>
      <IntegrationsDesktopRows />
    </SettingsPanel>
  );
}
