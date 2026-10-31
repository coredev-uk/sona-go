import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { SystemService } from "@bindings/services";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";

import { useCapabilities } from "@/shared/lib/capabilities";
import { callFailure } from "@/shared/lib/errors";
import { Button } from "@/shared/ui/button";

import { SettingRow } from "./SettingRow";
import { changedRows } from "./settings-file";

/**
 * Settings → Storage: save the settings to a file and load one, through
 * the desktop's file chooser (Go, xdg-desktop-portal). Go leaves out what
 * belongs to this computer and applies an import like any other change,
 * so settings:changed refreshes every row. Left out without a file chooser.
 */
export function SettingsFileRows() {
  const { t } = useLingui();
  const fileChooser = useCapabilities()?.fileChooser ?? false;
  const exporting = useMutation({
    mutationFn: () => SystemService.ExportSettings(),
    onSuccess: (path) => {
      // "" when the chooser was cancelled.
      if (!path) return;
      const file = path.split("/").pop();
      toast(t`Settings exported`, { description: t`Saved as ${file}.` });
    },
    onError: (err) =>
      toast.error(t`Couldn't export your settings`, { description: callFailure(err).message }),
  });
  const importing = useMutation({
    mutationFn: () => SystemService.ImportSettings(),
    onSuccess: (r) => {
      if (!r.path) return;
      const rows = changedRows(r.changed);
      if (rows.length === 0) {
        const file = r.path.split("/").pop();
        toast(t`Nothing to change`, {
          description: t`The settings in ${file} already match yours.`,
        });
        return;
      }
      const count = rows.length;
      toast(t`${plural(count, { one: "Imported # setting", other: "Imported # settings" })}`, {
        description: rows.join(", "),
      });
    },
    onError: (err) =>
      toast.error(t`Couldn't import the settings`, { description: callFailure(err).message }),
  });
  const busy = exporting.isPending || importing.isPending;
  if (!fileChooser) return null;

  return (
    <SettingRow
      id="settings-file"
      title={t`Settings file`}
      description={t`Save your settings to a file, or load one exported from Sona on another computer. The window size, CDM location, audio output, rendering fixes and connected accounts stay with this computer.`}
    >
      <div
        id="settings-file"
        role="group"
        aria-label={t`Settings file`}
        aria-describedby="settings-file-desc"
        className="flex gap-2"
      >
        <Button variant="outline" size="sm" disabled={busy} onClick={() => exporting.mutate()}>
          {exporting.isPending ? <Trans>Exporting…</Trans> : <Trans>Export…</Trans>}
        </Button>
        <Button variant="outline" size="sm" disabled={busy} onClick={() => importing.mutate()}>
          {importing.isPending ? <Trans>Importing…</Trans> : <Trans>Import…</Trans>}
        </Button>
      </div>
    </SettingRow>
  );
}
