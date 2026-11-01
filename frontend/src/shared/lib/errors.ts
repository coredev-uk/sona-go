import { i18n, type MessageDescriptor } from "@lingui/core";
import { msg } from "@lingui/core/macro";

/** A rejected binding call, reduced to Go's stable code and a message for the user. */
export interface CallFailure {
  /** Services error code from `err.cause.code` (internal/services/errors.go), or "". */
  code: string;
  /**
   * The user-facing message in the active locale: the translation for a
   * known code, else Go's text (`detail`).
   */
  message: string;
  /** Go's error text without its package prefix ("appleid: …"); English, for matching and logs. */
  detail: string;
}

/**
 * The wording for every code in internal/services/errors.go except
 * `unknown`, which keeps Go's message. A code missing here also falls back
 * to Go's message.
 */
const codeMessages: Record<string, MessageDescriptor> = {
  invalid_credentials: msg`The account name or password isn't right.`,
  incorrect_code: msg`That verification code isn't right.`,
  locked: msg`This Apple Account is locked or temporarily unavailable.`,
  action_required: msg`Your Apple Account needs attention at account.apple.com before it can sign in.`,
  no_subscription: msg`This Apple Account doesn't have an active Apple Music subscription.`,
  unsupported_country: msg`Apple Music isn't available in this account's country or region.`,
  unsupported_2fa: msg`This account's two-factor method can't be completed inside Sona.`,
  out_of_order: msg`The sign-in was replaced or cancelled. Start again.`,
  timeout: msg`That took too long and timed out. Try again.`,
  cancelled: msg`It was cancelled.`,
  network: msg`Sona couldn't reach the network. Check your connection and try again.`,
  mut_rejected: msg`Apple Music no longer accepts your sign-in. Sign in again in Settings → Account.`,
  dev_token_rejected: msg`Apple Music refused Sona's access. Try again later.`,
  not_signed_in: msg`Sign in to Apple Music in Settings → Account.`,
  unauthorized: msg`Apple Music refused the request.`,
  not_found: msg`It couldn't be found. It may have been removed.`,
  not_playable: msg`This song isn't available to stream in your storefront.`,
  not_released: msg`This song isn't out yet. It plays from its release date.`,
  sync_running: msg`A library sync is already running.`,
  private_forced: msg`Private listening stays on because Sona was started with SONA_PRIVATE=1.`,
  invalid_argument: msg`That value isn't valid.`,
  conflict: msg`It changed elsewhere since Sona read it. Reload and try again.`,
  pins_full: msg`You have six pins. Unpin one to make room.`,
  not_configured: msg`This build of Sona has no account for that service.`,
  unsupported: msg`That isn't available on this platform.`,
  no_cdm: msg`Sona needs Google's Widevine to play songs, and it isn't ready yet. Settings → Playback shows its progress.`,
  keyring: msg`Your keyring didn't answer or refused. Unlock it, then try again.`,
};

/** Whether `code` has its own translated wording (else the message is Go's text). */
export function hasCodeMessage(code: string): boolean {
  return Object.hasOwn(codeMessages, code);
}

function field(obj: unknown, key: string): unknown {
  return obj && typeof obj === "object" && key in obj
    ? (obj as Record<string, unknown>)[key]
    : undefined;
}

/**
 * Reads a Wails call rejection: the Go service marshals errors to
 * `err.cause = {code, message}`; anything else falls back to its message.
 * The user-facing message is translated from the code; Go's text is kept
 * for unknown codes.
 */
export function callFailure(err: unknown): CallFailure {
  if (!err) return { code: "", message: "", detail: "" };
  const cause = field(err, "cause");
  const code = field(cause, "code");
  const causeMsg = field(cause, "message");
  const msgField = field(err, "message");
  const raw =
    typeof causeMsg === "string" && causeMsg
      ? causeMsg
      : typeof msgField === "string"
        ? msgField
        : typeof err === "string"
          ? err
          : String(err);
  const detail = raw.replace(/^[a-z]+: /, "");
  const c = typeof code === "string" ? code : "";
  return {
    code: c,
    message: hasCodeMessage(c) ? i18n._(codeMessages[c]) : detail,
    detail,
  };
}
