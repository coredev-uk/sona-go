import { Trans, useLingui } from "@lingui/react/macro";
import { getRouteApi, useNavigate } from "@tanstack/react-router";
import { ChevronLeft } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { GroupHeadingLevel } from "@/features/settings";
import { closeAccountDialog, useAccountDialog } from "@/shared/lib/account";
import { Button } from "@/shared/ui/button";

import { AccountStep } from "./AccountStep";
import { AppearanceStep } from "./AppearanceStep";
import { useLeaveSetup } from "./complete";
import { ConnectionsStep } from "./ConnectionsStep";
import { DesktopStep } from "./DesktopStep";
import { DoneStep } from "./DoneStep";
import { STEP_TITLE_ID } from "./parts";
import { PlaybackStep } from "./PlaybackStep";
import { StepRail } from "./StepRail";
import { stepAfter, stepBefore, type SetupStep } from "./steps";
import { WelcomeStep } from "./WelcomeStep";

const route = getRouteApi("/welcome");

const bodies: Record<SetupStep, ReactNode> = {
  welcome: <WelcomeStep />,
  account: <AccountStep />,
  playback: <PlaybackStep />,
  connections: <ConnectionsStep />,
  appearance: <AppearanceStep />,
  desktop: <DesktopStep />,
  done: <DoneStep />,
};

/**
 * The first-run setup (/welcome), the whole window (the route is
 * full-window: the shell draws only the title bar's band over its top):
 * the steps down the leading side, each with its live line, and one step
 * at a time in a centred column, its way on in a bar along the foot that
 * stays put while the step scrolls. Every step can be skipped ("Not now",
 * Continue, a later step in the list) and "Skip setup" ends it. The window
 * opens here on a first start (Go's services.StartRoute); Settings →
 * General opens it again.
 */
export function SetupPage() {
  const { t } = useLingui();
  const { step } = route.useSearch();
  const navigate = useNavigate();
  const leave = useLeaveSetup();
  const signedIn = hasAccount(useAuth().data?.state);
  const scroller = useRef<HTMLDivElement>(null);

  // A new step starts at its top and is announced by its title: focus moves there.
  useEffect(() => {
    if (scroller.current) scroller.current.scrollTop = 0;
    document.getElementById(STEP_TITLE_ID)?.focus();
  }, [step]);

  const go = (to: SetupStep) => void navigate({ to: "/welcome", search: { step: to } });

  // The account dialog lives beside the top bar's account menu, which the
  // full-window setup doesn't draw: a "Sign in" from elsewhere (the
  // palette) opens the setup's own Apple Music step instead, so there is
  // only ever one sign-in form and nothing pops up once the setup is left.
  const dialogOpen = useAccountDialog((d) => d.open);
  useEffect(() => {
    if (!dialogOpen) return;
    closeAccountDialog();
    void navigate({ to: "/welcome", search: { step: "account" } });
  }, [dialogOpen, navigate]);
  const next = () => go(stepAfter(step));

  let forward: ReactNode;
  if (step === "welcome") {
    forward = (
      <Button size="sm" onClick={next}>
        <Trans>Get started</Trans>
      </Button>
    );
  } else if (step === "account" && !signedIn) {
    forward = (
      <Button variant="outline" size="sm" onClick={next}>
        <Trans>Not now</Trans>
      </Button>
    );
  } else if (step === "done") {
    forward = (
      <Button size="sm" onClick={() => void leave()}>
        <Trans>Done</Trans>
      </Button>
    );
  } else {
    forward = (
      <Button size="sm" onClick={next}>
        <Trans>Continue</Trans>
      </Button>
    );
  }

  return (
    <GroupHeadingLevel.Provider value={2}>
      <div className="grid h-full grid-cols-[232px_minmax(0,1fr)] bg-ground">
        <nav
          aria-labelledby="setup-steps"
          className="flex min-h-0 flex-col bg-surface px-3 pt-[var(--title-bar-height)] pb-3"
        >
          <div className="-mx-3 mt-4 min-h-0 flex-1 overflow-y-auto px-3 pb-4">
            <StepRail />
          </div>
          {step !== "done" && (
            <Button
              variant="ghost"
              size="sm"
              className="self-start px-2.5 text-ink-muted"
              title={t`Ends the setup; Settings → General opens it again.`}
              onClick={() => void leave()}
            >
              <Trans>Skip setup</Trans>
            </Button>
          )}
        </nav>
        <main
          id="content"
          // Focus target for the skip link (not a Tab stop itself).
          tabIndex={-1}
          className="flex min-h-0 min-w-0 flex-col border-l border-line pt-[var(--title-bar-height)] focus-visible:outline-offset-[-2px]"
        >
          <div
            ref={scroller}
            className="min-h-0 flex-1 [scrollbar-gutter:stable_both-edges] overflow-y-auto"
          >
            <section
              aria-labelledby={STEP_TITLE_ID}
              className="mx-auto w-full max-w-[680px] px-10 pt-8 pb-12 max-[1080px]:px-8"
            >
              {bodies[step]}
            </section>
          </div>
          <div className="shrink-0 border-t border-line">
            <div className="mx-auto flex h-16 w-full max-w-[680px] items-center gap-3 px-10 max-[1080px]:px-8">
              {step !== "welcome" && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="-ms-2.5"
                  onClick={() => go(stepBefore(step))}
                >
                  <ChevronLeft strokeWidth={1.75} />
                  <Trans>Back</Trans>
                </Button>
              )}
              <div className="flex-1" />
              {forward}
            </div>
          </div>
        </main>
      </div>
    </GroupHeadingLevel.Provider>
  );
}
