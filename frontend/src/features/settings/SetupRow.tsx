import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";

import { Button } from "@/shared/ui/button";

import { SettingRow } from "./SettingRow";

/**
 * Opens the first-run setup again (features/onboarding's /welcome): a
 * route link, so settings never imports onboarding.
 */
export function SetupRow() {
  const { t } = useLingui();
  return (
    <SettingRow
      id="setup-again"
      title={t`First-run setup`}
      description={t`Sign in, Widevine and how you listen, one step at a time, as on Sona's first start.`}
    >
      <Button
        id="setup-again"
        aria-describedby="setup-again-desc"
        variant="outline"
        size="sm"
        asChild
      >
        <Link to="/welcome" search={{ step: "welcome" }}>
          <Trans>Set up Sona again</Trans>
        </Link>
      </Button>
    </SettingRow>
  );
}
