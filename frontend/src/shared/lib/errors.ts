import { i18n, type MessageDescriptor } from "@lingui/core";
import { msg } from "@lingui/core/macro";
// From models, not the index: tests mock "@bindings/services" (the calls).
import { ErrorCode } from "@bindings/services/models";

/** A rejected binding call, reduced to Go's stable code and a message for the user. */
export interface CallFailure {
  /** Services error code from `err.cause.code` (internal/services/errors.go), or "". */
  code: string;
  /**
   * The user-facing message in the active locale: the wording for the
   * code, or a generic message for an unknown one. Never Go's text.
   */
  message: string;
  /** Go's error text without its package prefix ("appleid: …"); English, for logs only. */
  detail: string;
  /**
   * Apple's own wording of a sign-in failure Sona has no code for
   * (`err.cause.appleMessage`, code `unknown`); Apple localised it. "" otherwise.
   */
  appleMessage: string;
}

/**
 * The wording for every code in internal/services/errors.go. Keyed by the
 * generated enum, so a code without wording fails the type check.
 */
const codeMessages: Record<ErrorCode, MessageDescriptor> = {
  [ErrorCode.$zero]: msg`Something went wrong.`,
  [ErrorCode.CodeUnknown]: msg`Something went wrong.`,
  [ErrorCode.CodeInvalidCredentials]: msg`The account name or password isn't right.`,
  [ErrorCode.CodeIncorrectCode]: msg`That verification code isn't right.`,
  [ErrorCode.CodeLocked]: msg`This Apple Account is locked or temporarily unavailable.`,
  [ErrorCode.CodeActionRequired]: msg`Your Apple Account needs attention at account.apple.com before it can sign in.`,
  [ErrorCode.CodeNoSubscription]: msg`This Apple Account doesn't have an active Apple Music subscription.`,
  [ErrorCode.CodeUnsupportedCountry]: msg`Apple Music isn't available in this account's country or region.`,
  [ErrorCode.CodeUnsupported2FA]: msg`This account's two-factor method can't be completed inside Sona.`,
  [ErrorCode.CodeOutOfOrder]: msg`The sign-in was replaced or cancelled. Start again.`,
  [ErrorCode.CodeTimeout]: msg`That took too long and timed out. Try again.`,
  [ErrorCode.CodeCancelled]: msg`It was cancelled.`,
  [ErrorCode.CodeNetwork]: msg`Sona couldn't reach the network. Check your connection and try again.`,
  [ErrorCode.CodeMUTRejected]: msg`Apple Music no longer accepts your sign-in. Sign in again from the account menu in the top bar.`,
  [ErrorCode.CodeDevTokenRejected]: msg`Apple Music refused Sona's access. Try again later.`,
  [ErrorCode.CodeNotSignedIn]: msg`Sign in to Apple Music from the account menu in the top bar.`,
  [ErrorCode.CodeUnauthorized]: msg`Apple Music refused the request.`,
  [ErrorCode.CodeNotFound]: msg`It couldn't be found. It may have been removed.`,
  [ErrorCode.CodeNotPlayable]: msg`This song isn't available to stream in your storefront.`,
  [ErrorCode.CodeLiveStation]: msg`This isn't available while listening to a live station.`,
  [ErrorCode.CodeStation]: msg`This isn't available while listening to a station.`,
  [ErrorCode.CodeNotReleased]: msg`This song isn't out yet. It plays from its release date.`,
  [ErrorCode.CodeSyncRunning]: msg`A library sync is already running.`,
  [ErrorCode.CodePrivateForced]: msg`Private listening stays on because Sona was started with SONA_PRIVATE=1.`,
  [ErrorCode.CodeInvalidArgument]: msg`That value isn't valid.`,
  [ErrorCode.CodeNameRequired]: msg`Enter a name.`,
  [ErrorCode.CodeFolderNotEmpty]: msg`The folder isn't empty. Move or delete what's in it first.`,
  [ErrorCode.CodeNotAppleMusicLink]: msg`That isn't an Apple Music link.`,
  [ErrorCode.CodeClipboard]: msg`Sona couldn't use the clipboard.`,
  [ErrorCode.CodeTokenRequired]: msg`Paste your user token first.`,
  [ErrorCode.CodeBrowserOpen]: msg`Sona couldn't open your web browser. Open the link yourself.`,
  [ErrorCode.CodeSignOutIncomplete]: msg`Sona signed out, but couldn't remove everything it saved. Unlock your keyring, then sign out again.`,
  [ErrorCode.CodeStoredUnreadable]: msg`Your saved sign-in couldn't be read, so Sona removed it. Sign in again.`,
  [ErrorCode.CodeSettingsNotExport]: msg`That file isn't a Sona settings export.`,
  [ErrorCode.CodeSettingsNewerExport]: msg`That file was exported by a newer version of Sona. Update Sona, then import it.`,
  [ErrorCode.CodeSettingsBadValue]: msg`That file has a setting Sona can't use, so nothing changed.`,
  [ErrorCode.CodeSettingsRead]: msg`Sona couldn't read that file.`,
  [ErrorCode.CodeSettingsWrite]: msg`Sona couldn't save the file there.`,
  [ErrorCode.CodeNoRendition]: msg`Apple doesn't offer this video in a format Sona can play.`,
  [ErrorCode.CodeConflict]: msg`It changed elsewhere since Sona read it. Reload and try again.`,
  [ErrorCode.CodePinsFull]: msg`You have six pins. Unpin one to make room.`,
  [ErrorCode.CodeNotConfigured]: msg`This build of Sona has no account for that service.`,
  [ErrorCode.CodeUnsupported]: msg`That isn't available on this platform.`,
  [ErrorCode.CodeNoCDM]: msg`Sona needs Google's Widevine to play songs, and it isn't ready yet. Settings → Playback shows its progress.`,
  [ErrorCode.CodeVideoHDOnly]: msg`Apple offers this video only in HD, which needs a hardware-protected player. Sona plays videos in standard definition.`,
  [ErrorCode.CodeVideoLicence]: msg`Apple refused the licence for this video, or it can't be used on this computer.`,
  [ErrorCode.CodeKeyring]: msg`Your keyring didn't answer or refused. Unlock it, then try again.`,
};

/** Whether `code` is one of Go's error codes. */
function isErrorCode(code: string): code is ErrorCode {
  return Object.hasOwn(codeMessages, code);
}

/**
 * The wording for a code Go sent in a DTO (AuthStatus.lastErrorCode, a sync
 * event…): an unknown or unrecognised code gets the generic message.
 */
export function codeMessage(code: string): string {
  return i18n._(codeMessages[isErrorCode(code) ? code : ErrorCode.CodeUnknown]);
}

function field(obj: unknown, key: string): unknown {
  return obj && typeof obj === "object" && key in obj
    ? (obj as Record<string, unknown>)[key]
    : undefined;
}

/**
 * Reads a Wails call rejection: the Go service marshals errors to
 * `err.cause = {code, message}`; anything else falls back to its message.
 * The user-facing message is worded from the code; Go's text stays in
 * `detail`, for logs.
 */
export function callFailure(err: unknown): CallFailure {
  if (!err) return { code: "", message: "", detail: "", appleMessage: "" };
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
  const apple = field(cause, "appleMessage");
  return {
    code: c,
    message: codeMessage(c),
    detail,
    appleMessage: typeof apple === "string" ? apple : "",
  };
}
