import { Trans, useLingui } from "@lingui/react/macro";

import {
  CloseActionRow,
  IntegrationsDesktopRows,
  SettingsGroup,
  StartupRows,
  useSettings,
} from "@/features/settings";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";

import { StepHeader, UnavailableRow } from "./parts";

/**
 * Desktop: starting at login, what closing the window does, and track
 * notifications, as Settings → General has them. What this platform can't
 * do yet is named where its row would be, so nothing goes missing quietly.
 */
export function DesktopStep() {
  const { t } = useLingui();
  const settings = useSettings();
  const caps = useCapabilities();
  const notYet = <Trans>Not available on this platform yet.</Trans>;
  let body;
  if (settings.error) {
    body = (
      <ErrorBlock
        title={t`Couldn't read Sona's settings`}
        error={settings.error}
        onRetry={() => void settings.refetch()}
        retrying={settings.isRefetching}
      />
    );
  } else if (!settings.data || !caps) {
    body = <div aria-busy className="h-72 rounded-lg bg-surface" />;
  } else {
    body = (
      <div className="flex flex-col gap-8">
        <SettingsGroup id="setup-startup" title={t`Starting Sona`}>
          {caps.autostart ? (
            <StartupRows />
          ) : (
            <UnavailableRow title={t`Start Sona when you log in`}>{notYet}</UnavailableRow>
          )}
        </SettingsGroup>
        <SettingsGroup id="setup-window" title={t`Window`}>
          {caps.tray ? (
            <CloseActionRow />
          ) : (
            <UnavailableRow title={t`When the window is closed`}>
              <Trans>
                Sona quits. Playing on in the background needs a system tray, which Sona can't use
                on this platform yet.
              </Trans>
            </UnavailableRow>
          )}
        </SettingsGroup>
        <SettingsGroup id="setup-playing" title={t`While music plays`}>
          {!caps.notifications && (
            <UnavailableRow title={t`Track notifications`}>{notYet}</UnavailableRow>
          )}
          <IntegrationsDesktopRows />
        </SettingsGroup>
      </div>
    );
  }
  return (
    <>
      <StepHeader title={t`Desktop`}>
        <Trans>
          How Sona starts, what closing its window does, and what it shows while music plays.
        </Trans>
      </StepHeader>
      {body}
    </>
  );
}
