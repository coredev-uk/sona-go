import { i18n } from "@lingui/core";
import { I18nProvider, useLingui } from "@lingui/react";
import { Fragment, type ReactNode, useSyncExternalStore } from "react";

import { getRevision, subscribeRevision } from "./revision";

/**
 * Provides the active locale and remounts its subtree when the locale (or
 * the locales it formats with, or its catalogue) changes, so strings
 * computed outside hooks (registries, memoised components) are recomputed
 * in the new language.
 */
export function I18nRoot({ children }: { children: ReactNode }) {
  return (
    <I18nProvider i18n={i18n}>
      <LocaleKeyed>{children}</LocaleKeyed>
    </I18nProvider>
  );
}

function LocaleKeyed({ children }: { children: ReactNode }) {
  const { i18n: active } = useLingui();
  const revision = useSyncExternalStore(subscribeRevision, getRevision);
  return (
    <Fragment key={`${active.locale}|${String(active.locales)}|${revision}`}>{children}</Fragment>
  );
}
