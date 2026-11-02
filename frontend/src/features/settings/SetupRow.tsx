import { Trans, useLingui } from "@lingui/react/macro";

import { openSetup } from "@/shared/lib/setup";
import { Button } from "@/shared/ui/button";

import { SettingRow } from "./SettingRow";

/**
 * Opens the first-run setup again over Settings (features/onboarding's
 * layer), through shared/lib/setup, so settings never imports onboarding.
 */
export function SetupRow() {
  const { t } = useLingui();
  return (
    <SettingRow
      id="setup-again"
      title={t`First-run setup`}
      description={t`Sign in, then how Sona plays, connects, looks and starts, one step at a time, as on Sona's first start.`}
    >
      <Button
        id="setup-again"
        aria-describedby="setup-again-desc"
        variant="outline"
        size="sm"
        onClick={openSetup}
      >
        <Trans>Set up Sona again</Trans>
      </Button>
    </SettingRow>
  );
}
