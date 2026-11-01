import type { Capabilities, CDMStatus } from "@bindings/services";
import { z } from "zod";

/** The setup's steps, in order. */
export const setupSteps = ["welcome", "account", "widevine", "listening", "done"] as const;
export type SetupStep = (typeof setupSteps)[number];

/** /welcome?step=… — a missing or unknown step starts at the beginning. */
export const setupSearch = z.object({
  step: z.enum(setupSteps).catch("welcome"),
});

/** The step after `step`; the last one stays where it is. */
export function stepAfter(step: SetupStep): SetupStep {
  return setupSteps[Math.min(setupSteps.indexOf(step) + 1, setupSteps.length - 1)];
}

/**
 * What the Widevine step can say, from the platform's capabilities and
 * Go's CDM status:
 * - "loading": capabilities or the CDM status haven't come yet;
 * - "unavailable": this build has no playback engine here (Windows today);
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
