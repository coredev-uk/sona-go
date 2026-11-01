import type { AuthStatus } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { StatusBanner } from "./StatusBanner";

const env = vi.hoisted(() => ({
  status: undefined as AuthStatus | undefined,
  retry: vi.fn(() => Promise.resolve()),
}));

vi.mock("@bindings/services", () => ({ AuthService: { RetryKeyring: env.retry } }));
// The real KeyringRetry with a fake auth query. vi.mock factories run
// before the file's static imports, so the component is imported inside it.
vi.mock("@/features/auth", async () => ({
  useAuth: () => ({ data: env.status }),
  KeyringRetry: (await import("@/features/auth/KeyringRetry")).KeyringRetry,
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}));

function status(patch: Partial<AuthStatus>): AuthStatus {
  return {
    state: "signed_out",
    storefrontId: "",
    storefrontName: "",
    language: "",
    restricted: false,
    backend: "",
    unencrypted: false,
    keyring: "",
    offline: false,
    lastError: "",
    lastErrorCode: "",
    signInUrl: "",
    secondFactor: null,
    ...patch,
  };
}

function show(s: AuthStatus) {
  env.status = s;
  return render(<StatusBanner />, { wrapper: I18nRoot });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("StatusBanner and the keyring", () => {
  it("says Sona is signing in while the keyring holds the stored sign-in, with no retry nobody asked for", () => {
    show(status({ state: "restoring", keyring: "waiting" }));
    expect(screen.getByRole("status").textContent).toBe(
      "Signing in… Sona is waiting for your keyring; unlock it if it asks.",
    );
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("stays quiet during the first second of a normal start", () => {
    show(status({ state: "restoring" }));
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("offers a retry when the keyring didn't answer, and keeps focus on it while it reads again", () => {
    const view = show(
      status({
        keyring: "unavailable",
        lastError: "the keyring didn't answer",
        lastErrorCode: "keyring",
      }),
    );
    expect(screen.getByRole("status").textContent).toContain(
      "Sona couldn't read your sign-in from your keyring, so you're signed out.",
    );
    const button = screen.getByRole("button", { name: "Try again" });
    button.focus();
    fireEvent.click(button);
    expect(env.retry).toHaveBeenCalledOnce();

    env.status = status({ state: "restoring", keyring: "waiting" });
    view.rerender(<StatusBanner />);
    const trying = screen.getByRole("button", { name: "Trying…" });
    expect(trying).toBe(button);
    expect(trying.getAttribute("aria-disabled")).toBe("true");
    expect(document.activeElement).toBe(trying);
    fireEvent.click(trying);
    expect(env.retry).toHaveBeenCalledOnce();

    env.status = status({ state: "signed_in", storefrontId: "gb" });
    view.rerender(<StatusBanner />);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says a sign-in is being saved while the keyring holds the write, with nothing to retry", () => {
    show(status({ state: "validating", keyring: "waiting" }));
    expect(screen.getByRole("status").textContent).toBe(
      "Saving your sign-in in your keyring; unlock it if it asks.",
    );
    expect(screen.queryByRole("button")).toBeNull();
  });
});
