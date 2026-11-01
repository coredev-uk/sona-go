import type { AuthStatus } from "@bindings/services";

import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";

import { authInvalidations } from "./invalidations";

export { AccountSection } from "./Account";
export { KeyringRetry } from "./KeyringRetry";
export { hasAccount, isSignedOut, useAuth } from "./queries";

/** Sign-in state: the auth query, its Go event, and the Account settings section. */
export const authModule: FeatureModule<never> = {
  id: "auth",
  routes: [],
  eventHandlers: {
    "auth:changed": (data, { qc, invalidate }) => {
      const prev = qc.getQueryData<AuthStatus>(keys.auth);
      qc.setQueryData(keys.auth, data);
      invalidate(authInvalidations(prev, data));
    },
  },
};
