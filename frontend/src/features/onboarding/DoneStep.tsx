import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { useEffect, type ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { useCdm } from "@/features/cdm";
import { usePrivate } from "@/features/private";
import { useSettings } from "@/features/settings";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Button } from "@/shared/ui/button";

import { useCompleteSetup, useLeaveSetup } from "./complete";
import { Panel, StepActions, StepHeader } from "./parts";
import { widevineShort } from "./status";
import { widevineView, type SetupStep } from "./steps";

/** One choice and its value; `action` leads back to the step that sets it. */
function SummaryRow({
  term,
  value,
  action,
}: {
  term: string;
  value: ReactNode;
  action?: { step: SetupStep; text: ReactNode; label: string };
}) {
  return (
    <div className="grid grid-cols-[9rem_minmax(0,1fr)] items-baseline gap-x-6 py-3.5 max-[960px]:grid-cols-1">
      <dt className="text-ui font-medium text-ink">{term}</dt>
      <dd className="flex min-w-0 items-baseline justify-between gap-6 text-ui text-ink-muted">
        <span>{value}</span>
        {action && (
          <Link
            to="/welcome"
            search={{ step: action.step }}
            aria-label={action.label}
            className="shrink-0 rounded-sm text-sub underline underline-offset-2 hover:text-ink"
          >
            {action.text}
          </Link>
        )}
      </dd>
    </div>
  );
}

/**
 * The last step: what the setup left behind, each with the way back to
 * its step. Reaching it marks the setup finished; Done opens For you.
 */
export function DoneStep() {
  const { t } = useLingui();
  const leave = useLeaveSetup();
  const complete = useCompleteSetup();
  const state = useAuth().data?.state;
  const caps = useCapabilities();
  const cdm = useCdm((s) => s.status);
  const priv = usePrivate((s) => s.state);
  const s = useSettings().data;

  // Saved once: a later run finds the setup already marked.
  useEffect(() => void complete(), [complete]);

  const signedIn = hasAccount(state);
  const seconds = s?.crossfade.seconds ?? 0;
  const appleMusic = t`Apple Music`;
  const widevine = t`Widevine`;
  const privateListening = t`Private listening`;
  const soundCheck = t`Sound Check`;
  const crossfade = t`Crossfade`;
  const change = <Trans>Change</Trans>;

  return (
    <>
      <StepHeader title={signedIn ? t`You're all set` : t`Setup is done`}>
        <Trans>
          Everything here can be changed in Settings, and Settings → General runs this setup again.
        </Trans>
      </StepHeader>
      <Panel>
        <dl className="divide-y divide-line">
          <SummaryRow
            term={appleMusic}
            value={signedIn ? t`Signed in` : t`Not signed in`}
            action={
              signedIn
                ? undefined
                : {
                    step: "account",
                    text: <Trans>Sign in</Trans>,
                    label: t`Sign in to Apple Music`,
                  }
            }
          />
          <SummaryRow term={widevine} value={widevineShort(widevineView(caps, cdm), cdm)} />
          <SummaryRow
            term={privateListening}
            value={priv ? (priv.on ? t`On` : t`Off`) : ""}
            action={{ step: "listening", text: change, label: t`Change ${privateListening}` }}
          />
          {caps?.playback.soundCheck && s && (
            <SummaryRow
              term={soundCheck}
              value={s.audio.soundCheck ? t`On` : t`Off`}
              action={{ step: "listening", text: change, label: t`Change ${soundCheck}` }}
            />
          )}
          {caps?.playback.crossfade && s && (
            <SummaryRow
              term={crossfade}
              value={
                s.crossfade.enabled
                  ? t`On, ${plural(seconds, { one: "# second", other: "# seconds" })}`
                  : t`Off`
              }
              action={{ step: "listening", text: change, label: t`Change ${crossfade}` }}
            />
          )}
        </dl>
      </Panel>
      <StepActions>
        <Button size="sm" onClick={() => void leave()}>
          <Trans>Done</Trans>
        </Button>
      </StepActions>
    </>
  );
}
