import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { CircleCheck } from "lucide-react";
import { useEffect, type ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { useCdm } from "@/features/cdm";
import { SettingsPanel } from "@/features/settings";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Button } from "@/shared/ui/button";

import { useCompleteSetup } from "./complete";
import { StepHeader } from "./parts";
import { widevineShort } from "./status";
import { widevineView, type SetupStep } from "./steps";

/** Something the setup left undone: what, its state, and the way back to its step. */
function LeftRow({
  title,
  children,
  step,
  action,
}: {
  title: string;
  children: ReactNode;
  step: SetupStep;
  action: string;
}) {
  return (
    <div className="flex items-center justify-between gap-8 py-4">
      <div className="min-w-0">
        <p className="text-ui font-medium text-ink">{title}</p>
        <p className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">{children}</p>
      </div>
      <Button variant="outline" size="sm" asChild>
        <Link to="/welcome" search={{ step }}>
          {action}
        </Link>
      </Button>
    </div>
  );
}

/**
 * The last step: what is still to do (signing in, a Widevine download that
 * hasn't finished), each with the way back to its step, or that everything
 * is ready. Reaching it marks the setup finished; Done opens For you.
 */
export function DoneStep() {
  const { t } = useLingui();
  const complete = useCompleteSetup();
  const state = useAuth().data?.state;
  const caps = useCapabilities();
  const cdm = useCdm((s) => s.status);

  // Saved once: a later run finds the setup already marked.
  useEffect(() => void complete(), [complete]);

  const signedIn = hasAccount(state);
  const view = widevineView(caps, cdm);
  const widevine = widevineShort(view, cdm);
  const pending = view === "waiting" || view === "downloading";
  const failed = view === "failed";
  const left = !signedIn || pending || failed;
  const noPlayback = view === "unavailable" || view === "unsupported";

  return (
    <>
      <StepHeader title={signedIn ? t`You're all set` : t`Setup is done`}>
        <Trans>
          Everything here can be changed in Settings, and Settings → General runs this setup again.
        </Trans>
      </StepHeader>
      {left ? (
        <section aria-labelledby="setup-left" className="flex flex-col gap-2">
          <h2 id="setup-left" className="text-ui font-semibold text-ink">
            <Trans>Still to do</Trans>
          </h2>
          <SettingsPanel>
            {!signedIn && (
              <LeftRow title={t`Apple Music`} step="account" action={t`Sign in`}>
                <Trans>Not signed in. Signing in brings your library and plays whole songs.</Trans>
              </LeftRow>
            )}
            {pending && (
              <LeftRow title={t`Widevine`} step="playback" action={t`Go to Playback`}>
                <Trans>
                  {widevine}. Songs play once it's ready; it carries on in the background.
                </Trans>
              </LeftRow>
            )}
            {failed && (
              <LeftRow title={t`Widevine`} step="playback" action={t`Go to Playback`}>
                <Trans>{widevine}. Sona tries again by itself later.</Trans>
              </LeftRow>
            )}
          </SettingsPanel>
        </section>
      ) : (
        <div className="flex items-start gap-3 rounded-lg bg-surface p-5">
          <CircleCheck
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-positive"
            strokeWidth={1.75}
          />
          <div>
            <p className="text-ui font-medium text-ink">
              {noPlayback ? (
                <Trans>Playback isn't available here yet</Trans>
              ) : (
                <Trans>Ready to play</Trans>
              )}
            </p>
            <p className="mt-0.5 text-sub text-ink-muted">
              <Trans>Your library syncs in the background.</Trans>
            </p>
          </div>
        </div>
      )}
    </>
  );
}
