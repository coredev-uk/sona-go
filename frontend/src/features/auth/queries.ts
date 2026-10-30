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
