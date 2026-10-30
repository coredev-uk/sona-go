import { t } from "@lingui/core/macro";

/** The sign-in action for empty library states; a function so its label follows the locale. */
export const signInAction = () =>
  ({
    label: t`Sign in`,
    to: "/settings",
    search: { section: "account" },
  }) as const;
