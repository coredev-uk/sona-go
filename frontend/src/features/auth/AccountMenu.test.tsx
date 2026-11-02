import type { AuthStatus, PrivateState } from "@bindings/services";
import { ErrorCode } from "@bindings/services/models";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { usePrivate } from "@/features/private";
import { I18nRoot } from "@/shared/i18n";
import { closeAccountDialog, openAccountDialog } from "@/shared/lib/account";
import { keys } from "@/shared/lib/keys";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { AccountMenu } from "./AccountMenu";

const go = vi.hoisted(() => ({
  Status: vi.fn(),
  Revalidate: vi.fn(() => Promise.resolve()),
  RetryKeyring: vi.fn(() => Promise.resolve()),
  SignOut: vi.fn(() => Promise.resolve()),
  CancelSignIn: vi.fn(() => Promise.resolve()),
  SetPrivate: vi.fn((on: boolean) =>
    Promise.resolve({ on, source: "setting", saved: on } as PrivateState),
  ),
}));

vi.mock("@bindings/services", () => ({
  AuthService: go,
  PrivateService: { Set: go.SetPrivate },
}));
vi.mock("@wailsio/runtime", () => ({
  Browser: { OpenURL: vi.fn() },
  Clipboard: { SetText: vi.fn() },
}));

const status = (over: Partial<AuthStatus>): AuthStatus =>
  ({
    state: "signed_out",
    storefrontId: "",
    storefrontName: "",
    language: "",
    restricted: false,
    backend: "",
    unencrypted: false,
    keyring: "",
    offline: false,
    lastErrorCode: ErrorCode.$zero,
    browserSignIn: false,
    ...over,
  }) as AuthStatus;

const signedIn = status({
  state: "signed_in",
  storefrontId: "gb",
  storefrontName: "United Kingdom",
  language: "en-GB",
  backend: "secret-service",
});

beforeAll(() => {
  // Radix's popper measures with ResizeObserver, which jsdom lacks.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  closeAccountDialog();
  usePrivate.setState({ state: null });
});

function renderMenu(
  s: AuthStatus,
  priv: PrivateState = { on: false, source: "setting", saved: false },
) {
  const qc = new QueryClient();
  qc.setQueryData(keys.auth, s);
  go.Status.mockResolvedValue(s);
  usePrivate.setState({ state: priv });
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <AccountMenu />
      </TooltipProvider>
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  return screen.getByRole("button", { name: "Account" });
}

function openMenu(trigger: HTMLElement) {
  trigger.focus();
  fireEvent.keyDown(trigger, { key: "Enter" });
  return screen.getByRole("menu");
}

describe("the account menu", () => {
  it("says where a signed-out account stands and opens the sign-in from the menu", async () => {
    const trigger = renderMenu(status({}));
    expect(trigger.getAttribute("aria-haspopup")).toBe("menu");
    expect(trigger.getAttribute("aria-describedby")).toBeTruthy();
    const menu = openMenu(trigger);
    expect(within(menu).getByText("Not signed in")).toBeTruthy();
    expect(within(menu).queryByRole("menuitem", { name: "Sign out" })).toBeNull();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Sign in" }));
    const dialog = await screen.findByRole("dialog", { name: "Account" });
    expect(within(dialog).getByRole("heading", { name: "Sign in to Apple Music" })).toBeTruthy();
    const field = within(dialog).getByLabelText("Apple Account");
    await waitFor(() => expect(document.activeElement).toBe(field));
  });

  it("shows the storefront, restricted content, offline and an unencrypted token", () => {
    const trigger = renderMenu({ ...signedIn, restricted: true, offline: true, unencrypted: true });
    const menu = openMenu(trigger);
    expect(within(menu).getByText("Signed in · United Kingdom")).toBeTruthy();
    expect(
      within(menu).getByText("Explicit content is restricted on this account; Sona hides it."),
    ).toBeTruthy();
    expect(within(menu).getByText("Your token is stored unencrypted")).toBeTruthy();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Check now" }));
    expect(go.Revalidate).toHaveBeenCalledOnce();
  });

  it("opens the account's settings in the dialog", async () => {
    openMenu(renderMenu(signedIn));
    fireEvent.click(screen.getByRole("menuitem", { name: "Account settings" }));
    const dialog = await screen.findByRole("dialog", { name: "Account" });
    expect(within(dialog).getByText("Signed in to Apple Music")).toBeTruthy();
    // The account's language as the interface names it, not its tag.
    expect(within(dialog).getByText(/British English/)).toBeTruthy();
  });

  it("asks before signing out, then signs out", async () => {
    openMenu(renderMenu(signedIn));
    fireEvent.click(screen.getByRole("menuitem", { name: "Sign out" }));
    const confirm = await screen.findByRole("dialog", { name: "Sign out of Apple Music?" });
    expect(go.SignOut).not.toHaveBeenCalled();
    fireEvent.click(within(confirm).getByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(go.SignOut).toHaveBeenCalledOnce());
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("offers the keyring's retry when it didn't hand the sign-in over", () => {
    const trigger = renderMenu(status({ keyring: "unavailable" }));
    const menu = openMenu(trigger);
    expect(
      within(menu).getByText("Sona couldn't read your sign-in from your keyring"),
    ).toBeTruthy();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Try again" }));
    expect(go.RetryKeyring).toHaveBeenCalledOnce();
  });

  it("switches private listening and stays open", async () => {
    const menu = openMenu(renderMenu(signedIn));
    const item = within(menu).getByRole("menuitemcheckbox", { name: "Private listening" });
    expect(item.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(item);
    expect(go.SetPrivate).toHaveBeenCalledWith(true);
    await waitFor(() => expect(item.getAttribute("aria-checked")).toBe("true"));
    expect(screen.getByRole("menu")).toBe(menu);
  });

  it("can't switch private listening off while SONA_PRIVATE=1 forces it, and says why", () => {
    const menu = openMenu(renderMenu(signedIn, { on: true, source: "environment", saved: false }));
    const item = within(menu).getByRole("menuitemcheckbox", { name: "Private listening" });
    expect(item.getAttribute("aria-disabled")).toBe("true");
    expect(item.getAttribute("aria-describedby")).toBeTruthy();
    expect(document.getElementById(item.getAttribute("aria-describedby")!)?.textContent).toBe(
      "On because Sona was started with SONA_PRIVATE=1",
    );
    fireEvent.click(item);
    expect(go.SetPrivate).not.toHaveBeenCalled();
  });

  it("closes on Escape and gives focus back to the avatar", async () => {
    const trigger = renderMenu(signedIn);
    const menu = openMenu(trigger);
    fireEvent.keyDown(menu, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(document.activeElement).toBe(trigger);
  });

  it("opens the account dialog for every other Sign in action", async () => {
    renderMenu(status({ state: "needs_reauth" }));
    act(() => openAccountDialog());
    const dialog = await screen.findByRole("dialog", { name: "Account" });
    expect(within(dialog).getByText("Apple stopped accepting Sona's sign-in")).toBeTruthy();
  });
});
