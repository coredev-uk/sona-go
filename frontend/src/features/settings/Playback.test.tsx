import type { Settings } from "@bindings/config";
import type { Capabilities } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { PlaybackSection } from "./Playback";

const capabilities = vi.fn<() => Promise<Capabilities>>();

const settings = {
  volume: 0.5,
  engineIdleMinutes: 10,
  audioQuality: 256,
  cdmPath: "",
  audio: { outputDevice: "", soundCheck: false, equalizer: { enabled: false } },
  crossfade: { enabled: false, seconds: 6 },
  queue: { replaceConfirm: "undo" },
} as unknown as Settings;

vi.mock("@bindings/services", () => ({
  SystemService: { Settings: () => Promise.resolve(settings), UpdateSettings: vi.fn() },
  PlatformService: { Capabilities: () => capabilities() },
  AudioService: { OutputDevices: () => Promise.resolve([]) },
  DesktopService: { ChooseCDMPath: vi.fn() },
}));
vi.mock("@/features/private", () => ({
  usePrivate: (select: (s: { state: null }) => unknown) => select({ state: null }),
  isForced: () => false,
  setPrivate: vi.fn(),
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}));

/** The rows and buttons that need a playback capability. */
const gated = [
  "Volume",
  "Stop the playback host when idle",
  "Output",
  "Sound Check",
  "Equaliser",
  "Crossfade",
  "Widevine CDM location",
  "Choose…",
];

const shown = (text: string) => screen.queryByText(text) !== null;

async function renderSection(caps: Promise<Capabilities>) {
  capabilities.mockReturnValue(caps);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <PlaybackSection />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  await screen.findByText("Audio quality");
}

beforeAll(() => {
  // Radix's slider measures with ResizeObserver, which jsdom lacks.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("PlaybackSection", () => {
  it("shows every playback control on Linux, without the unavailable note", async () => {
    await renderSection(Promise.resolve(linuxCapabilities()));
    await screen.findByText("Sound Check");
    expect(gated.filter((text) => !shown(text))).toEqual([]);
    expect(shown("Private listening")).toBe(true);
    expect(shown("Playback isn't available on this platform yet")).toBe(false);
  });

  it("says playback isn't available and leaves out its controls without an engine", async () => {
    await renderSection(Promise.resolve(bareCapabilities()));
    await screen.findByText("Playback isn't available on this platform yet");
    expect(gated.filter(shown)).toEqual([]);
    expect(shown("Audio quality")).toBe(true);
    expect(shown("Private listening")).toBe(true);
  });

  it("keeps the CDM location but not its Choose button without a file chooser", async () => {
    await renderSection(Promise.resolve({ ...linuxCapabilities(), fileChooser: false }));
    await screen.findByText("Widevine CDM location");
    expect(shown("Choose…")).toBe(false);
  });

  it("shows no gated control while the capabilities load", async () => {
    await renderSection(new Promise(() => {}));
    expect(gated.filter(shown)).toEqual([]);
    expect(shown("Playback isn't available on this platform yet")).toBe(false);
  });
});
