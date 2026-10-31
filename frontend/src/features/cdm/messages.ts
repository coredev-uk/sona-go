import { i18n, type MessageDescriptor } from "@lingui/core";
import { msg } from "@lingui/core/macro";

/**
 * Why there is no Widevine CDM, or why its last update check failed: the
 * wording for every failure code in internal/playback/cdm/errors.go
 * (CDMStatus.code). Plain words for people who have never heard of a CDM:
 * what happened, then what to do, without blame.
 */
const problems: Record<string, MessageDescriptor> = {
  network: msg`Sona couldn't reach Google's download server. Check your internet connection.`,
  unsupported_platform: msg`Google doesn't make Widevine for this computer, so songs can't play here.`,
  manifest_invalid: msg`Sona couldn't tell which version of Widevine to download. Try again later.`,
  untrusted_url: msg`The download wasn't coming from Google, so Sona stopped it. Try again later.`,
  too_large: msg`The download was larger than expected, so Sona stopped it. Try again later.`,
  hash_mismatch: msg`The downloaded file wasn't the one expected, so Sona deleted it. Try again.`,
  signature_invalid: msg`The download isn't signed by Google, so Sona deleted it. Try again later.`,
  archive_invalid: msg`The download was damaged or incomplete, so Sona deleted it. Try again.`,
  load_failed: msg`The downloaded version of Widevine didn't start on this computer, so Sona removed it.`,
  load_check_incomplete: msg`Sona couldn't finish checking the downloaded Widevine. Try again.`,
  disk: msg`Sona couldn't save Widevine on this computer. Make sure there's free disk space, then try again.`,
  override_missing: msg`SONA_WIDEVINE_CDM doesn't point to a Widevine library. Correct the path, then restart Sona.`,
};

/** Whether `code` has its own wording. */
export function hasCdmProblem(code: string): boolean {
  return Object.hasOwn(problems, code);
}

/** The sentence for a failure code; a code Sona doesn't know yet gets a general one. */
export function cdmProblem(code: string): string {
  return hasCdmProblem(code)
    ? i18n._(problems[code])
    : i18n._(msg`Widevine couldn't be downloaded. Try again later.`);
}

/**
 * The short state of a CDM that failed (CDMStatus.state "failed"): its
 * install, when the download arrived and only loading it failed or could
 * not be finished; otherwise the download.
 */
export function cdmFailedState(code: string): string {
  return code === "load_failed" || code === "load_check_incomplete"
    ? i18n._(msg`Couldn't install`)
    : i18n._(msg`Couldn't download`);
}
