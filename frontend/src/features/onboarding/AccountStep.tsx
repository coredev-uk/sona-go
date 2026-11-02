import { Trans } from "@lingui/react/macro";
import { CircleCheck } from "lucide-react";
import { useContext } from "react";

import { AccountSection, hasAccount, useAuth } from "@/features/auth";

import { SetupFoot } from "./foot";
import { Well } from "./parts";

/**
 * The Apple Music step: the account dialog's own sign-in (the native form,
 * then the browser and token fallbacks, with its keyring notices; its Sign
 * in sits in the setup's foot as the step's primary action), or,
 * once signed in, a line saying so. Nothing about the account itself is
 * shown here.
 */
export function AccountStep() {
  const state = useAuth().data?.state;
  const foot = useContext(SetupFoot);
  if (!hasAccount(state)) return <AccountSection actions={foot} />;
  return (
    <Well className="flex items-start gap-3">
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
        <p className="text-sub text-ink-muted">
          <Trans>Your library syncs in the background while you carry on.</Trans>
        </p>
      </div>
    </Well>
  );
}
