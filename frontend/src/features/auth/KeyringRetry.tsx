import { AuthService, type AuthStatus } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { useState } from "react";

/**
 * Reads the stored sign-in from the keyring again (AuthService.RetryKeyring)
 * after the keyring didn't answer, refused, or wasn't there. While that read
 * waits it stays in place as "Trying…" (aria-disabled, so focus stays on it);
 * during the start-up read nobody asked for, it shows nothing.
 */
export function KeyringRetry({ status, className }: { status: AuthStatus; className: string }) {
  const { t } = useLingui();
  const [asked, setAsked] = useState(false);
  const trying = status.state === "restoring";
  if (trying && !asked) return null;
  return (
    <button
      type="button"
      className={className}
      aria-disabled={trying}
      onClick={() => {
        if (trying) return;
        setAsked(true);
        void AuthService.RetryKeyring();
      }}
    >
      {trying
        ? t`Trying…`
        : t({
            message: "Try again",
            comment:
              "Reads the sign-in from the system keyring again. Apple's wording: ASE.Web.Music.Error.TryAgain",
          })}
    </button>
  );
}
