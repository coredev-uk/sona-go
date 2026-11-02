import { Trans, useLingui } from "@lingui/react/macro";
import { CircleCheck } from "lucide-react";
import { useEffect, type ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { useCdm } from "@/features/cdm";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Button } from "@/shared/ui/button";

import { useCompleteSetup } from "./complete";
import { StepGroup, Well } from "./parts";
import { widevineShort } from "./status";
import { goToStep, widevineView, type SetupStep } from "./steps";

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
    <li className="flex items-center justify-between gap-6 py-3">
      <div className="min-w-0">
        <p className="text-ui font-medium text-ink">{title}</p>
        <p className="text-sub text-pretty text-ink-muted">{children}</p>
      </div>
      <Button variant="outline" size="sm" onClick={() => goToStep(step)}>
        {action}
      </Button>
    </li>
  );
}

/**
 * The last step: what is still to do (signing in, a Widevine download that
 * hasn't finished), each with the way back to its step, or that everything
 * is ready. Reaching it marks the setup finished; Done closes it.
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
  const noPlayback = view === "unavailable" || view === "unsupported";

  if (!signedIn || pending || failed) {
    return (
      <StepGroup id="setup-left" title={t`Still to do`}>
        <ul className="flex flex-col divide-y divide-line">
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
        </ul>
      </StepGroup>
    );
  }
  return (
    <Well className="flex items-start gap-3">
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
        <p className="text-sub text-ink-muted">
          <Trans>Your library syncs in the background.</Trans>
        </p>
      </div>
    </Well>
  );
}
