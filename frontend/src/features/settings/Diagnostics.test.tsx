import type { Settings } from "@bindings/config";
import type { Capabilities, CDMStatus, Diagnostics } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { I18nRoot } from "@/shared/i18n";
import {
  bareCapabilities,
  linuxCapabilities,
  macCapabilities,
} from "@/shared/testing/capabilities";

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
  CDMService: { Retry: vi.fn(), Status: () => new Promise(() => {}) },
}));
vi.mock("@wailsio/runtime", () => ({ Clipboard: { SetText: vi.fn() } }));
vi.mock("./Updates", () => ({ UpdatesGroup: () => null }));

const readyCdm: CDMStatus = {
  state: "ready",
  version: "4.10.3112.0",
  path: "/data/widevine/4.10.3112.0/linux_x64/libwidevinecdm.so",
  code: "",
  received: 0,
  total: 0,
  updating: false,
  checkedAt: 0,
};

/** A diagnostics report; `playback` and `keyring` vary per platform. */
function report(playback: Partial<Diagnostics["playback"]>, backend: string): Diagnostics {
  return {
    app: { version: "1.0.0", goVersion: "go1.26", platform: "test", session: "", startupMs: 0 },
    webkit: { version: "2.48.0" },
    playback: {
      engine: "",
      cdm: { ...readyCdm, state: "" },
      cdmPath: "",
      cdmVersion: "",
      cdmOk: false,
      ...playback,
    },
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

beforeEach(() => useCdm.setState({ status: null }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

/** The Linux report's playback stack with the CDM in `cdm`; the probe loaded it when `cdmOk`. */
function linuxPlayback(cdm: Partial<CDMStatus>, cdmOk: boolean): Partial<Diagnostics["playback"]> {
  return {
    engine: "cdm",
    cdm: { ...readyCdm, ...cdm },
    cdmOk,
    gstreamer: "1.24",
    decoder: "avdec_aac",
    sink: "pipewiresink",
    sandboxed: true,
  };
}

/** The value cell beside a row's label (the first such row: Sona's own group has a Version too). */
const valueOf = (label: string) =>
  screen.getAllByText(label, { selector: "dt" })[0].nextElementSibling?.textContent ?? "";

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
          cdm: readyCdm,
          cdmOk: true,
          cdmVersion: "4.10.3112.0",
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
    expect(shown("CDM location")).toBe(false);
  });

  it("names the CDM's state, source, version and update check", async () => {
    await renderSection(linuxCapabilities(), report(linuxPlayback({}, true), "secret-service"));
    expect(valueOf("Widevine CDM")).toBe("Working");
    expect(valueOf("Source")).toBe("Downloaded from Google");
    expect(valueOf("Version")).toContain("4.10.3112.0");
    expect(valueOf("Updates")).toBe("Not checked yet");
    expect(valueOf("CDM file")).toBe(readyCdm.path);
    expect(shown("Everything Sona needs is working")).toBe(true);
  });

  it("follows a running download from cdm:status", async () => {
    await renderSection(
      linuxCapabilities(),
      report(linuxPlayback({ state: "downloading", version: "", path: "" }, false), "keychain"),
    );
    expect(valueOf("Widevine CDM")).toContain("Downloading…");
    act(() =>
      useCdm.setState({
        status: { ...readyCdm, state: "downloading", version: "", received: 3, total: 4 },
      }),
    );
    expect(valueOf("Widevine CDM")).toContain("Downloading · 75%");
    expect(valueOf("Version")).toBe("—");
    // No CDM, no probe: GStreamer and the rest weren't checked, which isn't a problem.
    expect(valueOf("GStreamer")).toBe("Checked once Widevine is ready");
    expect(valueOf("AAC decoder")).toBe("Checked once Widevine is ready");
    expect(shown("Everything Sona needs is working")).toBe(true);
  });

  it("counts a failed download as a problem and says why", async () => {
    await renderSection(
      linuxCapabilities(),
      report(
        linuxPlayback({ state: "failed", version: "", path: "", code: "signature_invalid" }, false),
        "keychain",
      ),
    );
    expect(valueOf("Widevine CDM")).toContain("Couldn't download");
    expect(valueOf("Widevine CDM")).toContain(
      "The download isn't signed by Google, so Sona deleted it. Try again later.",
    );
    expect(shown("1 problem found")).toBe(true);
  });

  it("warns when the last update check failed", async () => {
    await renderSection(
      linuxCapabilities(),
      report(linuxPlayback({ code: "network" }, true), "keychain"),
    );
    expect(valueOf("Updates")).toContain("Couldn't check for updates");
    expect(shown("Everything Sona needs is working")).toBe(true);
  });

  it("names Windows Credential Manager", async () => {
    await renderSection(bareCapabilities("windows"), report({}, "credential-manager"));
    expect(shown("Windows Credential Manager")).toBe(true);
  });

  it("shows the macOS sandbox, or that the host runs without it", async () => {
    const mac = {
      ...linuxPlayback({ path: "/data/widevine/4.10.3112.0/mac_arm64/libwidevinecdm.dylib" }, true),
      sink: "osxaudiosink",
    };
    await renderSection(
      macCapabilities(),
      report({ ...mac, sandboxKind: "seatbelt", sandboxed: true }, "keychain"),
    );
    expect(shown("Sandboxed by macOS · no network or keychain access")).toBe(true);
    cleanup();

    const note = "no macOS sandbox: stat /usr/bin/sandbox-exec: no such file or directory";
    await renderSection(
      macCapabilities(),
      report({ ...mac, sandboxKind: "none", sandboxed: false, sandboxNote: note }, "keychain"),
    );
    expect(shown("Not sandboxed")).toBe(true);
    expect(screen.queryByText(/the host can reach the network and your files/)).not.toBeNull();
    expect(shown("Sandboxed by macOS · no network or keychain access")).toBe(false);
  });
});
