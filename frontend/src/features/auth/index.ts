import type { AuthStatus } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { CircleUser } from "lucide-react";

import { openAccountDialog } from "@/shared/lib/account";
import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";

import { authInvalidations } from "./invalidations";

export { AccountSection } from "./Account";
export { AccountMenu } from "./AccountMenu";
export { KeyringRetry } from "./KeyringRetry";
export { hasAccount, isSignedOut, useAuth } from "./queries";

/**
 * Sign-in state: the auth query and its Go event, the top bar's account
 * menu and the account dialog it opens, and the palette's way to it.
 */
export const authModule: FeatureModule<never> = {
  id: "auth",
  routes: [],
  shortcuts: [
    {
      id: "auth.account",
      label: () => t`Account settings`,
      group: "general",
      keys: [],
      keywords: () => t`account apple music sign in sign out storefront keyring`,
      icon: CircleUser,
      run: openAccountDialog,
    },
  ],
  eventHandlers: {
    "auth:changed": (data, { qc, invalidate }) => {
      const prev = qc.getQueryData<AuthStatus>(keys.auth);
      qc.setQueryData(keys.auth, data);
      invalidate(authInvalidations(prev, data));
    },
  },
};
