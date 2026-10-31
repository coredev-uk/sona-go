import { CDMService, type CDMStatus } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { create } from "zustand";

import { numberFormat } from "@/shared/lib/format";
import { run } from "@/shared/lib/run";

/**
 * The Widevine CDM as Go reports it (CDMService.Status, then cdm:status):
 * Sona downloads it from Google after the first paint and keeps it up to
 * date, unless the developer override (SONA_WIDEVINE_CDM) names one. null
 * until Go answers; `state` "" on a platform whose playback needs no CDM.
 */
export const useCdm = create<{ status: CDMStatus | null }>(() => ({ status: null }));

/**
 * Replaces the mirror with Go's latest status. Returns whether more than
 * the download's progress changed (state, version, code or source), so the
 * caller can refresh what depends on the CDM without doing so on every
 * progress step.
 */
export function applyCdmStatus(status: CDMStatus): boolean {
  const prev = useCdm.getState().status;
  useCdm.setState({ status });
  return (
    prev === null ||
    prev.state !== status.state ||
    prev.version !== status.version ||
    prev.code !== status.code ||
    prev.source !== status.source
  );
}

/** The running download's progress from 0 to 1, or null while its size is unknown. */
export function cdmProgress(s: Pick<CDMStatus, "received" | "total">): number | null {
  return s.total > 0 ? Math.min(Math.max(s.received / s.total, 0), 1) : null;
}

/** The running download's progress as a percentage in the active locale ("42%"), or "" while its size is unknown. */
export function cdmPercent(s: Pick<CDMStatus, "received" | "total">): string {
  const fraction = cdmProgress(s);
  return fraction === null
    ? ""
    : numberFormat({ style: "percent", maximumFractionDigits: 0 }).format(fraction);
}
/**
 * Whether trying again can help: a download (never the developer override,
 * which Sona does not fetch) that failed, or whose last update check
 * failed, on a platform Google makes the CDM for.
 */
export function canRetryCdm(s: CDMStatus | null): boolean {
  return (
    s !== null &&
    s.source === "download" &&
    s.code !== "" &&
    s.code !== "unsupported_platform" &&
    (s.state === "failed" || s.state === "ready") &&
    !s.updating
  );
}

/** Asks Go to download the CDM (or check for an update) now; cdm:status follows. */
export function retryCdm(): Promise<boolean> {
  return run(t`Couldn't start the Widevine download`, CDMService.Retry());
}
