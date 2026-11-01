import { Trans, useLingui } from "@lingui/react/macro";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { Check, CircleAlert } from "lucide-react";
import { useEffect, type ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { cdmPercent, useCdm } from "@/features/cdm";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Button } from "@/shared/ui/button";

import { AccountStep } from "./AccountStep";
import { useLeaveSetup } from "./complete";
import { DoneStep } from "./DoneStep";
import { ListeningStep } from "./ListeningStep";
import { STEP_TITLE_ID } from "./parts";
import { widevineShort } from "./status";
import { setupSteps, stepAfter, widevineView, type SetupStep } from "./steps";
import { WelcomeStep } from "./WelcomeStep";
import { WidevineStep } from "./WidevineStep";

const route = getRouteApi("/welcome");

/**
 * The step list: every step is a link (go back, or skip ahead), the
 * router marks the current one (aria-current="page": each step has its own URL). Apple Music and Widevine carry their
 * live state, so the download's progress shows from every step.
 */
function StepList() {
  const { t } = useLingui();
  const signedIn = hasAccount(useAuth().data?.state);
  const caps = useCapabilities();
  const cdm = useCdm((s) => s.status);
  const view = widevineView(caps, cdm);
  const labels: Record<SetupStep, string> = {
    welcome: t`Welcome`,
    account: t`Apple Music`,
    widevine: t`Widevine`,
    listening: t`Listening`,
    done: t`Done`,
  };

  const mark = (step: SetupStep): ReactNode => {
    if (step === "account" && signedIn) {
      return (
        <>
          <Check aria-hidden className="size-4 text-ink-muted" strokeWidth={1.75} />
          <span className="sr-only">{t`Signed in`}</span>
        </>
      );
    }
    if (step !== "widevine") return null;
    if (view === "ready") {
      return (
        <>
          <Check aria-hidden className="size-4 text-ink-muted" strokeWidth={1.75} />
          <span className="sr-only">{widevineShort(view, cdm)}</span>
        </>
      );
    }
    if (view === "failed") {
      return (
        <>
          <CircleAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />
          <span className="sr-only">{widevineShort(view, cdm)}</span>
        </>
      );
    }
    if (view === "downloading" && cdm && cdm.total > 0) {
      return (
        <>
          <span aria-hidden className="text-meta text-ink-faint num">
            {cdmPercent(cdm)}
          </span>
          <span className="sr-only">{widevineShort(view, cdm)}</span>
        </>
      );
    }
    return null;
  };

  return (
    <nav aria-label={t`Setup steps`}>
      <ol className="flex flex-col gap-0.5 max-[960px]:flex-row max-[960px]:flex-wrap">
        {setupSteps.map((step) => (
          <li key={step}>
            <Link
              to="/welcome"
              search={{ step }}
              className="flex h-8 items-center justify-between gap-3 rounded-md px-2.5 text-ui text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink aria-[current=page]:bg-active aria-[current=page]:font-medium aria-[current=page]:text-ink"
            >
              <span>{labels[step]}</span> {mark(step)}
            </Link>
          </li>
        ))}
      </ol>
    </nav>
  );
}

/**
 * The first-run setup (/welcome): welcome, Apple Music sign-in, Widevine,
 * listening preferences, done. It is a page inside the shell, so the
 * sidebar and everything else stay a click away; every step can be
 * skipped, and "Skip setup" ends it. The window opens here on a first
 * start (Go's services.StartRoute); Settings → General opens it again.
 */
export function SetupPage() {
  const { t } = useLingui();
  const { step } = route.useSearch();
  const navigate = useNavigate();
  const leave = useLeaveSetup();

  // A new step is announced by its title: focus moves there, not to the page.
  useEffect(() => {
    document.getElementById(STEP_TITLE_ID)?.focus();
  }, [step]);

  const next = () => void navigate({ to: "/welcome", search: { step: stepAfter(step) } });

  let body: ReactNode;
  switch (step) {
    case "welcome":
      body = <WelcomeStep onNext={next} />;
      break;
    case "account":
      body = <AccountStep onNext={next} />;
      break;
    case "widevine":
      body = <WidevineStep onNext={next} />;
      break;
    case "listening":
      body = <ListeningStep onNext={next} />;
      break;
    case "done":
      body = <DoneStep />;
      break;
  }

  return (
    <div className="grid grid-cols-[180px_minmax(0,600px)] gap-10 pt-2 max-[960px]:grid-cols-1 max-[960px]:gap-6">
      <div className="flex flex-col items-start gap-4">
        <StepList />
        {step !== "done" && (
          <Button
            variant="ghost"
            size="sm"
            className="px-2.5 text-ink-muted"
            title={t`Ends the setup; Settings → General opens it again.`}
            onClick={() => void leave()}
          >
            <Trans>Skip setup</Trans>
          </Button>
        )}
      </div>
      <section aria-labelledby={STEP_TITLE_ID} className="min-w-0">
        {body}
      </section>
    </div>
  );
}
