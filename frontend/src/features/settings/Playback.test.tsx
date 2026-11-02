import type { Settings } from "@bindings/config";
import type { Capabilities, CDMStatus } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { I18nRoot } from "@/shared/i18n";
import { keys } from "@/shared/lib/keys";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { moveProvider } from "./lyrics-order";
import { PlaybackSection } from "./Playback";

const capabilities = vi.fn<() => Promise<Capabilities>>();
const retry = vi.hoisted(() => vi.fn(() => Promise.resolve()));
const updateSettings = vi.hoisted(() => vi.fn((s: unknown) => Promise.resolve(s)));

const settings = {
  engineIdleMinutes: 10,
  audioQuality: 256,
  audio: { outputDevice: "", soundCheck: false, equalizer: { enabled: false } },
  crossfade: { enabled: false, seconds: 6 },
  queue: { replaceConfirm: "undo" },
  lyrics: {
    providers: [
      { id: "apple", enabled: true },
      { id: "musixmatch", enabled: true },
      { id: "lrclib", enabled: true },
      { id: "netease", enabled: false },
    ],
    preferWordTiming: true,
    translationLanguage: "",
  },
} as unknown as Settings;

vi.mock("@bindings/services", () => ({
  SystemService: { Settings: () => Promise.resolve(settings), UpdateSettings: updateSettings },
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
      "Lyrics",
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

  it("says playback isn't available and keeps only the lyrics without an engine", async () => {
    await renderSection(Promise.resolve(bareCapabilities()));
    await screen.findByText("Playback isn't available on this platform yet");
    expect(gated.filter(shown)).toEqual([]);
    // Song pages still show lyrics.
    expect(screen.queryAllByRole("heading").map((h) => h.textContent)).toEqual(["Lyrics"]);
  });

  it("explains private listening where the engine reports plays itself", async () => {
    const caps = linuxCapabilities();
    caps.playback.privateListening = false;
    await renderSection(Promise.resolve(caps));
    await screen.findByText("Replacing your queue");
    expect(shown("Private listening")).toBe(true);
    expect(screen.queryByRole("switch", { name: "Private listening" })).toBeNull();
    expect(
      shown("This platform's player reports plays to Apple itself, so Sona can't hold them back."),
    ).toBe(true);
  });

  it("keeps the rows this platform's playback can't back, saying why", async () => {
    const caps = linuxCapabilities();
    caps.os = "darwin";
    caps.playback.outputDevice = false;
    caps.playback.equalizer = false;
    caps.playback.soundCheck = false;
    caps.playback.crossfade = false;
    await renderSection(Promise.resolve(caps));
    await screen.findByText("Replacing your queue");
    for (const title of ["Output", "Sound Check", "Equaliser", "Crossfade"]) {
      const row = screen.getByRole("group", { name: title });
      expect(row.textContent).toContain("Not available on macOS yet.");
    }
    expect(screen.queryByRole("switch", { name: "Crossfade" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Open equaliser" })).toBeNull();
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

describe("Lyrics rows", () => {
  const sources = () =>
    within(screen.getByRole("list", { name: "Lyrics sources" }))
      .getAllByRole("listitem")
      .map((li) => li.querySelector("label")?.textContent);
  const saved = () => (updateSettings.mock.calls.at(-1)?.[0] as Settings).lyrics;

  it("lists the sources in order, with NetEase off", async () => {
    await renderSection(Promise.resolve(linuxCapabilities()));
    await screen.findByRole("list", { name: "Lyrics sources" });
    expect(sources()).toEqual(["Apple Music", "Musixmatch", "LRCLIB", "NetEase Cloud Music"]);
    expect(
      screen.getByRole("switch", { name: "NetEase Cloud Music" }).getAttribute("aria-checked"),
    ).toBe("false");
    expect(screen.getByRole("button", { name: "Move Apple Music up" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(screen.getByRole("button", { name: "Move NetEase Cloud Music down" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("moves a source and switches one on, saving each change", async () => {
    await renderSection(Promise.resolve(linuxCapabilities()));
    fireEvent.click(await screen.findByRole("button", { name: "Move LRCLIB up" }));
    await waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1));
    expect(saved().providers.map((p) => p.id)).toEqual([
      "apple",
      "lrclib",
      "musixmatch",
      "netease",
    ]);
    await waitFor(() => expect(sources()[1]).toBe("LRCLIB"));
    fireEvent.click(screen.getByRole("switch", { name: "NetEase Cloud Music" }));
    await waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(2));
    expect(saved().providers.find((p) => p.id === "netease")?.enabled).toBe(true);
  });

  it("saves the word-timing preference", async () => {
    await renderSection(Promise.resolve(linuxCapabilities()));
    fireEvent.click(await screen.findByRole("switch", { name: "Prefer word-by-word lyrics" }));
    await waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1));
    expect(saved().preferWordTiming).toBe(false);
  });
});

describe("moveProvider", () => {
  const list = [
    { id: "a", enabled: true },
    { id: "b", enabled: false },
  ];
  it("swaps with the neighbour and stops at the ends", () => {
    expect(moveProvider(list, 1, -1).map((p) => p.id)).toEqual(["b", "a"]);
    expect(moveProvider(list, 0, -1).map((p) => p.id)).toEqual(["a", "b"]);
    expect(moveProvider(list, 1, 1).map((p) => p.id)).toEqual(["a", "b"]);
  });
});
