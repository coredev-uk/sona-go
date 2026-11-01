import type { Settings } from "@bindings/config";
import type { AuthStatus, Capabilities, CDMStatus } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { I18nRoot } from "@/shared/i18n";
import { keys } from "@/shared/lib/keys";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { SetupPage } from "./SetupPage";
import { setupSearch, stepAfter, widevineView, type SetupStep } from "./steps";

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

vi.mock("@bindings/services", () => ({
  SystemService: {
    Settings: () => Promise.resolve(h.settings),
    UpdateSettings: (next: unknown) => h.update(next),
  },
  PlatformService: { Capabilities: () => capabilities() },
  CDMService: { Retry: h.retry, Status: () => new Promise(() => {}) },
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
    audio: { outputDevice: "", soundCheck: false, equalizer: { enabled: false } },
    crossfade: { enabled: false, seconds: 6 },
    private: { enabled: false },
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
      {/* The shell's page, where focus goes when the setup is left. */}
      <main id="content" tabIndex={-1}>
        <SetupPage />
      </main>
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  await screen.findByRole("heading", { level: 1 });
  // Every step reads the capabilities.
  await waitFor(() => expect(qc.getQueryData(keys.capabilities)).toBeDefined());
  await act(async () => {});
  return view;
}

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
    for (let i = 0; i < 6; i++) seen.push(stepAfter(seen.at(-1)!));
    expect(seen).toEqual(["welcome", "account", "widevine", "listening", "done", "done", "done"]);
  });

  it("starts at the welcome for a missing or unknown step", () => {
    expect(setupSearch.parse({})).toEqual({ step: "welcome" });
    expect(setupSearch.parse({ step: "billing" })).toEqual({ step: "welcome" });
    expect(setupSearch.parse({ step: "listening" })).toEqual({ step: "listening" });
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
    await show("widevine");
    const current = screen.getByRole("link", { current: "page" });
    expect(current.textContent).toContain("Widevine");
    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "Widevine" }),
    );
  });

  it("moves on to the next step", async () => {
    await show("welcome");
    fireEvent.click(screen.getByRole("button", { name: "Get started" }));
    expect(h.navigate).toHaveBeenCalledWith({ to: "/welcome", search: { step: "account" } });
  });

  it("skips the whole setup: marks it done and opens For you, focus on the page", async () => {
    await show("account");
    fireEvent.click(screen.getByRole("button", { name: "Skip setup" }));
    await waitFor(() => expect(h.navigate).toHaveBeenCalledWith({ to: "/" }));
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("main")));
    expect(h.update).toHaveBeenCalledTimes(1);
    expect(h.update.mock.calls[0][0]).toMatchObject({ onboarding: { completed: true } });
  });

  it("shows the progress of a running download in the step list from any step", async () => {
    useCdm.setState({
      status: cdm({ state: "downloading", version: "", received: 25, total: 100 }),
    });
    await show("listening");
    const link = screen.getByRole("link", { name: "Widevine Downloading, 25%" });
    expect(link.textContent).toBe("Widevine 25%Downloading, 25%");
  });
});

describe("Apple Music step", () => {
  it("offers the sign-in, and lets you carry on without it", async () => {
    await show("account");
    expect(screen.getByRole("form", { name: "Sign in to Apple Music" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Not now" }));
    expect(h.navigate).toHaveBeenCalledWith({ to: "/welcome", search: { step: "widevine" } });
  });

  it("says you're signed in instead of the form", async () => {
    h.auth = "signed_in";
    await show("account");
    expect(screen.queryByRole("form")).toBeNull();
    expect(screen.getByText("Signed in to Apple Music")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Continue" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Apple Music Signed in" })).toBeTruthy();
  });
});

describe("Widevine step", () => {
  const state = () => screen.getByRole("status");

  it("shows the download's progress on its bar", async () => {
    useCdm.setState({
      status: cdm({ state: "downloading", version: "", received: 42, total: 100 }),
    });
    await show("widevine");
    expect(state().textContent).toBe("Downloading…");
    const bar = screen.getByRole("progressbar", { name: "Widevine download" });
    expect(bar.getAttribute("aria-valuenow")).toBe("42");
    expect(bar.getAttribute("aria-valuetext")).toBe("42%, 42 byte of 100 byte");
  });

  it("names the version once it's ready", async () => {
    await show("widevine");
    expect(state().textContent).toBe("Version 4.10.3112.0 is ready");
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("says why the download failed, retries it and keeps focus on the state", async () => {
    useCdm.setState({ status: cdm({ state: "failed", version: "", code: "network" }) });
    await show("widevine");
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
    await show("widevine");
    expect(state().textContent).toBe("Playback isn't available on this computer");
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("says playback isn't available on a platform without an engine", async () => {
    useCdm.setState({ status: cdm({ state: "", version: "" }) });
    await show("widevine", bareCapabilities());
    await screen.findByText("Playback isn't available on this platform yet");
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });
});

describe("Listening step", () => {
  it("saves Sound Check and crossfade through the settings, and private listening through its switch", async () => {
    await show("listening");
    fireEvent.click(await screen.findByRole("switch", { name: "Sound Check" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ audio: { soundCheck: true } });
    fireEvent.click(screen.getByRole("switch", { name: "Crossfade" }));
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(2));
    expect(h.update.mock.calls[1][0]).toMatchObject({ crossfade: { enabled: true, seconds: 6 } });
    fireEvent.click(screen.getByRole("switch", { name: "Private listening" }));
    expect(h.setPrivate).toHaveBeenCalledWith(true);
  });

  it("leaves out what the platform's playback can't do", async () => {
    await show("listening", bareCapabilities());
    await screen.findByRole("switch", { name: "Private listening" });
    expect(screen.queryByRole("switch", { name: "Sound Check" })).toBeNull();
    expect(screen.queryByRole("switch", { name: "Crossfade" })).toBeNull();
  });
});

describe("Done step", () => {
  it("marks the setup done once and sums up the choices", async () => {
    h.settings = settings({ crossfade: { enabled: true, seconds: 8 } } as Partial<Settings>);
    await show("done");
    await waitFor(() => expect(h.update).toHaveBeenCalledTimes(1));
    expect(h.update.mock.calls[0][0]).toMatchObject({ onboarding: { completed: true } });
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Setup is done");
    expect(await screen.findByText("On, 8 seconds")).toBeTruthy();
    expect(screen.getByText("Not signed in")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Sign in to Apple Music" }).getAttribute("href")).toBe(
      "#/welcome?step=account",
    );
    expect(screen.queryByRole("button", { name: "Skip setup" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(h.navigate).toHaveBeenCalledWith({ to: "/" }));
    expect(h.update).toHaveBeenCalledTimes(1);
  });

  it("doesn't save again when the setup was already done", async () => {
    h.auth = "signed_in";
    h.settings = settings({ onboarding: { completed: true } } as Partial<Settings>);
    await show("done");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("You're all set");
    expect((await screen.findAllByText("Signed in")).length).toBe(2);
    await act(async () => {});
    expect(h.update).not.toHaveBeenCalled();
  });
});
