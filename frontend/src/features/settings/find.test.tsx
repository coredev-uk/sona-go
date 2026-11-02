import type { Settings } from "@bindings/config";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { I18nRoot } from "@/shared/i18n";
import type { Shortcut } from "@/shared/module";
import { registerShortcuts } from "@/shared/shortcuts/registry";
import { linuxCapabilities } from "@/shared/testing/capabilities";

import { AppearanceSection } from "./AppearanceSection";
import { ConnectionsSection } from "./Connections";
import { DiagnosticsSection } from "./Diagnostics";
import { findSettings, focusSetting, settingIndex, type FindContext } from "./find";
import { FindSetting } from "./FindSetting";
import { GeneralSection } from "./General";
import { PlaybackSection } from "./Playback";
import type { SettingsSection } from "./search-params";
import { ShortcutsSection } from "./Shortcuts";
import { StorageSection } from "./Storage";

const settings = {
  language: "",
  theme: "system",
  reduceEffects: false,
  motionArtwork: { enabled: true },
  desktop: {
    closeAction: "background",
    autostart: true,
    startHidden: false,
    startPage: "/",
    resumeLastRoute: false,
    lastRoute: "",
  },
  window: { width: 1280, height: 800, maximised: false },
  titleBar: { style: "auto" },
  integrations: { notifications: true, inhibitSleep: true, discordPresence: false },
  updates: { check: true },
  engineIdleMinutes: 10,
  audioQuality: 256,
  audio: { outputDevice: "", soundCheck: false, equalizer: { enabled: false } },
  crossfade: { enabled: true, seconds: 6 },
  queue: { replaceConfirm: "undo" },
  lyrics: {
    providers: [
      { id: "apple", enabled: true },
      { id: "lrclib", enabled: true },
    ],
    preferWordTiming: true,
    translationLanguage: "",
  },
  scrobbling: {
    lastfm: { enabled: false, username: "" },
    listenbrainz: { enabled: false, username: "" },
  },
  storage: { artCacheMiB: 512 },
  shortcuts: {},
  diagnostics: { disableDmabufRenderer: false, disableCompositingMode: false },
} as unknown as Settings;

const scrobbler = { available: true, connected: false, connecting: false, pending: 0 };

vi.mock("@bindings/services", () => ({
  SystemService: {
    Settings: () => Promise.resolve(settings),
    UpdateSettings: vi.fn(),
    Diagnostics: () => new Promise(() => {}),
  },
  PlatformService: { Capabilities: () => Promise.resolve(linuxCapabilities()) },
  DesktopService: { ResetWindowSize: vi.fn() },
  IntegrationsService: { Status: () => new Promise(() => {}) },
  UpdatesService: { Status: () => new Promise(() => {}), Check: vi.fn() },
  TranslationsService: {
    Languages: () =>
      Promise.resolve({
        mapped: 0.95,
        locales: [{ tag: "fr-FR", coverage: 0.95, measured: true }],
      }),
  },
  AudioService: { OutputDevices: () => Promise.resolve([]) },
  CDMService: { Retry: vi.fn(), Status: () => new Promise(() => {}) },
  ScrobbleService: {
    Status: () =>
      Promise.resolve({ lastfm: scrobbler, listenbrainz: scrobbler, unencrypted: false }),
  },
  StorageService: {
    Usage: () =>
      Promise.resolve({
        artworkBytes: 1024,
        artworkLimitBytes: 512 * 1024 * 1024,
        catalogBytes: 2048,
        catalogEntries: 3,
        databaseBytes: 300_000,
        databaseFreeBytes: 0,
        lastMaintenance: 0,
      }),
  },
  MotionService: { Status: () => new Promise(() => {}), Report: vi.fn() },
}));
vi.mock("@/features/private", () => ({
  usePrivate: (select: (s: { state: null }) => unknown) => select({ state: null }),
  isForced: () => false,
  setPrivate: vi.fn(),
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}));
vi.mock("@wailsio/runtime", () => ({
  Clipboard: { SetText: vi.fn() },
  Browser: { OpenURL: vi.fn() },
}));

const sc = (id: string, label: string): Shortcut => ({
  id,
  label: () => label,
  group: "view",
  keys: [],
  run: () => {},
});

beforeAll(() => {
  // Radix sliders measure themselves; jsdom has no ResizeObserver.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  registerShortcuts([sc("test.sidebar", "Show or hide the sidebar")]);
  useCdm.setState({
    status: {
      state: "ready",
      version: "4.10.3112.0",
      path: "",
      code: "",
      received: 0,
      total: 0,
      updating: false,
      checkedAt: 0,
    },
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderWithData(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>, {
    wrapper: I18nRoot,
  });
}

const context: FindContext = { caps: linuxCapabilities(), settings, languageRow: true };
const ids = (query: string, c: FindContext = context) =>
  findSettings(settingIndex(), query, c, "en").map((e) => e.id);

describe("findSettings", () => {
  it("puts titles that start with the query first", () => {
    expect(ids("cross")).toEqual(["crossfade", "crossfade-seconds"]);
    expect(ids("lyrics")[0]).toBe("lyrics-sources");
  });

  it("finds a setting by its other words, its group and its section", () => {
    expect(ids("dark")).toEqual(["theme"]);
    expect(ids("tray")).toContain("close-action");
    expect(ids("scrobbling")).toEqual(["scrobble-lastfm", "scrobble-listenbrainz"]);
  });

  it("needs every word, in any case and without accents", () => {
    expect(ids("CACHE artwork")).toEqual(["art-cache"]);
    expect(ids("équaliser")).toEqual(["equalizer"]);
    expect(ids("nothing like this")).toEqual([]);
  });

  it("leaves out rows their section doesn't show", () => {
    const off = { ...settings, crossfade: { enabled: false, seconds: 6 } } as Settings;
    expect(ids("crossfade", { ...context, settings: off })).toEqual(["crossfade"]);
    expect(ids("language", { ...context, languageRow: false })).toEqual(["lyrics-translation"]);
    const noPlayback = linuxCapabilities();
    noPlayback.playback.available = false;
    expect(ids("audio quality", { ...context, caps: noPlayback })).toEqual([]);
  });

  it("finds shortcuts by their action", () => {
    expect(ids("sidebar")).toEqual(["shortcut-test.sidebar"]);
  });
});

describe("the setting index", () => {
  const sections: [SettingsSection, ReactElement, string][] = [
    ["general", <GeneralSection key="g" />, "Window size"],
    ["playback", <PlaybackSection key="p" />, "Free memory when idle"],
    ["appearance", <AppearanceSection key="a" />, "Motion artwork"],
    ["connections", <ConnectionsSection key="c" />, "Discord status"],
    ["shortcuts", <ShortcutsSection key="s" />, "Show or hide the sidebar"],
    ["storage", <StorageSection key="st" />, "Settings file"],
    ["diagnostics", <DiagnosticsSection key="d" />, "Copy diagnostics"],
  ];

  it.each(sections)("finds every %s row on its section", async (section, ui, ready) => {
    renderWithData(ui);
    await screen.findAllByText(ready);
    // The language row waits for the languages.
    if (section === "general") await screen.findByRole("combobox", { name: "Language" });
    // Rendering's switches come with the checks' report (Diagnostics.test.tsx).
    const expected = settingIndex().filter(
      (e) =>
        e.section === section &&
        (!e.when || e.when(context)) &&
        !(section === "diagnostics" && e.group === "Rendering"),
    );
    expect(expected.length).toBeGreaterThan(0);
    const missing = expected.filter((e) => !document.querySelector(`[data-setting="${e.id}"]`));
    expect(missing.map((e) => e.id)).toEqual([]);
  });
});

describe("FindSetting", () => {
  function renderFind() {
    const onPick = vi.fn();
    renderWithData(
      <FindSetting title={<h1>Settings</h1>} onPick={onPick}>
        <nav aria-label="Settings sections">sections</nav>
      </FindSetting>,
    );
    return { onPick, field: screen.getByRole("searchbox", { name: "Find a setting" }) };
  }

  it("lists what a query finds in place of the sections and announces the count", async () => {
    const { field } = renderFind();
    fireEvent.change(field, { target: { value: "dark" } });
    const results = await screen.findByRole("list", { name: "Settings found" });
    expect(within(results).getByRole("button").textContent).toBe("ThemeAppearance");
    expect(screen.queryByRole("navigation", { name: "Settings sections" })).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("1 setting found");
    fireEvent.change(field, { target: { value: "zzz" } });
    expect(screen.getByRole("status").textContent).toBe("No setting matches “zzz”.");
  });

  it("moves into the results with the arrow keys and back to the field", async () => {
    const { field } = renderFind();
    fireEvent.change(field, { target: { value: "cache" } });
    const buttons = within(
      await screen.findByRole("list", { name: "Settings found" }),
    ).getAllByRole("button");
    field.focus();
    fireEvent.keyDown(field, { key: "ArrowDown" });
    expect(document.activeElement).toBe(buttons[0]);
    fireEvent.keyDown(buttons[0], { key: "ArrowDown" });
    expect(document.activeElement).toBe(buttons[1]);
    fireEvent.keyDown(buttons[1], { key: "ArrowUp" });
    fireEvent.keyDown(buttons[0], { key: "ArrowUp" });
    expect(document.activeElement).toBe(field);
    fireEvent.keyDown(field, { key: "ArrowDown" });
    fireEvent.keyDown(buttons[0], { key: "Escape" });
    expect(document.activeElement).toBe(field);
    expect((field as HTMLInputElement).value).toBe("");
    expect(screen.getByRole("navigation", { name: "Settings sections" })).toBeTruthy();
  });

  it("opens the setting chosen, by click or Enter on the field, and clears the query", async () => {
    const { field, onPick } = renderFind();
    fireEvent.change(field, { target: { value: "theme" } });
    fireEvent.click(await screen.findByRole("button", { name: /^Theme/ }));
    expect(onPick).toHaveBeenCalledWith(
      expect.objectContaining({ id: "theme", section: "appearance" }),
    );
    expect((field as HTMLInputElement).value).toBe("");
    fireEvent.change(field, { target: { value: "sound check" } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(onPick).toHaveBeenLastCalledWith(expect.objectContaining({ id: "sound-check" }));
  });
});

describe("focusSetting", () => {
  it("waits for the row, focuses its control and marks the row", async () => {
    vi.useFakeTimers({
      toFake: ["requestAnimationFrame", "cancelAnimationFrame", "setTimeout", "Date"],
    });
    try {
      focusSetting("theme");
      const { container } = render(
        <div data-setting="theme">
          <p>Theme</p>
          <button type="button">System</button>
        </div>,
      );
      const row = container.querySelector<HTMLElement>('[data-setting="theme"]')!;
      row.scrollIntoView = vi.fn();
      await act(() => vi.advanceTimersByTimeAsync(50));
      expect(document.activeElement?.textContent).toBe("System");
      expect(row.hasAttribute("data-found")).toBe(true);
      await act(() => vi.advanceTimersByTimeAsync(2500));
      expect(row.hasAttribute("data-found")).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("focuses a row without a control (a setting the platform can't back)", () => {
    const { container } = render(
      <div data-setting="autostart">
        <p>Start Sona when you log in</p>
      </div>,
    );
    const row = container.querySelector<HTMLElement>('[data-setting="autostart"]')!;
    row.scrollIntoView = vi.fn();
    focusSetting("autostart");
    expect(document.activeElement).toBe(row);
  });
});
