import { useLingui } from "@lingui/react/macro";
import { DesktopService } from "@bindings/services";
import { SquareTerminal } from "lucide-react";

import { useCapabilities } from "@/shared/lib/capabilities";
import { run } from "@/shared/lib/run";
import { Button } from "@/shared/ui/button";
import { Switch } from "@/shared/ui/switch";

import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsGroup } from "./SettingRow";

/**
 * WebKit's web inspector for Sona's own page: its console, network and
 * storage, for debugging and for writing plugins (Settings → Diagnostics).
 * The setting applies from the next start (the window is created with the
 * inspector on or off); `Capabilities.developerTools` says it is on now.
 */
export function DeveloperTools() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const active = useCapabilities()?.developerTools ?? false;
  const s = settings.data;
  if (!s) return null;
  return (
    <SettingsGroup id="developer-tools" title={t`Developer tools`}>
      <SettingRow
        id="developerTools"
        title={t`Web inspector`}
        description={t`Adds Inspect Element to the right-click menu and the button below: the console, network and storage of Sona's page, for debugging and writing plugins. Applies from Sona's next start (SONA_DEVTOOLS=1 turns it on for one run). Only paste into the console what you understand: it runs with Sona's access.`}
      >
        <Switch
          id="developerTools"
          aria-describedby="developerTools-desc"
          checked={s.diagnostics.developerTools}
          onCheckedChange={(on) =>
            update.mutate({ ...s, diagnostics: { ...s.diagnostics, developerTools: on } })
          }
        />
      </SettingRow>
      <SettingRow
        id="openDeveloperTools"
        title={t`Open the web inspector`}
        description={
          active
            ? t`Shows the inspector docked in Sona's window.`
            : t`Turn on Web inspector and restart Sona first.`
        }
      >
        <Button
          id="openDeveloperTools"
          aria-describedby="openDeveloperTools-desc"
          variant="outline"
          size="sm"
          disabled={!active}
          onClick={() =>
            void run(t`Couldn't open the web inspector`, DesktopService.OpenDeveloperTools())
          }
        >
          <SquareTerminal strokeWidth={1.75} />
          {t`Open`}
        </Button>
      </SettingRow>
    </SettingsGroup>
  );
}
