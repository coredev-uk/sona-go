import { AuthService, type AuthStatus } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

/** Auth status; kept current by auth:changed (this module's event handler). */
export function useAuth() {
  return useQuery({ queryKey: keys.auth, queryFn: () => AuthService.Status() });
}

/** The account states in which the library and personal endpoints are usable. */
export function hasAccount(state: string | undefined): boolean {
  return state === "signed_in" || state === "validating";
}

/**
 * Known to be signed out: not while Go still reads the stored sign-in from
 * the keyring (restoring), and not when Apple only asked to sign in again
 * (needs_reauth: the library is still here).
 */
export function isSignedOut(state: string | undefined): boolean {
  return (
    state !== undefined && state !== "restoring" && state !== "needs_reauth" && !hasAccount(state)
  );
}

/**
 * What the account needs from the user: Apple stopped taking the token, the
 * keyring didn't hand the sign-in over, or Apple refuses Sona's requests.
 * The avatar carries a dot for these; offline and the keyring's own waits
 * are the status banner's.
 */
export function needsAttention(s: AuthStatus | undefined): boolean {
  return (
    s !== undefined &&
    (s.state === "needs_reauth" ||
      s.state === "error" ||
      (s.state === "signed_out" && s.keyring === "unavailable"))
  );
}
