import { describe, expect, it } from "vitest";

import { cachedKeys as cached, staleAfter } from "@/shared/testing/query-cache";

import { authInvalidations } from "./invalidations";

describe("authInvalidations", () => {
  const signedIn = { state: "signed_in", storefrontId: "gb", backend: "secret-service" };

  it("does nothing for the first status", () => {
    expect(authInvalidations(undefined, signedIn)).toEqual([]);
  });

  it("refreshes library, catalogue and diagnostics when signing in or out", () => {
    const stale = staleAfter(authInvalidations({ state: "signed_out" }, signedIn), cached);
    expect(stale).not.toContain("auth");
    expect(stale).not.toContain("settings");
    expect(stale).toContain("library/songs/title/false/0");
    expect(stale).toContain("catalog/lyrics/1097862703");
    expect(stale).toContain("diagnostics");
    expect(authInvalidations(signedIn, { state: "signed_out" })).toEqual(
      authInvalidations({ state: "signed_out" }, signedIn),
    );
  });

  it("ignores the intermediate sign-in states", () => {
    expect(authInvalidations({ state: "signed_out" }, { state: "signing_in" })).toEqual([]);
    expect(authInvalidations({ state: "signing_in" }, { state: "validating" })).toEqual([]);
  });

  it("refreshes only the catalogue when the storefront changes", () => {
    const stale = staleAfter(
      authInvalidations(signedIn, { ...signedIn, storefrontId: "us" }),
      cached,
    );
    expect(stale.every((k) => k.startsWith("catalog/"))).toBe(true);
    expect(stale).toHaveLength(3);
  });

  it("refreshes diagnostics when the keyring backend changes", () => {
    expect(
      staleAfter(
        authInvalidations(signedIn, { ...signedIn, backend: "file (unencrypted)" }),
        cached,
      ),
    ).toEqual(["diagnostics"]);
  });
});
