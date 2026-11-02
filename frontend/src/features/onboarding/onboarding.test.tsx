import type { Settings } from "@bindings/config";
import type { AuthStatus, Capabilities, CDMStatus } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { I18nRoot } from "@/shared/i18n";
import { openAccountDialog, useAccountDialog } from "@/shared/lib/account";
import { keys } from "@/shared/lib/keys";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { SetupPage } from "./SetupPage";
import { setupSearch, stepAfter, stepBefore, widevineView, type SetupStep } from "./steps";

const h = vi.hoisted(() => ({
  step: "welcome" as string,
  auth: "signed_out",
  settings: null as unknown,
  navigate: vi.fn(() => Promise.resolve()),
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
}));
vi.mock("@/features/auth", () => ({
  AccountSection: () => <form aria-label="Sign in to Apple Music" />,
  useAuth: () => ({ data: { state: h.auth } as AuthStatus }),
  hasAccount: (s: string | undefined) => s === "signed_in" || s === "validating",
}));
vi.mock("@/features/private", () => ({
  usePrivate: (select: (s: { state: unknown }) => unknown) =>
    select({ state: { on: false, saved: false, source: "setting", since: 0 } }),
  isForced: () => false,
  setPrivate: h.setPrivate,
}));
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  getRouteApi: () => ({ useSearch: () => ({ step: h.step }) }),
  useNavigate: () => h.navigate,
  // The router marks the link to the current location aria-current="page".
  Link: ({ children, search, ...rest }: { children: ReactNode; search?: { step?: string } }) => (
    <a
      href={`#/welcome?step=${search?.step ?? ""}`}
      aria-current={search?.step === h.step ? "page" : undefined}
      {...rest}
    >
      {children}
    </a>
  ),
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

async function show(step: SetupStep, caps: Capabilities = linuxCapabilities()) {
  h.step = step;
  capabilities.mockResolvedValue(caps);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={qc}>
      <SetupPage />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  await screen.findByRole("heading", { level: 1 });
  // Every step reads the capabilities and the settings.
  await waitFor(() => expect(qc.getQueryData(keys.capabilities)).toBeDefined());
  await waitFor(() => expect(qc.getQueryData(keys.settings)).toBeDefined());
  await act(async () => {});
  return view;
}

const rail = () => screen.getByRole("list", { name: "Set up Sona" });

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
    // Links from the earlier setup, whose steps are gone.
    expect(setupSearch.parse({ step: "listening" })).toEqual({ step: "welcome" });
    expect(setupSearch.parse({ step: "appearance" })).toEqual({ step: "appearance" });
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

describe("SetupPage", () => {
  it("marks the current step and focuses its title", async () => {
    await show("playback");
    const current = within(rail()).getByRole("link", { current: "page" });
    expect(current.textContent).toContain("Playback");
    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "Playback" }),
    );
  });

  it("moves on with Get started, and back with Back", async () => {
    await show("welcome");
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Get started" }));
    expect(h.navigate).toHaveBeenCalledWith({ to: "/welcome", search: { step: "account" } });
    cleanup();
    await show("appearance");
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(h.navigate).toHaveBeenCalledWith({ to: "/welcome", search: { step: "connections" } });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(h.navigate).toHaveBeenCalledWith({ to: "/welcome", search: { step: "desktop" } });
  });

  it("skips the whole setup: marks it done and opens For you, focus on the page", async () => {
    await show("account");
    fireEvent.click(screen.getByRole("button", { name: "Skip setup" }));
    await waitFor(() => expect(h.navigate).toHaveBeenCalledWith({ to: "/" }));
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("main")));
    expect(h.update).toHaveBeenCalledTimes(1);
    expect(h.update.mock.calls[0][0]).toMatchObject({ onboarding: { completed: true } });
  });

  it("says where each step stands in the step list, from any step", async () => {
    useCdm.setState({
      status: cdm({ state: "downloading", version: "", received: 25, total: 100 }),
    });
    h.settings = settings({ audioQuality: 64, theme: "dark" } as Partial<Settings>);
    await show("welcome");
    const link = (name: string) =>
      within(rail())
        .getAllByRole("link")
        .find((l) => l.textContent?.startsWith(name))!;
    expect(link("Apple Music").textContent).toBe("Apple Music Not signed in");
    expect(link("Playback").textContent).toBe("Playback Widevine 25% · Data saver");
    expect(link("Connections").textContent).toBe("Connections Optional");
    expect(link("Appearance").textContent).toBe("Appearance Dark theme");
  });
});

describe("Apple Music step", () => {
  it("takes a sign-in asked for elsewhere (the palette) to its own step, not a hidden dialog", async () => {
    await show("appearance");
    act(() => openAccountDialog());
    await waitFor(() =>
      expect(h.navigate).toHaveBeenCalledWith({ to: "/welcome", search: { step: "account" } }),
    );
    expect(useAccountDialog.getState().open).toBe(false);
  });

  it("offers the sign-in, and lets you carry on without it", async () => {
    await show("account");
    expect(screen.getByRole("form", { name: "Sign in to Apple Music" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Not now" }));
    expect(h.navigate).toHaveBeenCalledWith({ to: "/welcome", search: { step: "playback" } });
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
    expect(state().textContent).toBe("Downloading…");
    const bar = screen.getByRole("progressbar", { name: "Widevine download" });
    expect(bar.getAttribute("aria-valuenow")).toBe("42");
    expect(bar.getAttribute("aria-valuetext")).toBe("42%, 42 byte of 100 byte");
  });

  it("names the version once it's ready", async () => {
    await show("playback");
    expect(state().textContent).toBe("Version 4.10.3112.0 is ready");
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
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Retry" })));
    expect(h.retry).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(state());
  });

  it("says playback isn't available on a computer Google makes no Widevine for", async () => {
    useCdm.setState({
      status: cdm({ state: "failed", version: "", code: "unsupported_platform" }),
    });
    await show("playback");
    expect(state().textContent).toBe("Playback isn't available on this computer");
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("saves Sound Check and crossfade through the settings, and private listening through its switch", async () => {
    await show("playback");
    fireEvent.click(await screen.findByRole("switch", { name: "Sound Check" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ audio: { soundCheck: true } });
    fireEvent.click(screen.getByRole("switch", { name: "Crossfade" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(2));
    expect(h.update.mock.calls[1][0]).toMatchObject({ crossfade: { enabled: true, seconds: 6 } });
    fireEvent.click(screen.getByRole("switch", { name: "Private listening" }));
    expect(h.setPrivate).toHaveBeenCalledWith(true);
    expect(screen.getByRole("combobox", { name: "Audio quality" })).toBeTruthy();
  });

  it("says playback isn't available on a platform without an engine, and offers nothing else", async () => {
    useCdm.setState({ status: cdm({ state: "", version: "" }) });
    await show("playback", bareCapabilities());
    await screen.findByText("Playback isn't available on this platform yet");
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("combobox")).toBeNull();
  });
});

describe("Connections step", () => {
  it("offers Last.fm, ListenBrainz and Discord as Settings does", async () => {
    await show("connections");
    expect(await screen.findByText("Last.fm")).toBeTruthy();
    expect(screen.getByText("ListenBrainz")).toBeTruthy();
    expect(screen.getByRole("switch", { name: "Discord status" })).toBeTruthy();
    // The groups sit right under the step's h1.
    expect(screen.getByRole("heading", { level: 2, name: "Scrobbling" })).toBeTruthy();
  });
});

describe("Appearance step", () => {
  it("saves the theme picked from its pictures, and reduce effects", async () => {
    await show("appearance");
    const system = screen.getByRole("radio", { name: /^System/ }) as HTMLInputElement;
    expect(system.checked).toBe(true);
    fireEvent.click(screen.getByRole("radio", { name: /^Dark/ }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ theme: "dark" });
    expect((screen.getByRole("radio", { name: /^Dark/ }) as HTMLInputElement).checked).toBe(true);
    fireEvent.click(screen.getByRole("switch", { name: "Reduce effects" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(2));
    expect(h.update.mock.calls[1][0]).toMatchObject({ theme: "dark", reduceEffects: true });
  });
});

describe("Desktop step", () => {
  it("saves starting at login and offers what closing the window does", async () => {
    await show("desktop");
    fireEvent.click(screen.getByRole("switch", { name: "Start Sona when you log in" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ desktop: { autostart: true } });
    expect(screen.getByRole("combobox", { name: "When the window is closed" })).toBeTruthy();
    expect(screen.getByRole("switch", { name: "Track notifications" })).toBeTruthy();
  });

  it("names what the platform can't do instead of hiding it", async () => {
    await show("desktop", bareCapabilities());
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.getByText("Start Sona when you log in")).toBeTruthy();
    expect(screen.getByText("Track notifications")).toBeTruthy();
    expect(screen.getAllByText("Not available on this platform yet.")).toHaveLength(2);
    expect(screen.getByText(/Playing on in the background needs a system tray/)).toBeTruthy();
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
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Setup is done");
    const left = within(screen.getByRole("region", { name: "Still to do" }));
    expect(left.getByRole("link", { name: "Sign in" }).getAttribute("href")).toBe(
      "#/welcome?step=account",
    );
    expect(left.getByRole("link", { name: "Go to Playback" }).getAttribute("href")).toBe(
      "#/welcome?step=playback",
    );
    expect(screen.queryByRole("button", { name: "Skip setup" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(h.navigate).toHaveBeenCalledWith({ to: "/" }));
    expect(h.update).toHaveBeenCalledTimes(1);
  });

  it("says Sona is ready when nothing is left, and doesn't save again", async () => {
    h.auth = "signed_in";
    h.settings = settings({ onboarding: { completed: true } } as Partial<Settings>);
    await show("done");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("You're all set");
    expect(screen.getByText("Ready to play")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Still to do" })).toBeNull();
    await act(async () => {});
    expect(h.update).not.toHaveBeenCalled();
  });
});
