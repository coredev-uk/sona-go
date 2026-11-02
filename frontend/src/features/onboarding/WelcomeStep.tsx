import { Trans, useLingui } from "@lingui/react/macro";

import { useCdm } from "@/features/cdm";
import { LanguagePanel, SettingsPanel } from "@/features/settings";
import { SonaMark } from "@/shared/brand/SonaMark";
import { useCapabilities } from "@/shared/lib/capabilities";

import { StepHeader } from "./parts";
import { widevineShort } from "./status";
import { widevineView } from "./steps";

/**
 * The first step: the language (once there is a choice, as in Settings →
 * General), what Sona is, and the two things whole songs need, the
 * download's state included (it is already running), so nothing about
 * playback is a surprise later.
 */
export function WelcomeStep() {
  const { t } = useLingui();
  const caps = useCapabilities();
  const cdm = useCdm((s) => s.status);
  const view = widevineView(caps, cdm);
  const widevine = view === "not-needed" ? "" : widevineShort(view, cdm);
  return (
    <>
      <SonaMark className="mb-6 size-12" />
      <StepHeader large title={t`Welcome to Sona`}>
        <Trans>
          Sona plays Apple Music on this computer. The next few steps sign you in and set how Sona
          plays, looks and starts. Each can be skipped, and everything can be changed later in
          Settings.
        </Trans>
      </StepHeader>
      <LanguagePanel className="mb-6" />
      <section aria-labelledby="setup-needs" className="flex flex-col gap-2">
        <h2 id="setup-needs" className="text-ui font-semibold text-ink">
          <Trans>What whole songs need</Trans>
        </h2>
        <SettingsPanel>
          <dl className="divide-y divide-line">
            <div className="grid grid-cols-[8rem_minmax(0,1fr)] gap-6 py-4">
              <dt className="text-ui font-medium text-ink">{t`Apple Music`}</dt>
              <dd className="text-sub text-ink-muted">
                <Trans>
                  A subscription, signed in on the next step. You can browse Apple Music without
                  one.
                </Trans>
              </dd>
            </div>
            {view !== "not-needed" && (
              <div className="grid grid-cols-[8rem_minmax(0,1fr)] gap-6 py-4">
                <dt className="text-ui font-medium text-ink">{t`Widevine`}</dt>
                <dd className="text-sub text-ink-muted">
                  <Trans>
                    Google's component that plays Apple Music's protected songs. Sona downloads it
                    by itself.
                  </Trans>
                  {widevine && <span className="mt-1 block text-ink">{widevine}</span>}
                </dd>
              </div>
            )}
          </dl>
        </SettingsPanel>
      </section>
    </>
  );
}
