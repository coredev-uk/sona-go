import { describe, expect, it } from "vitest";

import { isSignedOut } from "./queries";

describe("isSignedOut", () => {
  it("is not signed out while Go reads the sign-in from the keyring, or before it reported", () => {
    expect(isSignedOut(undefined)).toBe(false);
    expect(isSignedOut("restoring")).toBe(false);
  });

  it("keeps the library when Apple only asks to sign in again", () => {
    expect(isSignedOut("needs_reauth")).toBe(false);
    expect(isSignedOut("signed_in")).toBe(false);
    expect(isSignedOut("validating")).toBe(false);
  });

  it("is signed out without a token, and while a sign-in is still running", () => {
    expect(isSignedOut("signed_out")).toBe(true);
    expect(isSignedOut("signing_in")).toBe(true);
    expect(isSignedOut("error")).toBe(true);
  });
});
