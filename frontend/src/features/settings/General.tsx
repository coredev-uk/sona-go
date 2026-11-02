import { Trans, useLingui } from "@lingui/react/macro";
import type { Settings } from "@bindings/config";
import { DesktopService } from "@bindings/services";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";
import { run } from "@/shared/lib/run";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { CloseActionRow, StartupRows } from "./DesktopRows";
import { IntegrationsDesktopRows } from "./IntegrationsRows";
import { LanguagePanel } from "./LanguageRow";
import { useSettings, useUpdateSettings } from "./queries";
import {
  SELECT_WIDTH,
  SettingRow,
  SettingsGroup,
  SettingsLoading,
  SettingsSections,
} from "./SettingRow";
import { SetupRow } from "./SetupRow";
import { startPages } from "./summaries";
import { UpdatesGroup } from "./Updates";

/** The "Open on" value standing for the page Sona was last on (Radix Select needs a non-empty value). */
const LAST_PAGE = "last-page";

/**
 * Title bar (Windows and Linux; macOS always keeps its own buttons in
 * Sona's top bar): who draws it. The hint says what Automatic picks here.
 */
function TitleBarRow({ value, onChange }: { value: string; onChange: (style: string) => void }) {
  const { t } = useLingui();
  const controls = useCapabilities()?.windowControls;
  if (!controls) return null;
  const desktop = controls.desktop;
  let hint: string;
  if (controls.automatic === "sona") {
    hint = desktop
      ? t`Automatic uses Sona's title bar on ${desktop}, with the window buttons where your desktop puts them.`
      : t`Automatic uses Sona's title bar, with the window buttons where your system puts them.`;
  } else {
    hint = desktop
      ? t`Automatic leaves the title bar to ${desktop}.`
      : t`Automatic leaves the title bar to your desktop, which Sona doesn't recognise.`;
  }
  // The window keeps the title bar it was created with until Sona restarts.
  const pending = value !== controls.style;
  return (
    <SettingRow
      id="title-bar"
      title={t`Title bar`}
      description={
        pending ? (
          <>
            {hint} <span className="text-ink">{t`Applies the next time Sona starts.`}</span>
          </>
        ) : (
          hint
        )
      }
    >
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id="title-bar" aria-describedby="title-bar-desc" className={SELECT_WIDTH}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="auto">
            <Trans>Automatic</Trans>
          </SelectItem>
          <SelectItem value="sona">
            <Trans>Sona's, with window buttons</Trans>
          </SelectItem>
          <SelectItem value="sona-no-buttons">
            <Trans>Sona's, without window buttons</Trans>
          </SelectItem>
          <SelectItem value="desktop">
            <Trans>The desktop's</Trans>
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingRow>
  );
}

/**
 * Settings → General: the language (once there is a choice), starting
 * Sona, its window, what it does while music plays, and updates. Rows the
 * platform can't back keep their place and say so (UnavailableRow).
 */
export function GeneralSection() {
  const { t } = useLingui();
  const pages = startPages();
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
  if (!s) return <SettingsLoading rows={6} />;
  const d = s.desktop;
  const { width, height } = s.window;
  const save = (patch: Partial<Settings["desktop"]>) =>
    update.mutate({ ...s, desktop: { ...d, ...patch } });

  return (
    <SettingsSections>
      <LanguagePanel />
      <SettingsGroup id="general-startup" title={t`Starting Sona`}>
        <StartupRows />
        <SettingRow
          id="start-page"
          title={t`Open on`}
          description={t`The page Sona shows when it starts. Your queue is always kept.`}
        >
          <Select
            value={d.resumeLastRoute ? LAST_PAGE : d.startPage}
            onValueChange={(v) =>
              v === LAST_PAGE
                ? save({ resumeLastRoute: true })
                : save({ startPage: v, resumeLastRoute: false, lastRoute: "" })
            }
          >
            <SelectTrigger
              id="start-page"
              aria-describedby="start-page-desc"
              className={SELECT_WIDTH}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={LAST_PAGE}>
                <Trans>The page you were last on</Trans>
              </SelectItem>
              {pages.map((p) => (
                <SelectItem key={p.route} value={p.route}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
        <SetupRow />
      </SettingsGroup>
      <SettingsGroup id="general-window" title={t`Window`}>
        <CloseActionRow />
        {caps && caps.windowControls.native === "frame" && (
          <TitleBarRow
            value={s.titleBar.style}
            onChange={(style) => update.mutate({ ...s, titleBar: { style } })}
          />
        )}
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
      </SettingsGroup>
      {caps && (
        <SettingsGroup id="general-playing" title={t`While music plays`}>
          <IntegrationsDesktopRows />
        </SettingsGroup>
      )}
      <UpdatesGroup />
    </SettingsSections>
  );
}
