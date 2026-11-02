import type { SecondFactor } from "@bindings/services";
import { ErrorCode } from "@bindings/services/models";
import { describe, expect, it } from "vitest";

import { callFailure } from "@/shared/lib/errors";

import {
  classifySignInError,
  currentMethod,
  errorCopy,
  initialSignIn,
  signInReducer,
  type SignInAction,
  type SignInStep,
} from "./signin";

const device: SecondFactor = {
  codeLength: 6,
  trustedDevices: true,
  phones: [
    { id: 1, number: "+44 ••••• ••••12", mode: "sms" },
    { id: 2, number: "+1 (•••) •••-••34", mode: "voice" },
  ],
  sentToPhone: 0,
};
const smsOnly: SecondFactor = {
  codeLength: 6,
  trustedDevices: false,
  phones: [
    { id: 1, number: "+44 ••••• ••••12", mode: "sms" },
    { id: 2, number: "+1 (•••) •••-••34", mode: "voice" },
  ],
  sentToPhone: 2,
};
const phonesOnly: SecondFactor = { ...smsOnly, sentToPhone: 0 };

const run = (actions: SignInAction[], from: SignInStep = initialSignIn) =>
  actions.reduce(signInReducer, from);

describe("classifySignInError", () => {
  it("prefers a stable code from the error cause", () => {
    const err = Object.assign(new Error("boom"), {
      cause: { code: "locked", message: "This Apple Account has been locked." },
    });
    // The message is the translated wording for the code, not Go's text.
    expect(classifySignInError(err)).toEqual({
      code: "locked",
      message: callFailure(err).message,
    });
    expect(callFailure(err).message).not.toBe("This Apple Account has been locked.");
  });

  it("classifies by code, never by Go's text", () => {
    expect(classifySignInError(new Error("auth: sign-in timed out")).code).toBe("unknown");
    expect(classifySignInError(undefined, ErrorCode.CodeTimeout).code).toBe("timeout");
    const other = classifySignInError({
      cause: { code: "dev_token_rejected", message: "auth: x" },
    });
    expect(other.code).toBe("unknown");
    expect(other.message).toBe(callFailure({ cause: { code: "dev_token_rejected" } }).message);
  });

  it("shows Apple's own message for a sign-in code Sona doesn't map", () => {
    const french = "Ce compte Apple est désactivé.";
    const fromCall = classifySignInError({
      cause: { code: "unknown", message: "appleid: x (-99999)", appleMessage: french },
    });
    expect(fromCall).toEqual({ code: "unknown", message: french });
    expect(errorCopy(fromCall).detail).toBe(french);
    expect(classifySignInError(undefined, ErrorCode.CodeUnknown, french).message).toBe(french);
    // A code Sona words keeps its own wording; Go's text never shows.
    expect(classifySignInError(undefined, ErrorCode.CodeLocked, french).message).not.toBe(french);
    expect(classifySignInError({ cause: { code: "unknown", message: "x" } }).message).toBe(
      "Something went wrong.",
    );
  });

  it("gives every code a title and a next step", () => {
    const unknown = classifySignInError({
      cause: { code: "unknown", message: "Too many codes requested." },
    });
    expect(errorCopy(unknown)).toMatchObject({ detail: "Something went wrong.", next: "retry" });
    expect(errorCopy({ code: "locked", message: "" }).next).toBe("browser");
    expect(errorCopy({ code: "unsupported_2fa", message: "" }).next).toBe("browser");
    expect(errorCopy({ code: "action_required", message: "" }).next).toBe("account-site");
    expect(errorCopy({ code: "timeout", message: "" }).next).toBe("restart");
    expect(errorCopy({ code: "keyring", message: "" }).next).toBe("retry");
  });
});

describe("signInReducer", () => {
  it("opens the trusted-device code step with the code length", () => {
    const step = run([{ type: "submit" }, { type: "second-factor", factor: device }]);
    expect(step).toMatchObject({ kind: "code", target: { phoneId: 0 }, busy: false });
    expect(step.kind === "code" && step.factor.codeLength).toBe(6);
  });

  it("targets the phone Apple already texted", () => {
    const step = run([{ type: "second-factor", factor: smsOnly }]);
    expect(step).toMatchObject({ kind: "code", target: { phoneId: 2, mode: "voice" } });
  });

  it("asks for a phone when no code was sent anywhere", () => {
    const step = run([{ type: "second-factor", factor: phonesOnly }]);
    expect(step.kind).toBe("choose-phone");
    const sent = run(
      [
        { type: "send-to-phone", phoneId: 1, mode: "sms" },
        { type: "sent", target: { phoneId: 1, mode: "sms" } },
      ],
      step,
    );
    expect(sent).toMatchObject({ kind: "code", target: { phoneId: 1, mode: "sms" } });
    expect(sent.kind === "code" && sent.notice).toMatch(/text message/);
  });

  it("switches a single number from text to a call", () => {
    const oneNumber: SecondFactor = {
      codeLength: 6,
      trustedDevices: false,
      phones: [{ id: 1, number: "+44 ••••• ••••12", mode: "sms" }],
      sentToPhone: 1,
    };
    const step = run([
      { type: "second-factor", factor: oneNumber },
      { type: "send-to-phone", phoneId: 1, mode: "voice" },
      { type: "sent", target: { phoneId: 1, mode: "voice" } },
    ]);
    expect(step).toMatchObject({
      kind: "code",
      target: { phoneId: 1, mode: "voice" },
      busy: false,
    });
    expect(step.kind === "code" && step.notice).toMatch(/calling/);
  });

  it("treats a challenge with no route as unsupported", () => {
    const step = run([
      {
        type: "second-factor",
        factor: { codeLength: 6, trustedDevices: false, phones: [], sentToPhone: 0 },
      },
    ]);
    expect(step).toMatchObject({ kind: "credentials", error: { code: "unsupported_2fa" } });
  });

  it("keeps the code step open on a wrong code", () => {
    const step = run([
      { type: "second-factor", factor: device },
      { type: "submit" },
      { type: "failed", error: { code: "incorrect_code", message: "" } },
    ]);
    expect(step).toMatchObject({ kind: "code", busy: false, error: { code: "incorrect_code" } });
  });

  it("ends the attempt on errors Go ends it for", () => {
    for (const code of ["locked", "action_required", "unsupported_2fa", "timeout"] as const) {
      const step = run([
        { type: "second-factor", factor: device },
        { type: "submit" },
        { type: "failed", error: { code, message: "" } },
      ]);
      expect(step, code).toMatchObject({ kind: "credentials", error: { code } });
    }
  });

  it("clears the error and notice when the user acts again", () => {
    const step = run([
      { type: "second-factor", factor: device },
      { type: "failed", error: { code: "incorrect_code", message: "" } },
      { type: "resend" },
    ]);
    expect(step).toMatchObject({ kind: "code", busy: true, error: undefined });
    expect(signInReducer(step, { type: "resent" })).toMatchObject({
      busy: false,
      notice: "A new code is on its way to your devices.",
    });
  });

  it("lets a device account pick a text or call number instead", () => {
    const step = run([
      { type: "second-factor", factor: device },
      { type: "choose-phone", mode: "voice" },
    ]);
    expect(step).toMatchObject({ kind: "choose-phone", mode: "voice" });
    expect(currentMethod(step)).toBe("voice");
    // And back to the code already on the devices.
    const back = signInReducer(step, { type: "use-device" });
    expect(back).toMatchObject({ kind: "code", target: { phoneId: 0 } });
    expect(currentMethod(back)).toBe("device");
  });

  it("has no device route without an Apple device", () => {
    const step = run([{ type: "second-factor", factor: phonesOnly }]);
    expect(signInReducer(step, { type: "use-device" })).toBe(step);
  });

  it("switches a texted code to another number, keeping the method", () => {
    const step = run([{ type: "second-factor", factor: smsOnly }, { type: "choose-phone" }]);
    expect(step).toMatchObject({ kind: "choose-phone", mode: "voice" });
  });

  it("follows the auth status", () => {
    // A pending challenge from before the form mounted is restored.
    expect(
      signInReducer(initialSignIn, { type: "status", state: "signing_in", factor: device }).kind,
    ).toBe("code");
    // Go dropped the attempt: back to the form with the reason.
    const dropped = run([
      { type: "second-factor", factor: device },
      { type: "status", state: "signed_out", lastErrorCode: ErrorCode.CodeTimeout },
    ]);
    expect(dropped).toMatchObject({ kind: "credentials", error: { code: "timeout" } });
    // Minted: done.
    const done = run([
      { type: "second-factor", factor: device },
      { type: "submit" },
      { type: "status", state: "validating" },
    ]);
    expect(done.kind).toBe("done");
    // An idle form stays as it is when someone is already signed in.
    expect(signInReducer(initialSignIn, { type: "status", state: "signed_in" })).toBe(
      initialSignIn,
    );
  });

  it("cancel returns to an empty form", () => {
    expect(run([{ type: "second-factor", factor: device }, { type: "cancel" }])).toEqual(
      initialSignIn,
    );
  });
});
