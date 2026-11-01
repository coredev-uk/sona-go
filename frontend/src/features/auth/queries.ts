import { AuthService } from "@bindings/services";
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
