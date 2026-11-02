import type { Capabilities, CDMStatus } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { z } from "zod";
import { create } from "zustand";

import { openSetup, useSetupOpen } from "@/shared/lib/setup";

/** The setup's steps, in order. */
export const setupSteps = [
  "welcome",
  "account",
  "playback",
  "connections",
  "appearance",
  "desktop",
  "done",
] as const;
export type SetupStep = (typeof setupSteps)[number];

/** /welcome?step=… — a missing or unknown step starts at the beginning. */
export const setupSearch = z.object({
  step: z.enum(setupSteps).catch("welcome"),
});

/** The step after `step`; the last one stays where it is. */
export function stepAfter(step: SetupStep): SetupStep {
  return setupSteps[Math.min(setupSteps.indexOf(step) + 1, setupSteps.length - 1)];
}

/** The step before `step`; the first one stays where it is. */
export function stepBefore(step: SetupStep): SetupStep {
  return setupSteps[Math.max(setupSteps.indexOf(step) - 1, 0)];
}

/** Each step's name, as the strip and the step titles say it. */
export function useStepNames(): Record<SetupStep, string> {
  const { t } = useLingui();
  return {
    welcome: t`Welcome`,
    account: t`Apple Music`,
    playback: t`Playback`,
    connections: t`Connections`,
    appearance: t`Appearance`,
    desktop: t`Desktop`,
    done: t`Done`,
  };
}

/**
 * The step the setup shows. It lives outside the layer, which a new
 * language remounts with the whole tree.
 */
export const useSetupStep = create<{ step: SetupStep }>(() => ({ step: "welcome" }));

export function goToStep(step: SetupStep) {
  useSetupStep.setState({ step });
}

/** The step the next opening starts at, when it isn't the welcome. */
let requested: SetupStep | null = null;

/** Opens the setup over the app at `step` (the /welcome?step= links). */
export function showSetup(step: SetupStep) {
  if (useSetupOpen.getState().open) {
    goToStep(step);
    return;
  }
  requested = step;
  openSetup();
}

// Every opening starts at the welcome ("Set up Sona again") or at the step
// a link asked for; while it stays open the step is the user's.
useSetupOpen.subscribe((now, before) => {
  if (!now.open || before.open) return;
  goToStep(requested ?? "welcome");
  requested = null;
});

/**
 * What the setup can say about Widevine, from the platform's capabilities
 * and Go's CDM status:
 * - "loading": capabilities or the CDM status haven't come yet;
 * - "unavailable": this build has no playback engine here;
 * - "not-needed": the engine plays without a CDM;
 * - "unsupported": the engine needs one but Google makes none for this
 *   computer (Linux on arm64);
 * - "waiting": the download starts once the window has painted;
 * - "downloading", "ready", "failed": the download's own states.
 */
export type WidevineView =
  | "loading"
  | "unavailable"
  | "not-needed"
  | "unsupported"
  | "waiting"
  | "downloading"
  | "ready"
  | "failed";

export function widevineView(
  caps: Capabilities | undefined,
  status: CDMStatus | null,
): WidevineView {
  if (!caps) return "loading";
  if (!caps.playback.available) return "unavailable";
  if (caps.playback.engine !== "cdm") return "not-needed";
  if (!status) return "loading";
  if (status.code === "unsupported_platform") return "unsupported";
  switch (status.state) {
    case "downloading":
    case "ready":
    case "failed":
      return status.state;
    default:
      return "waiting";
  }
}
