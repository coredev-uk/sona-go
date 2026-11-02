import type { Settings } from "@bindings/config";
import { Trans, useLingui } from "@lingui/react/macro";

import { useSettings, useUpdateSettings } from "@/features/settings";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";

import { ChoiceCards, Loading, StepGroup, SwitchList, SwitchRow } from "./parts";

/**
 * Desktop: what closing the window does, starting at login, and what Sona
 * does while music plays, each where the platform can. What it can't do
 * yet is named in one line at the foot, so nothing goes missing quietly.
 */
export function DesktopStep() {
  const { t, i18n } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const caps = useCapabilities();
  const s = settings.data;

  if (settings.error) {
    return (
      <ErrorBlock
        title={t`Couldn't read Sona's settings`}
        error={settings.error}
        onRetry={() => void settings.refetch()}
        retrying={settings.isRefetching}
      />
    );
  }
  if (!s || !caps) return <Loading className="h-72" />;

  const d = s.desktop;
  const desktop = (patch: Partial<Settings["desktop"]>) =>
    update.mutate({ ...s, desktop: { ...d, ...patch } });
  const integrations = (patch: Partial<Settings["integrations"]>) =>
    update.mutate({ ...s, integrations: { ...s.integrations, ...patch } });
  const missing = [
    caps.autostart ? "" : t`starting at login`,
    caps.notifications ? "" : t`track notifications`,
  ].filter(Boolean);
  const missingList = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(missing);

  return (
    <div className="flex flex-col gap-5">
      {caps.tray && (
        <StepGroup id="setup-close" title={t`When you close the window`}>
          <ChoiceCards
            name="setup-close"
            labelledBy="setup-close"
            value={d.closeAction}
            choices={[
              {
                value: "background",
                label: t`Keep playing`,
                hint: t`Sona stays in the system tray.`,
              },
              { value: "quit", label: t`Quit Sona`, hint: t`The music stops with it.` },
            ]}
            onChange={(closeAction) => desktop({ closeAction })}
          />
        </StepGroup>
      )}
      <SwitchList>
        {caps.autostart && (
          <SwitchRow
            id="setup-autostart"
            label={t`Start Sona when you log in`}
            hint={t`Adds Sona to your desktop's startup applications.`}
            checked={d.autostart}
            onChange={(autostart) => desktop({ autostart })}
          />
        )}
        {/* Starting without a window needs the tray to open it from, and only applies at login. */}
        {caps.autostart && caps.tray && d.autostart && (
          <SwitchRow
            id="setup-start-hidden"
            label={t`Start in the tray`}
            hint={t`At login, Sona starts without a window, ready for your media keys.`}
            checked={d.startHidden}
            onChange={(startHidden) => desktop({ startHidden })}
          />
        )}
        {caps.notifications && (
          <SwitchRow
            id="setup-notifications"
            label={t`Track notifications`}
            hint={t`The song and artist when a new track starts, while Sona is in the background.`}
            checked={s.integrations.notifications}
            onChange={(notifications) => integrations({ notifications })}
          />
        )}
        {caps.sleepInhibit && (
          <SwitchRow
            id="setup-inhibit-sleep"
            label={t`Stay awake while playing`}
            hint={t`The computer doesn't suspend on its own while music plays.`}
            checked={s.integrations.inhibitSleep}
            onChange={(inhibitSleep) => integrations({ inhibitSleep })}
          />
        )}
      </SwitchList>
      {(missing.length > 0 || !caps.tray) && (
        <p className="text-sub text-pretty text-ink-muted">
          {missing.length > 0 && <Trans>Not on this platform yet: {missingList}.</Trans>}{" "}
          {!caps.tray && (
            <Trans>
              Closing the window quits Sona: playing on in the background needs a system tray, which
              Sona can't use here yet.
            </Trans>
          )}
        </p>
      )}
    </div>
  );
}
