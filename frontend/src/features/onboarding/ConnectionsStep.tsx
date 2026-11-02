import { Trans, useLingui } from "@lingui/react/macro";

import { ConnectionsSection } from "@/features/settings";

import { StepHeader } from "./parts";

/** Connections, all optional: Settings → Connections' scrobblers and Discord status as they are. */
export function ConnectionsStep() {
  const { t } = useLingui();
  return (
    <>
      <StepHeader title={t`Connections`}>
        <Trans>
          Optional. Keep a record of what you play on Last.fm or ListenBrainz, or show it on
          Discord. Settings → Connections has these too.
        </Trans>
      </StepHeader>
      <ConnectionsSection />
    </>
  );
}
