import { Trans, useLingui } from "@lingui/react/macro";
import { CircleCheck } from "lucide-react";

import { AccountSection, hasAccount, useAuth } from "@/features/auth";

import { StepHeader } from "./parts";

/**
 * The Apple Music step: the app's own sign-in (the native form, then the
 * browser and token fallbacks, with its keyring notices), or, once signed
 * in, a line saying so. Nothing about the account itself is shown here.
 */
export function AccountStep() {
  const { t } = useLingui();
  const state = useAuth().data?.state;
  return (
    <>
      <StepHeader title={t`Apple Music`}>
        <Trans>
          Signing in brings your Apple Music library, playlists and recommendations to Sona, and
          lets it play whole songs. Sona keeps the sign-in in your keyring; it never saves your
          password.
        </Trans>
      </StepHeader>
      {hasAccount(state) ? (
        <div className="flex items-start gap-3 rounded-lg bg-surface p-5">
          <CircleCheck
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-positive"
            strokeWidth={1.75}
          />
          <div>
            <p className="text-ui font-medium text-ink">
              {state === "validating" ? (
                <Trans>Checking your sign-in…</Trans>
              ) : (
                <Trans>Signed in to Apple Music</Trans>
              )}
            </p>
            <p className="mt-0.5 text-sub text-ink-muted">
              <Trans>Your library syncs in the background while you carry on.</Trans>
            </p>
          </div>
        </div>
      ) : (
        <AccountSection headingLevel={2} />
      )}
    </>
  );
}
