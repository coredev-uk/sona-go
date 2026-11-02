import { t } from "@lingui/core/macro";

import type { EmptyStateAction } from "@/shared/components/EmptyState";
import { openAccountDialog } from "@/shared/lib/account";

/** The sign-in action for empty library states; a function so its label follows the locale. */
export const signInAction = (): EmptyStateAction => ({
  label: t`Sign in`,
  onClick: openAccountDialog,
});
