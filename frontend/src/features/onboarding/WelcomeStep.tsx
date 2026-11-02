import { Trans, useLingui } from "@lingui/react/macro";
import { CircleCheck } from "lucide-react";
import type { ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { useCdm } from "@/features/cdm";
import {
  LanguageSelect,
  useLanguageOptions,
  useSetLanguage,
  useSettings,
} from "@/features/settings";
import { offersLanguages } from "@/shared/i18n";
import { useCapabilities } from "@/shared/lib/capabilities";

import { StepGroup } from "./parts";
import { widevineShort } from "./status";
import { widevineView } from "./steps";

/** The language, once there is one besides English to choose (as in Settings → General). */
function Language() {
  const { t } = useLingui();
  const current = useSettings().data?.language ?? "";
  const options = useLanguageOptions(current);
  const setLanguage = useSetLanguage();
  if (!options || !offersLanguages(options, current)) return null;
  return (
    <div className="flex items-center justify-between gap-6">
      <div className="min-w-0">
        <label htmlFor="setup-language" className="text-ui font-medium text-ink">
          {t`Language`}
        </label>
        <p id="setup-language-desc" className="text-sub text-ink-muted">
          <Trans>Automatic follows your Apple Music account and your system.</Trans>
        </p>
      </div>
      <LanguageSelect
        id="setup-language"
        value={current}
        options={options}
        onChange={setLanguage}
      />
    </div>
  );
}

/** One thing whole songs need: its name and what it is, then where it stands now. */
function Need({
  name,
  state,
  done,
  children,
}: {
  name: string;
  state: string;
  done: boolean;
  children: ReactNode;
}) {
  return (
    <li className="flex items-start justify-between gap-6 py-2.5">
      <div className="min-w-0">
        <p className="text-ui font-medium text-ink">{name}</p>
        <p className="text-sub text-pretty text-ink-muted">{children}</p>
      </div>
      {state && (
        <p className="flex shrink-0 items-center gap-1.5 pt-px text-sub text-ink">
          {done && (
            <CircleCheck aria-hidden className="size-3.5 text-positive" strokeWidth={1.75} />
          )}
          {state}
        </p>
      )}
    </li>
  );
}

/**
 * The first step: the language (once there is a choice), then the two
 * things whole songs need and where each stands (the download is already
 * running), so nothing about playback is a surprise later.
 */
export function WelcomeStep() {
  const { t } = useLingui();
  const caps = useCapabilities();
  const cdm = useCdm((s) => s.status);
  const state = useAuth().data?.state;
  const view = widevineView(caps, cdm);
  const signedIn = hasAccount(state);
  return (
    <div className="flex flex-col gap-5">
      <Language />
      <StepGroup id="setup-needs" title={t`What whole songs need`}>
        <ul className="flex flex-col divide-y divide-line">
          <Need
            name={t`Apple Music`}
            state={signedIn ? t`Signed in` : t`Next step`}
            done={signedIn}
          >
            <Trans>A subscription, signed in to Sona. Browsing works without one.</Trans>
          </Need>
          {view !== "not-needed" && (
            <Need name={t`Widevine`} state={widevineShort(view, cdm)} done={view === "ready"}>
              <Trans>
                Google's component that plays protected songs. Sona downloads it by itself.
              </Trans>
            </Need>
          )}
        </ul>
      </StepGroup>
    </div>
  );
}
