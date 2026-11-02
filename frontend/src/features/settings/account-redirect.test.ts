import { isRedirect } from "@tanstack/react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { closeAccountDialog, useAccountDialog } from "@/shared/lib/account";

import { settingsModule } from "./index";

vi.mock("./Settings", () => ({ SettingsPage: () => null }));

afterEach(() => closeAccountDialog());

/** Runs the Settings route's beforeLoad for a link with this search, as the router would. */
function load(search: Record<string, unknown>): unknown {
  const beforeLoad = settingsModule.routes[0].options.beforeLoad as
    ((ctx: { location: { search: unknown } }) => unknown) | undefined;
  try {
    return beforeLoad?.({ location: { search } });
  } catch (thrown) {
    return thrown;
  }
}

describe("links to the retired Settings → Account", () => {
  it("open the account dialog and land on Settings without a section", () => {
    const result = load({ section: "account" });
    expect(isRedirect(result)).toBe(true);
    expect(useAccountDialog.getState().open).toBe(true);
  });

  it("leave every other section alone", () => {
    expect(load({ section: "general" })).toBeUndefined();
    expect(load({})).toBeUndefined();
    expect(useAccountDialog.getState().open).toBe(false);
  });
});
