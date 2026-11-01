import { t } from "@lingui/core/macro";
import type { SecondFactor } from "@bindings/services";

import { callFailure } from "@/shared/lib/errors";

/**
 * The native sign-in flow (docs/auth.md §4.7) as a pure reducer: the
 * Account form dispatches what the user did and what Go answered; the
 * reducer decides which step to show and which error, with which next step.
 */

export type DeliveryMode = "sms" | "voice";

/** Where the code the user is typing was sent. phoneId 0 = trusted device. */
export interface CodeTarget {
  phoneId: number;
  mode: DeliveryMode;
}

export type SignInErrorCode =
  | "invalid_credentials"
  | "incorrect_code"
  | "locked"
  | "action_required"
  | "no_subscription"
  | "unsupported_country"
  | "unsupported_2fa"
  | "out_of_order"
  | "cancelled"
  | "timeout"
  | "network"
  | "keyring"
  | "unknown";

export interface SignInError {
  code: SignInErrorCode;
  /** Go's or Apple's message, shown when the code has no fixed wording. */
  message: string;
}

/** What the user can do next about an error. */
export type NextStep = "retry" | "code" | "browser" | "account-site" | "restart";

export interface ErrorCopy {
  title: string;
  detail: string;
  next: NextStep;
}

const codePattern: [SignInErrorCode, RegExp][] = [
  ["invalid_credentials", /incorrect apple account or password|-20101|invalid_credentials/i],
  ["incorrect_code", /incorrect verification code|-21669|incorrect_code/i],
  ["locked", /locked|temporarily unavailable|-20209|-20283|-24761/i],
  ["action_required", /needs attention|action_required|\b412\b/i],
  ["no_subscription", /subscription required|no_subscription|no-subscription/i],
  ["unsupported_country", /not available in this account's country|unsupported_country/i],
  ["unsupported_2fa", /two-factor method is not supported|unsupported_2fa/i],
  ["out_of_order", /out of order|out_of_order/i],
  ["cancelled", /cancel/i],
  ["keyring", /keyring/i],
  ["timeout", /timed out|deadline exceeded/i],
  ["network", /network|no such host|connection refused|connection reset|dial tcp|offline/i],
];

const knownCodes = new Set<string>(codePattern.map(([c]) => c));

/**
 * Classifies a rejected binding call, or a Status.lastError with its
 * lastErrorCode. Prefers the stable Services code and falls back to
 * matching Go's error text.
 */
export function classifySignInError(err: unknown, statusCode = ""): SignInError {
  const failure = callFailure(err);
  const code = statusCode || failure.code;
  const message = failure.message;
  if (knownCodes.has(code)) return { code: code as SignInErrorCode, message };
  for (const [c, re] of codePattern) {
    if (re.test(failure.detail)) return { code: c, message };
  }
  return { code: "unknown", message: message || t`Sign-in failed.` };
}

/** The message and next step for each case in docs/auth.md §4.7. */
export function errorCopy(e: SignInError): ErrorCopy {
  switch (e.code) {
    case "invalid_credentials":
      return {
        title: t`That Apple Account or password isn't right`,
        detail: t`Check the email or phone number and the password, then try again.`,
        next: "retry",
      };
    case "incorrect_code":
      return {
        title: t`That code isn't right`,
        detail: t`Enter the latest code you received. Older codes stop working once a new one is sent.`,
        next: "code",
      };
    case "locked":
      return {
        title: t`This Apple Account is locked`,
        detail: t`Apple has locked the account or made it temporarily unavailable. Unlock it at iforgot.apple.com, or sign in with your browser.`,
        next: "browser",
      };
    case "action_required":
      return {
        title: t`Your Apple Account needs attention`,
        detail: t`Apple wants you to review your account (new terms or a security update) before it can sign in. Do that at account.apple.com, then try again here.`,
        next: "account-site",
      };
    case "no_subscription":
      return {
        title: t`Apple Music subscription required`,
        detail: t`This Apple Account doesn't have an active Apple Music subscription. Subscribe on another device, or sign in with an account that has one.`,
        next: "retry",
      };
    case "unsupported_country":
      return {
        title: t`Apple Music isn't available in this account's country`,
        detail: t`The account's country or region has no Apple Music storefront.`,
        next: "retry",
      };
    case "unsupported_2fa":
      return {
        title: t`This account needs your browser to sign in`,
        detail: t`Its two-factor method (such as a security key) can't be completed inside Sona. Sign in with your browser instead.`,
        next: "browser",
      };
    case "out_of_order":
    case "cancelled":
      return {
        title: t`This sign-in was cancelled`,
        detail: t`It was replaced or cancelled before the code arrived. Start again.`,
        next: "restart",
      };
    case "timeout":
      return {
        title: t`Sign-in timed out`,
        detail: t`No code was entered in time, so Apple closed the attempt. Start again.`,
        next: "restart",
      };
    case "network":
      return {
        title: t`Couldn't reach Apple`,
        detail: t`Check your connection and try again.`,
        next: "retry",
      };
    case "keyring":
      return {
        title: t`Sona couldn't save your sign-in`,
        detail: t`Your keyring didn't answer or refused, so nothing was saved. Unlock GNOME Keyring or KWallet, then sign in again.`,
        next: "retry",
      };
    case "unknown":
      return { title: t`Sign-in didn't work`, detail: e.message, next: "retry" };
  }
}

/** Errors after which Go keeps the two-factor attempt open (docs/auth.md §4.7). */
const attemptStaysOpen: Partial<Record<SignInErrorCode, true>> = {
  incorrect_code: true,
  network: true,
  unknown: true,
};

export type SignInStep =
  | { kind: "credentials"; busy: boolean; error?: SignInError }
  | {
      kind: "code";
      factor: SecondFactor;
      target: CodeTarget;
      busy: boolean;
      error?: SignInError;
      /** One-line confirmation, e.g. after a resend. */
      notice?: string;
    }
  | {
      kind: "choose-phone";
      factor: SecondFactor;
      /** Text or call, preselected from the method the user picked. */
      mode: DeliveryMode;
      busy: boolean;
      error?: SignInError;
    }
  | { kind: "done" };

export type SignInAction =
  | { type: "submit" }
  | { type: "failed"; error: SignInError }
  | { type: "second-factor"; factor: SecondFactor }
  | { type: "signed-in" }
  /** Pick a number for a text or call (accounts with several numbers). */
  | { type: "choose-phone"; mode?: DeliveryMode }
  /** Switch back to the code Apple showed on the trusted devices. */
  | { type: "use-device" }
  | { type: "send-to-phone"; phoneId: number; mode: DeliveryMode }
  | { type: "sent"; target: CodeTarget }
  | { type: "resend" }
  | { type: "resent" }
  | { type: "cancel" }
  /** The auth status changed under the form (another window, timeout, restart). */
  | {
      type: "status";
      state: string;
      factor?: SecondFactor | null;
      lastError?: string;
      lastErrorCode?: string;
    };

export const initialSignIn: SignInStep = { kind: "credentials", busy: false };

/** How the user gets the code; "device" is the Apple device prompt. */
export type CodeMethod = "device" | DeliveryMode;

/** The method a step is currently on, for the method switcher. */
export function currentMethod(step: SignInStep): CodeMethod | undefined {
  if (step.kind === "code") return step.target.phoneId === 0 ? "device" : step.target.mode;
  if (step.kind === "choose-phone") return step.mode;
  return undefined;
}

/**
 * The step a fresh challenge opens on. Every route the account has is
 * offered: with trusted devices Apple has already prompted them, so the
 * device code comes first and text or call stay one click away. Without
 * devices: the number Apple already texted, else the number choice.
 */
export function stepForFactor(factor: SecondFactor): SignInStep {
  const phones = factor.phones;
  if (factor.trustedDevices) {
    return { kind: "code", factor, target: { phoneId: 0, mode: "sms" }, busy: false };
  }
  if (factor.sentToPhone) {
    const phone = phones.find((p) => p.id === factor.sentToPhone);
    return {
      kind: "code",
      factor,
      target: { phoneId: factor.sentToPhone, mode: phone?.mode === "voice" ? "voice" : "sms" },
      busy: false,
    };
  }
  if (phones.length > 0) {
    const mode = phones[0].mode === "voice" ? "voice" : "sms";
    return { kind: "choose-phone", factor, mode, busy: false };
  }
  return {
    kind: "credentials",
    busy: false,
    error: { code: "unsupported_2fa", message: t`No way to receive a code.` },
  };
}

export function signInReducer(step: SignInStep, action: SignInAction): SignInStep {
  switch (action.type) {
    case "submit":
    case "resend":
    case "send-to-phone":
      if (step.kind === "done") return step;
      return {
        ...step,
        busy: true,
        error: undefined,
        ...(step.kind === "code" && { notice: undefined }),
      };

    case "failed": {
      const { error } = action;
      if (step.kind === "code" || step.kind === "choose-phone") {
        if (attemptStaysOpen[error.code]) return { ...step, busy: false, error };
        return { kind: "credentials", busy: false, error };
      }
      return { kind: "credentials", busy: false, error };
    }

    case "second-factor":
      return stepForFactor(action.factor);

    case "signed-in":
      return { kind: "done" };

    case "choose-phone": {
      if (step.kind !== "code" && step.kind !== "choose-phone") return step;
      if (step.factor.phones.length === 0) return step;
      const mode =
        action.mode ??
        (step.kind === "choose-phone"
          ? step.mode
          : step.target.phoneId === 0
            ? "sms"
            : step.target.mode);
      return { kind: "choose-phone", factor: step.factor, mode, busy: false };
    }

    case "use-device":
      // Accounts without an Apple device have no device route.
      if ((step.kind !== "code" && step.kind !== "choose-phone") || !step.factor.trustedDevices) {
        return step;
      }
      return {
        kind: "code",
        factor: step.factor,
        target: { phoneId: 0, mode: "sms" },
        busy: false,
      };

    case "sent":
      if (step.kind !== "code" && step.kind !== "choose-phone") return step;
      return {
        kind: "code",
        factor: step.factor,
        target: action.target,
        busy: false,
        notice:
          action.target.mode === "voice"
            ? t`Apple is calling with your code.`
            : t`Apple sent a code by text message.`,
      };

    case "resent":
      if (step.kind !== "code") return step;
      return {
        ...step,
        busy: false,
        error: undefined,
        notice:
          step.target.phoneId === 0
            ? t`A new code is on its way to your devices.`
            : t`A new code is on its way.`,
      };

    case "cancel":
      return initialSignIn;

    case "status": {
      if (action.state === "signed_in" || action.state === "validating") {
        return step.kind === "credentials" && !step.busy ? step : { kind: "done" };
      }
      if (action.state === "signing_in" && action.factor) {
        // Restore a pending challenge the form doesn't know about yet.
        return step.kind === "credentials" && !step.busy ? stepForFactor(action.factor) : step;
      }
      if (step.kind === "code" || step.kind === "choose-phone") {
        // Go dropped the attempt (deadline, cancel, sign-out elsewhere).
        const error = action.lastError
          ? classifySignInError(action.lastError, action.lastErrorCode)
          : { code: "cancelled" as const, message: "" };
        return { kind: "credentials", busy: false, error };
      }
      return step;
    }
  }
}
