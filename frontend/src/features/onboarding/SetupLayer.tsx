import { Trans, useLingui } from "@lingui/react/macro";
import { ChevronLeft } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { SonaMark } from "@/shared/brand/SonaMark";
import { closeAccountDialog, useAccountDialog } from "@/shared/lib/account";
import { confirm } from "@/shared/lib/confirm";
import { useSetupOpen } from "@/shared/lib/setup";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/shared/ui/dialog";

import { AccountStep } from "./AccountStep";
import { AppearanceStep } from "./AppearanceStep";
import { useLeaveSetup } from "./complete";
import { ConnectionsStep } from "./ConnectionsStep";
import { DesktopStep } from "./DesktopStep";
import { DoneStep } from "./DoneStep";
import { SetupFoot } from "./foot";
import { STEP_TITLE_ID } from "./parts";
import { PlaybackStep } from "./PlaybackStep";
import { StepProgress } from "./StepProgress";
import {
  goToStep,
  setupSteps,
  stepAfter,
  stepBefore,
  useSetupStep,
  useStepNames,
  type SetupStep,
} from "./steps";
import { WelcomeStep } from "./WelcomeStep";

const DESCRIPTION_ID = "setup-step-description";

const bodies: Record<SetupStep, ReactNode> = {
  welcome: <WelcomeStep />,
  account: <AccountStep />,
  playback: <PlaybackStep />,
  connections: <ConnectionsStep />,
  appearance: <AppearanceStep />,
  desktop: <DesktopStep />,
  done: <DoneStep />,
};

/** Each step's title and the one line under it. */
function useStepText(step: SetupStep, signedIn: boolean): { title: string; line: string } {
  const { t } = useLingui();
  const names = useStepNames();
  switch (step) {
    case "welcome":
      return {
        title: t`Welcome to Sona`,
        line: t`Sona plays Apple Music on this computer. A few short steps sign you in and set how it plays, looks and starts. Each can be skipped and changed later in Settings.`,
      };
    case "account":
      return {
        title: names.account,
        line: t`Signing in brings your library, playlists and recommendations, and plays whole songs. Sona keeps the sign-in in your keyring and never saves your password.`,
      };
    case "playback":
      return { title: names.playback, line: t`What plays your music and how it sounds.` };
    case "connections":
      return {
        title: names.connections,
        line: t`Optional. Keep a record of what you play, or show it on Discord.`,
      };
    case "appearance":
      return { title: names.appearance, line: t`Each choice applies at once, to the app behind.` };
    case "desktop":
      return {
        title: names.desktop,
        line: t`How Sona starts, and what it does in the background.`,
      };
    case "done":
      return {
        title: signedIn ? t`You're all set` : t`Setup is done`,
        line: t`Settings has all of this, and Settings → General runs the setup again.`,
      };
  }
}

/**
 * The first-run setup: a modal sheet over the live app (a module layer, so
 * it covers the sidebar, top bar and player bar, which stay inert behind
 * the scrim), sized to its step. Its frame: where the setup is (a strip
 * of the steps), the step's title and line, the step, and its way on along
 * the foot. Every step can be skipped ("Not now", Continue, a later step in
 * the strip); "Skip setup" ends it at once, and Escape asks first. Opened
 * by Go on a first start (/welcome, services.StartRoute), its ?step= links
 * and Settings → General's "Set up Sona again".
 */
export function SetupLayer() {
  const { t } = useLingui();
  const open = useSetupOpen((s) => s.open);
  const step = useSetupStep((s) => s.step);
  const leave = useLeaveSetup();
  const signedIn = hasAccount(useAuth().data?.state);
  const text = useStepText(step, signedIn);
  const body = useRef<HTMLDivElement>(null);
  const title = useRef<HTMLHeadingElement>(null);
  const [foot, setFoot] = useState<HTMLElement | null>(null);

  // A new step starts at its top and is announced by its title: focus moves there.
  useEffect(() => {
    if (!open) return;
    if (body.current) body.current.scrollTop = 0;
    title.current?.focus();
  }, [open, step]);

  // The account dialog belongs to the top bar's account menu, behind the
  // setup: a "Sign in" asked for while the setup is open (a shortcut, the
  // palette) goes to the setup's own Apple Music step instead, so there is
  // only ever one sign-in form on screen.
  const dialogOpen = useAccountDialog((d) => d.open);
  useEffect(() => {
    if (!open || !dialogOpen) return;
    closeAccountDialog();
    goToStep("account");
  }, [open, dialogOpen]);

  const askToSkip = async () => {
    const skip = await confirm({
      title: t`Skip the rest of the setup?`,
      body: t`What you've chosen so far is kept. Settings → General runs the setup again.`,
      confirmLabel: t`Skip setup`,
    });
    if (skip) await leave();
  };

  const next = () => goToStep(stepAfter(step));
  let forward: ReactNode;
  if (step === "welcome") {
    forward = (
      <Button size="sm" onClick={next}>
        <Trans>Get started</Trans>
      </Button>
    );
  } else if (step === "account" && !signedIn) {
    // The sign-in form's own Sign in lands in the span (SetupFoot) as the
    // primary action; "Not now" is the way past it.
    forward = (
      <>
        <Button variant="ghost" size="sm" onClick={next}>
          <Trans>Not now</Trans>
        </Button>
        <span ref={setFoot} className="contents" />
      </>
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
  const position = setupSteps.indexOf(step) + 1;
  const total = setupSteps.length;

  return (
    <Dialog open={open}>
      <DialogContent
        showCloseButton={false}
        aria-labelledby={STEP_TITLE_ID}
        aria-describedby={DESCRIPTION_ID}
        // Its top edge stays put from step to step (the strip and the foot
        // don't jump); a step taller than the window scrolls inside.
        className="top-[max(1.5rem,10vh)] flex max-h-[calc(100%-max(1.5rem,10vh)-1.5rem)] w-[min(40rem,calc(100%-3rem))] max-w-none translate-y-0 flex-col gap-0 overflow-hidden rounded-lg border-0 bg-raised p-0 shadow-float sm:max-w-none dark:ring-1 dark:ring-white/10"
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          title.current?.focus();
        }}
        onCloseAutoFocus={(e) => {
          // A new language remounts the layer still open: focus stays in the new one.
          if (useSetupOpen.getState().open) {
            e.preventDefault();
            return;
          }
          // Opened at start-up, nothing was focused before it: the page takes focus.
          setTimeout(() => {
            const now = document.activeElement;
            if (!now || now === document.body) document.getElementById("content")?.focus();
          });
        }}
        onEscapeKeyDown={(e) => {
          e.preventDefault();
          // On Done there is nothing left to skip: Escape is Done.
          void (step === "done" ? leave() : askToSkip());
        }}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <div className="flex shrink-0 flex-col gap-2 px-6 pt-4">
          <div className="flex h-8 items-center gap-2">
            <SonaMark />
            <p className="text-ui font-semibold text-ink">
              <Trans>Set up Sona</Trans>
            </p>
            <p className="text-meta text-ink-faint num">{t`${position} of ${total}`}</p>
            <div className="flex-1" />
            {step !== "done" && (
              <Button
                variant="ghost"
                size="sm"
                className="-me-2.5 text-ink-muted"
                title={t`Ends the setup; Settings → General opens it again.`}
                onClick={() => void leave()}
              >
                <Trans>Skip setup</Trans>
              </Button>
            )}
          </div>
          <StepProgress step={step} />
        </div>
        <div ref={body} className="min-h-0 flex-1 overflow-y-auto px-6 pt-5 pb-6">
          <DialogTitle
            ref={title}
            id={STEP_TITLE_ID}
            tabIndex={-1}
            className="text-page leading-(--text-page--line-height) text-balance text-ink focus:outline-none"
          >
            {text.title}
          </DialogTitle>
          <DialogDescription
            id={DESCRIPTION_ID}
            className="mt-1.5 max-w-[60ch] text-ui text-pretty text-ink-muted"
          >
            {text.line}
          </DialogDescription>
          <div className="mt-5">
            <SetupFoot.Provider value={foot}>{bodies[step]}</SetupFoot.Provider>
          </div>
        </div>
        <div className="flex h-16 shrink-0 items-center gap-3 border-t border-line px-6">
          {step !== "welcome" && (
            <Button
              variant="ghost"
              size="sm"
              className="-ms-2.5"
              onClick={() => goToStep(stepBefore(step))}
            >
              <ChevronLeft strokeWidth={1.75} />
              <Trans>Back</Trans>
            </Button>
          )}
          <div className="flex-1" />
          {forward}
        </div>
      </DialogContent>
    </Dialog>
  );
}
