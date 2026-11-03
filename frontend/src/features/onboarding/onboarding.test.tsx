import type { Settings } from "@bindings/config";
import type { AuthStatus, Capabilities, CDMStatus } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { isRedirect } from "@tanstack/react-router";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { ConfirmHost } from "@/shared/components/ConfirmDialog";
import { I18nRoot } from "@/shared/i18n";
import { openAccountDialog, useAccountDialog } from "@/shared/lib/account";
import { openSetup, useSetupOpen } from "@/shared/lib/setup";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { onboardingModule } from ".";
import { SetupLayer } from "./SetupLayer";
import {
  setupSearch,
  showSetup,
  stepAfter,
  stepBefore,
  useSetupStep,
  widevineView,
  type SetupStep,
} from "./steps";

const h = vi.hoisted(() => ({
  auth: "signed_out",
  settings: null as unknown,
  update: vi.fn((next: unknown) => Promise.resolve(next)),
  retry: vi.fn(() => Promise.resolve()),
  setPrivate: vi.fn(),
}));
const capabilities = vi.fn<() => Promise<Capabilities>>();

const scrobbler = {
  available: true,
  connected: false,
  connecting: false,
  needsReconnect: false,
  pending: 0,
};

vi.mock("@bindings/services", () => ({
  SystemService: {
    Settings: () => Promise.resolve(h.settings),
    UpdateSettings: (next: unknown) => h.update(next),
  },
  PlatformService: { Capabilities: () => capabilities() },
  CDMService: { Retry: h.retry, Status: () => new Promise(() => {}) },
  ScrobbleService: {
    Status: () =>
      Promise.resolve({ lastfm: scrobbler, listenbrainz: scrobbler, unencrypted: false }),
  },
  IntegrationsService: {
    Status: () =>
      Promise.resolve({
        notifications: "",
        inhibit: "",
        discord: { configured: true, connected: false },
      }),
  },
  TranslationsService: { Languages: () => Promise.resolve({ mapped: 0.07, locales: [] }) },
}));
vi.mock("@/features/auth", async () => {
  const { createPortal } = await import("react-dom");
  return {
    // As the real one: with `actions`, the form's Sign in goes there.
    AccountSection: ({ actions }: { actions?: HTMLElement | null }) => (
      <form id="sign-in" aria-label="Sign in to Apple Music">
        {actions &&
          createPortal(
            <button type="submit" form="sign-in">
              Sign in
            </button>,
            actions,
          )}
      </form>
    ),
    useAuth: () => ({ data: { state: h.auth } as AuthStatus }),
    hasAccount: (s: string | undefined) => s === "signed_in" || s === "validating",
  };
});
vi.mock("@/features/private", () => ({
  usePrivate: (select: (s: { state: unknown }) => unknown) =>
    select({ state: { on: false, saved: false, source: "setting", since: 0 } }),
  isForced: () => false,
  setPrivate: h.setPrivate,
}));

function settings(patch: Partial<Settings> = {}): Settings {
  return {
    theme: "system",
    reduceEffects: false,
    audioQuality: 256,
    audio: { outputDevice: "", soundCheck: false, equalizer: { enabled: false } },
    crossfade: { enabled: false, seconds: 6 },
    motionArtwork: { enabled: true },
    private: { enabled: false },
    desktop: { autostart: false, startHidden: false, closeAction: "background" },
    integrations: { notifications: true, inhibitSleep: false, discordPresence: false },
    scrobbling: {
      lastfm: { enabled: false, username: "" },
      listenbrainz: { enabled: false, username: "" },
    },
    onboarding: { completed: false },
    ...patch,
  } as unknown as Settings;
}

function cdm(patch: Partial<CDMStatus> = {}): CDMStatus {
  return {
    state: "ready",
    version: "4.10.3112.0",
    path: "",
    code: "",
    received: 0,
    total: 0,
    updating: false,
    checkedAt: 0,
    ...patch,
  };
}

/** The setup open at `step`, over nothing (the app behind isn't the setup's). */
async function show(step: SetupStep, caps: Capabilities = linuxCapabilities()) {
  capabilities.mockResolvedValue(caps);
  showSetup(step);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={qc}>
      <main id="content" tabIndex={-1} />
      <ConfirmHost />
      <SetupLayer />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  await screen.findByRole("dialog");
  // Whatever the step reads (capabilities, settings, statuses) has landed.
  await waitFor(() => expect(qc.isFetching()).toBe(0));
  await act(async () => {});
  return view;
}

const setup = () => screen.getByRole("dialog");
const title = () => within(setup()).getByRole("heading", { level: 2 });
const strip = () => within(setup()).getByRole("navigation", { name: "Setup steps" });
/** The way out at the foot (the strip has a "Done" too: the step). */
const finish = () =>
  within(setup())
    .getAllByRole("button", { name: "Done" })
    .find((b) => !b.closest("nav"))!;

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

beforeEach(() => {
  h.auth = "signed_out";
  h.settings = settings();
  useCdm.setState({ status: cdm() });
});

afterEach(() => {
  cleanup();
  useSetupOpen.setState({ open: false });
  useAccountDialog.setState({ open: false });
  vi.clearAllMocks();
});

describe("steps", () => {
  it("goes through every step in order and stays on the last", () => {
    const seen: SetupStep[] = ["welcome"];
    for (let i = 0; i < 8; i++) seen.push(stepAfter(seen.at(-1)!));
    expect(seen).toEqual([
      "welcome",
      "account",
      "playback",
      "connections",
      "appearance",
      "desktop",
      "done",
      "done",
      "done",
    ]);
    expect(stepBefore("account")).toBe("welcome");
    expect(stepBefore("welcome")).toBe("welcome");
  });

  it("starts at the welcome for a missing or unknown step", () => {
    expect(setupSearch.parse({})).toEqual({ step: "welcome" });
    // Links from an earlier setup, whose steps are gone.
    expect(setupSearch.parse({ step: "listening" })).toEqual({ step: "welcome" });
    expect(setupSearch.parse({ step: "appearance" })).toEqual({ step: "appearance" });
  });

  it("opens the setup over For you from a /welcome link, at its step", () => {
    const route = onboardingModule.routes[0];
    let thrown: unknown;
    try {
      // @ts-expect-error -- only the search is read
      route.options.beforeLoad?.({ search: { step: "desktop" } });
    } catch (err) {
      thrown = err;
    }
    if (!isRedirect(thrown)) throw new Error("expected a redirect");
    expect(thrown.options.to).toBe("/");
    expect(useSetupOpen.getState().open).toBe(true);
    expect(useSetupStep.getState().step).toBe("desktop");
  });

  it("reads Widevine from the capabilities first, then the download", () => {
    const linux = linuxCapabilities();
    expect(widevineView(undefined, cdm())).toBe("loading");
    expect(widevineView(bareCapabilities(), null)).toBe("unavailable");
    expect(widevineView(bareCapabilities("windows"), cdm({ state: "" }))).toBe("unavailable");
    expect(
      widevineView({ ...linux, playback: { ...linux.playback, engine: "native" } }, null),
    ).toBe("not-needed");
    expect(widevineView(linux, null)).toBe("loading");
    expect(widevineView(linux, cdm({ state: "idle", version: "" }))).toBe("waiting");
    expect(widevineView(linux, cdm({ state: "downloading" }))).toBe("downloading");
    expect(widevineView(linux, cdm())).toBe("ready");
    expect(widevineView(linux, cdm({ state: "failed", code: "network" }))).toBe("failed");
    // Linux on arm64: the CDM engine, but Google makes no CDM for it.
    expect(widevineView(linux, cdm({ state: "failed", code: "unsupported_platform" }))).toBe(
      "unsupported",
    );
  });
});

describe("SetupLayer", () => {
  it("is a modal named by its step, marks the step in the strip and focuses its title", async () => {
    await show("playback");
    expect(setup().getAttribute("aria-describedby")).toBeTruthy();
    expect(title().textContent).toBe("Playback");
    const current = within(strip()).getByRole("button", { current: "step" });
    expect(current.textContent).toBe("Playback");
    // Every step's name shows under its segment, not only to screen readers.
    expect(strip().querySelector(".sr-only")).toBeNull();
    expect(within(strip()).getByText("Connections").className).not.toContain("sr-only");
    expect(within(setup()).getByText("3 of 7")).toBeTruthy();
    expect(document.activeElement).toBe(title());
  });

  it("moves on with Get started, and back with Back", async () => {
    await show("welcome");
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Get started" }));
    expect(title().textContent).toBe("Apple Music");
    expect(document.activeElement).toBe(title());
    act(() => useSetupStep.setState({ step: "appearance" }));
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(title().textContent).toBe("Connections");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(title().textContent).toBe("Appearance");
  });

  it("goes to any step from the strip, one Tab stop moved with the arrow keys", async () => {
    await show("account");
    const buttons = within(strip()).getAllByRole("button");
    expect(buttons.map((b) => b.tabIndex)).toEqual([-1, 0, -1, -1, -1, -1, -1]);
    buttons[1].focus();
    fireEvent.keyDown(buttons[1], { key: "ArrowRight" });
    expect(document.activeElement).toBe(buttons[2]);
    fireEvent.keyDown(buttons[2], { key: "End" });
    expect(document.activeElement).toBe(buttons[6]);
    fireEvent.click(within(strip()).getByRole("button", { name: "Desktop" }));
    expect(title().textContent).toBe("Desktop");
  });

  it("ends with Skip setup at once: marked done once, closed", async () => {
    await show("account");
    fireEvent.click(screen.getByRole("button", { name: "Skip setup" }));
    await waitFor(() => expect(useSetupOpen.getState().open).toBe(false));
    expect(h.update).toHaveBeenCalledTimes(1);
    expect(h.update.mock.calls[0][0]).toMatchObject({ onboarding: { completed: true } });
  });

  it("asks before Escape skips the setup", async () => {
    await show("playback");
    fireEvent.keyDown(setup(), { key: "Escape" });
    const ask = await screen.findByRole("alertdialog", { name: "Skip the rest of the setup?" });
    fireEvent.click(within(ask).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(useSetupOpen.getState().open).toBe(true);
    expect(h.update).not.toHaveBeenCalled();

    fireEvent.keyDown(setup(), { key: "Escape" });
    const again = await screen.findByRole("alertdialog");
    fireEvent.click(within(again).getByRole("button", { name: "Skip setup" }));
    await waitFor(() => expect(useSetupOpen.getState().open).toBe(false));
    expect(h.update.mock.calls[0][0]).toMatchObject({ onboarding: { completed: true } });
  });

  it("starts again at the welcome once closed, with focus back on the page", async () => {
    await show("done");
    fireEvent.click(finish());
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("main")));
    act(() => openSetup());
    expect((await screen.findByRole("heading", { level: 2 })).textContent).toBe("Welcome to Sona");
  });
});

describe("Welcome step", () => {
  it("says what whole songs need and where each stands", async () => {
    useCdm.setState({
      status: cdm({ state: "downloading", version: "", received: 25, total: 100 }),
    });
    await show("welcome");
    const needs = within(screen.getByRole("region", { name: "What whole songs need" }));
    expect(needs.getByText("Next step")).toBeTruthy();
    expect(needs.getByText("Downloading, 25%")).toBeTruthy();
    // Only English is complete enough to offer.
    expect(screen.queryByRole("combobox", { name: "Language" })).toBeNull();
  });
});

describe("Apple Music step", () => {
  it("takes a sign-in asked for elsewhere (the palette) to its own step, not the dialog behind", async () => {
    await show("appearance");
    act(() => openAccountDialog());
    await waitFor(() => expect(title().textContent).toBe("Apple Music"));
    expect(useAccountDialog.getState().open).toBe(false);
  });

  it("offers the sign-in, and lets you carry on without it", async () => {
    await show("account");
    const form = screen.getByRole("form", { name: "Sign in to Apple Music" });
    // Sign in is the foot's primary action, beside "Not now", outside the scrolling body.
    const signIn = screen.getByRole("button", { name: "Sign in" });
    expect(form.contains(signIn)).toBe(false);
    expect(signIn.parentElement?.previousElementSibling?.textContent).toBe("Not now");
    fireEvent.click(screen.getByRole("button", { name: "Not now" }));
    expect(title().textContent).toBe("Playback");
  });

  it("says you're signed in instead of the form", async () => {
    h.auth = "signed_in";
    await show("account");
    expect(screen.queryByRole("form")).toBeNull();
    expect(screen.getByText("Signed in to Apple Music")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Continue" })).toBeTruthy();
  });
});

describe("Playback step", () => {
  const state = () => screen.getByRole("status");

  it("shows the download's progress on its bar", async () => {
    useCdm.setState({
      status: cdm({ state: "downloading", version: "", received: 42, total: 100 }),
    });
    await show("playback");
    expect(state().textContent).toBe("Downloading Widevine…");
    const bar = screen.getByRole("progressbar", { name: "Widevine download" });
    expect(bar.getAttribute("aria-valuenow")).toBe("42");
    expect(bar.getAttribute("aria-valuetext")).toBe("42%, 42 byte of 100 byte");
  });

  it("names the version once it's ready", async () => {
    await show("playback");
    expect(state().textContent).toBe("Widevine 4.10.3112.0 is ready");
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("says why the download failed, retries it and keeps focus on the state", async () => {
    useCdm.setState({ status: cdm({ state: "failed", version: "", code: "network" }) });
    await show("playback");
    expect(state().textContent).toBe("Couldn't download");
    expect(
      screen.getByText(
        "Sona couldn't reach Google's download server. Check your internet connection. Sona also tries again by itself later.",
      ),
    ).toBeTruthy();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Try again" })));
    expect(h.retry).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(state());
  });

  it("says playback isn't available on a computer Google makes no Widevine for", async () => {
    useCdm.setState({
      status: cdm({ state: "failed", version: "", code: "unsupported_platform" }),
    });
    await show("playback");
    expect(state().textContent).toBe("Playback isn't available on this computer");
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("saves the quality from its cards, Sound Check and crossfade, and private listening", async () => {
    await show("playback");
    const high = screen.getByRole("radio", { name: "High quality" }) as HTMLInputElement;
    expect(high.checked).toBe(true);
    fireEvent.click(screen.getByRole("radio", { name: "Data saver" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ audioQuality: 64 });
    fireEvent.click(screen.getByRole("switch", { name: "Sound Check" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(2));
    expect(h.update.mock.calls[1][0]).toMatchObject({ audio: { soundCheck: true } });
    expect(screen.queryByRole("slider")).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "Crossfade" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(3));
    expect(h.update.mock.calls[2][0]).toMatchObject({ crossfade: { enabled: true, seconds: 6 } });
    expect(await screen.findByRole("slider", { name: "Length" })).toBeTruthy();
    fireEvent.click(screen.getByRole("switch", { name: "Private listening" }));
    expect(h.setPrivate).toHaveBeenCalledWith(true);
  });

  it("says playback isn't available on a platform without an engine, and offers nothing else", async () => {
    useCdm.setState({ status: cdm({ state: "", version: "" }) });
    await show("playback", bareCapabilities());
    await screen.findByText("Playback isn't available on this platform yet");
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("radio")).toBeNull();
  });
});

describe("Connections step", () => {
  it("offers Last.fm, ListenBrainz (its token form) and Discord", async () => {
    await show("connections");
    expect(await screen.findByRole("button", { name: "Connect Last.fm" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Connect ListenBrainz" }));
    expect(await screen.findByLabelText("User token")).toBeTruthy();
    expect(screen.getByRole("switch", { name: "Discord status" })).toBeTruthy();
  });
});

describe("Appearance step", () => {
  it("saves the theme picked from its pictures, and reduce effects, which turns motion off", async () => {
    await show("appearance");
    const system = screen.getByRole("radio", { name: "System" }) as HTMLInputElement;
    expect(system.checked).toBe(true);
    fireEvent.click(screen.getByRole("radio", { name: "Dark" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ theme: "dark" });
    expect((screen.getByRole("radio", { name: "Dark" }) as HTMLInputElement).checked).toBe(true);
    fireEvent.click(screen.getByRole("switch", { name: "Reduce effects" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(2));
    expect(h.update.mock.calls[1][0]).toMatchObject({ theme: "dark", reduceEffects: true });
    const motion = screen.getByRole("switch", { name: "Motion artwork" });
    await waitFor(() => expect(motion.getAttribute("aria-checked")).toBe("false"));
    expect((motion as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("Desktop step", () => {
  it("saves starting at login and what closing the window does", async () => {
    await show("desktop");
    fireEvent.click(screen.getByRole("switch", { name: "Start Sona when you log in" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ desktop: { autostart: true } });
    fireEvent.click(screen.getByRole("radio", { name: "Quit Sona" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(2));
    expect(h.update.mock.calls[1][0]).toMatchObject({
      desktop: { autostart: true, closeAction: "quit" },
    });
    expect(screen.getByRole("switch", { name: "Track notifications" })).toBeTruthy();
  });

  it("names what the platform can't do instead of hiding it", async () => {
    await show("desktop", bareCapabilities());
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("radio")).toBeNull();
    expect(
      screen.getByText(/Not on this platform yet: starting at login and track notifications\./),
    ).toBeTruthy();
    expect(screen.getByText(/playing on in the background needs a system tray/)).toBeTruthy();
  });
});

describe("Done step", () => {
  it("marks the setup done once and lists what is still to do", async () => {
    useCdm.setState({
      status: cdm({ state: "downloading", version: "", received: 50, total: 100 }),
    });
    await show("done");
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ onboarding: { completed: true } });
    expect(title().textContent).toBe("Setup is done");
    expect(screen.queryByRole("button", { name: "Skip setup" })).toBeNull();
    const left = within(screen.getByRole("region", { name: "Still to do" }));
    fireEvent.click(left.getByRole("button", { name: "Go to Playback" }));
    expect(title().textContent).toBe("Playback");
    act(() => useSetupStep.setState({ step: "done" }));
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(title().textContent).toBe("Apple Music");
    act(() => useSetupStep.setState({ step: "done" }));
    fireEvent.click(finish());
    await waitFor(() => expect(useSetupOpen.getState().open).toBe(false));
    expect(h.update).toHaveBeenCalledTimes(1);
  });

  it("says Sona is ready when nothing is left, and doesn't save again", async () => {
    h.auth = "signed_in";
    h.settings = settings({ onboarding: { completed: true } } as Partial<Settings>);
    await show("done");
    expect(title().textContent).toBe("You're all set");
    expect(screen.getByText("Ready to play")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Still to do" })).toBeNull();
    await act(async () => {});
    expect(h.update).not.toHaveBeenCalled();
  });

  it("closes on Escape without asking: nothing is left to skip", async () => {
    await show("done");
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() => expect(useSetupOpen.getState().open).toBe(false));
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});
