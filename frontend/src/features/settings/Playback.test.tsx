import type { Settings } from "@bindings/config";
import type { Capabilities, CDMStatus } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { I18nRoot } from "@/shared/i18n";
import { keys } from "@/shared/lib/keys";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { PlaybackSection } from "./Playback";

const capabilities = vi.fn<() => Promise<Capabilities>>();
const retry = vi.hoisted(() => vi.fn(() => Promise.resolve()));

const settings = {
  engineIdleMinutes: 10,
  audioQuality: 256,
  audio: { outputDevice: "", soundCheck: false, equalizer: { enabled: false } },
  crossfade: { enabled: false, seconds: 6 },
  queue: { replaceConfirm: "undo" },
} as unknown as Settings;

vi.mock("@bindings/services", () => ({
  SystemService: { Settings: () => Promise.resolve(settings), UpdateSettings: vi.fn() },
  PlatformService: { Capabilities: () => capabilities() },
  AudioService: { OutputDevices: () => Promise.resolve([]) },
  CDMService: { Retry: retry, Status: () => new Promise(() => {}) },
}));
vi.mock("@/features/private", () => ({
  usePrivate: (select: (s: { state: null }) => unknown) => select({ state: null }),
  isForced: () => false,
  setPrivate: vi.fn(),
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}));

/** The rows and buttons that need playback, or one of its capabilities. */
const gated = [
  "Audio quality",
  "Free memory when idle",
  "Output",
  "Sound Check",
  "Equaliser",
  "Crossfade",
  "Widevine",
  "Private listening",
  "Replacing your queue",
];

const shown = (text: string) => screen.queryByText(text) !== null;

/** Renders the section and waits for the settings (not the capabilities) to load. */
async function renderSection(caps: Promise<Capabilities>) {
  capabilities.mockReturnValue(caps);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <PlaybackSection />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  await waitFor(() => expect(qc.getQueryData(keys.settings)).toBeDefined());
}

beforeEach(() => {
  useCdm.setState({ status: cdm() });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function cdm(patch: Partial<CDMStatus> = {}): CDMStatus {
  return {
    state: "ready",
    version: "4.10.3112.0",
    path: "/data/widevine/4.10.3112.0/linux_x64/libwidevinecdm.so",
    code: "",
    received: 0,
    total: 0,
    updating: false,
    checkedAt: Date.now() - 2 * 3600_000,
    ...patch,
  };
}

describe("PlaybackSection", () => {
  it("groups every playback control on Linux, without the unavailable note", async () => {
    await renderSection(Promise.resolve(linuxCapabilities()));
    await screen.findByText("Sound Check");
    expect(gated.filter((text) => !shown(text))).toEqual([]);
    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual([
      "Sound",
      "Listening",
      "Advanced",
    ]);
    expect(shown("Playback isn't available on this platform yet")).toBe(false);
  });

  it("has no volume: the player bar, MPRIS and the CLI set it", async () => {
    await renderSection(Promise.resolve(linuxCapabilities()));
    await screen.findByText("Sound Check");
    expect(screen.queryByRole("slider", { name: /volume/i })).toBeNull();
    expect(shown("Volume")).toBe(false);
  });

  it("says playback isn't available and shows nothing else without an engine", async () => {
    await renderSection(Promise.resolve(bareCapabilities()));
    await screen.findByText("Playback isn't available on this platform yet");
    expect(gated.filter(shown)).toEqual([]);
    expect(screen.queryAllByRole("heading")).toEqual([]);
  });

  it("leaves out private listening where the engine reports plays itself", async () => {
    const caps = linuxCapabilities();
    caps.playback.privateListening = false;
    await renderSection(Promise.resolve(caps));
    await screen.findByText("Replacing your queue");
    expect(shown("Private listening")).toBe(false);
  });

  it("shows no gated control while the capabilities load", async () => {
    await renderSection(new Promise(() => {}));
    expect(gated.filter(shown)).toEqual([]);
    expect(shown("Playback isn't available on this platform yet")).toBe(false);
  });

  it("has no CDM file chooser", async () => {
    await renderSection(Promise.resolve(linuxCapabilities()));
    await screen.findByText("Widevine");
    expect(shown("Choose…")).toBe(false);
    expect(shown("Widevine CDM location")).toBe(false);
    expect(shown("Use automatic")).toBe(false);
  });
});

describe("Widevine row", () => {
  const show = async (status: CDMStatus) => {
    useCdm.setState({ status });
    await renderSection(Promise.resolve(linuxCapabilities()));
    return screen.findByLabelText("Widevine");
  };

  it("names the installed version and the last update check", async () => {
    const output = await show(cdm());
    expect(output.textContent).toBe("Version 4.10.3112.0");
    expect(shown("Checked for updates 2 hr. ago")).toBe(true);
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("shows a running download's progress on its bar", async () => {
    const output = await show(cdm({ state: "downloading", version: "", received: 42, total: 100 }));
    expect(output.textContent).toBe("Downloading…");
    const bar = screen.getByRole("progressbar", { name: "Widevine download" });
    expect(bar.getAttribute("aria-valuenow")).toBe("42");
    expect(bar.getAttribute("aria-valuetext")).toBe("42%, 42 byte of 100 byte");
  });

  it("shows an update downloading beside the version in use", async () => {
    await show(cdm({ updating: true, received: 10, total: 100 }));
    expect(shown("Downloading an update…")).toBe(true);
    expect(screen.getByRole("progressbar", { name: "Widevine update" })).toBeTruthy();
  });

  it("says why the download failed and retries it", async () => {
    const output = await show(cdm({ state: "failed", version: "", path: "", code: "network" }));
    expect(output.textContent).toBe("Couldn't download");
    const problem =
      "Sona couldn't reach Google's download server. Check your internet connection. Sona also tries again by itself later.";
    expect(screen.getByText(problem)).toBeTruthy();
    const button = screen.getByRole("button", { name: "Retry" });
    expect(button.getAttribute("aria-describedby")).toBe("cdm-problem");
    expect(output.getAttribute("aria-describedby")).toBe("cdm-desc cdm-problem");
    fireEvent.click(button);
    expect(retry).toHaveBeenCalledTimes(1);
    // The button leaves while the download runs; focus waits on the state, not the page.
    expect(document.activeElement).toBe(output);
  });

  it("says the install failed, not the download, when loading it couldn't finish", async () => {
    const output = await show(
      cdm({ state: "failed", version: "", path: "", code: "load_check_incomplete" }),
    );
    expect(output.textContent).toBe("Couldn't install");
    const problem =
      "Sona couldn't finish checking the downloaded Widevine. Try again. Sona also tries again by itself later.";
    expect(screen.getByText(problem)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });

  it("offers Retry when the last update check failed", async () => {
    await show(cdm({ code: "hash_mismatch" }));
    expect(shown("Couldn't check for updates")).toBe(true);
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });

  it("has no Retry where trying again can't help", async () => {
    await show(cdm({ state: "failed", version: "", code: "unsupported_platform" }));
    expect(shown("Google doesn't make Widevine for this computer, so songs can't play here.")).toBe(
      true,
    );
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });
});
