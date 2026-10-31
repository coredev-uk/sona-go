import type { Settings } from "@bindings/config";
import type { Capabilities, Diagnostics } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { DiagnosticsSection } from "./Diagnostics";

const capabilities = vi.fn<() => Promise<Capabilities>>();
const diagnostics = vi.fn<() => Promise<Diagnostics>>();

const settings = {
  diagnostics: { disableDmabufRenderer: false, disableCompositingMode: false },
} as unknown as Settings;

vi.mock("@bindings/services", () => ({
  SystemService: {
    Diagnostics: () => diagnostics(),
    Settings: () => Promise.resolve(settings),
    UpdateSettings: vi.fn(),
  },
  PlatformService: { Capabilities: () => capabilities() },
}));
vi.mock("@wailsio/runtime", () => ({ Clipboard: { SetText: vi.fn() } }));
vi.mock("./Updates", () => ({ UpdatesGroup: () => null }));

/** A diagnostics report; `playback` and `keyring` vary per platform. */
function report(playback: Partial<Diagnostics["playback"]>, backend: string): Diagnostics {
  return {
    app: { version: "1.0.0", goVersion: "go1.26", platform: "test", session: "", startupMs: 0 },
    webkit: { version: "2.48.0" },
    playback: { engine: "", cdmPath: "", cdmVersion: "", cdmOk: false, ...playback },
    database: {
      path: "/tmp/sona.db",
      sizeBytes: 0,
      schemaVersion: 1,
      lastSync: 0,
      counts: { songs: 0, albums: 0, artists: 0, playlists: 0 },
    },
    keyring: { backend, unencrypted: false },
    paths: { logFile: "", settings: "", data: "", cache: "" },
    artworkCacheBytes: 0,
    env: [],
  } as unknown as Diagnostics;
}

async function renderSection(caps: Capabilities, d: Diagnostics) {
  capabilities.mockResolvedValue(caps);
  diagnostics.mockResolvedValue(d);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <DiagnosticsSection />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  await screen.findByText("Token storage");
}

const shown = (text: string) => screen.queryByText(text) !== null;

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("DiagnosticsSection", () => {
  it("says playback isn't available instead of the CDM checks when there is no engine", async () => {
    await renderSection(bareCapabilities(), report({ engine: "" }, "keychain"));
    expect(shown("Not available on this platform yet")).toBe(true);
    for (const row of ["Widevine CDM", "GStreamer", "AAC decoder", "Host sandbox"]) {
      expect(shown(row)).toBe(false);
    }
    expect(shown("Everything Sona needs is working")).toBe(true);
    expect(shown("macOS Keychain")).toBe(true);
    expect(shown("WEBKIT_DISABLE_DMABUF_RENDERER")).toBe(false);
    expect(shown("Disable the DMA-BUF renderer")).toBe(false);
    expect(shown("Rendering")).toBe(false);
    expect(shown("Session")).toBe(false);
  });

  it("shows the CDM checks and the WebKitGTK workarounds on Linux", async () => {
    await renderSection(
      linuxCapabilities(),
      report(
        {
          engine: "cdm",
          cdmOk: true,
          cdmVersion: "4.10",
          gstreamer: "1.24",
          decoder: "avdec_aac",
          sink: "pipewiresink",
          sandboxed: true,
        },
        "secret-service",
      ),
    );
    await screen.findByText("Disable the DMA-BUF renderer");
    expect(shown("Widevine CDM")).toBe(true);
    expect(shown("Not available on this platform yet")).toBe(false);
    expect(shown("Secret Service keyring")).toBe(true);
    expect(shown("WEBKIT_DISABLE_DMABUF_RENDERER")).toBe(true);
    expect(shown("Session")).toBe(true);
  });

  it("names Windows Credential Manager", async () => {
    await renderSection(bareCapabilities("windows"), report({}, "credential-manager"));
    expect(shown("Windows Credential Manager")).toBe(true);
  });
});
