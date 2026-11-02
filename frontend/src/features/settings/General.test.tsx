import type { Settings } from "@bindings/config";
import type { AppleLanguages, Capabilities } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { GeneralSection } from "./General";

const capabilities = vi.fn<() => Promise<Capabilities>>();
let settings: Settings;
let appleLanguages: AppleLanguages = { mapped: 0.07, locales: [] };

vi.mock("@bindings/services", () => ({
  SystemService: { Settings: () => Promise.resolve(settings), UpdateSettings: vi.fn() },
  PlatformService: { Capabilities: () => capabilities() },
  DesktopService: { ResetWindowSize: vi.fn() },
  IntegrationsService: { Status: () => new Promise(() => {}) },
  UpdatesService: { Status: () => new Promise(() => {}), Check: vi.fn() },
  TranslationsService: { Languages: () => Promise.resolve(appleLanguages) },
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}));

function desktop(patch: Partial<Settings["desktop"]> = {}): Settings {
  return {
    desktop: {
      closeAction: "background",
      autostart: false,
      startHidden: true,
      startPage: "/",
      resumeLastRoute: false,
      lastRoute: "",
      ...patch,
    },
    window: { width: 1280, height: 800, maximised: false },
    titleBar: { style: "auto" },
    integrations: { notifications: true, inhibitSleep: true, discordPresence: false },
    updates: { check: true },
  } as unknown as Settings;
}

async function renderSection(s: Settings, caps: Capabilities = linuxCapabilities()) {
  settings = s;
  capabilities.mockReturnValue(Promise.resolve(caps));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <GeneralSection />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  await screen.findByText("Window size");
}

const headings = () => screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent);

afterEach(() => {
  appleLanguages = { mapped: 0.07, locales: [] };
  cleanup();
  vi.clearAllMocks();
});

describe("GeneralSection", () => {
  it("groups starting Sona, the window, what happens while music plays and updates", async () => {
    await renderSection(desktop());
    await screen.findByText("Track notifications");
    expect(headings()).toEqual(["Starting Sona", "Window", "While music plays", "Updates"]);
  });

  it("offers no Language row while English is the only complete language", async () => {
    await renderSection(desktop());
    await screen.findByText("Track notifications");
    expect(screen.queryByRole("combobox", { name: "Language" })).toBeNull();
  });

  it("puts Language first once a second language is selectable", async () => {
    appleLanguages = {
      mapped: 0.95,
      locales: [
        { tag: "fr-FR", coverage: 0.95, measured: true },
        { tag: "de-DE", coverage: 0.4, measured: true },
      ],
    };
    await renderSection(desktop());
    const language = await screen.findByRole("combobox", { name: "Language" });
    expect(language.textContent).toBe("Automatic");
    expect(screen.getAllByRole("combobox")[0]).toBe(language);
  });

  it("keeps the Language row while a language is chosen", async () => {
    await renderSection({ ...desktop(), language: "fr-FR" } as Settings);
    const language = await screen.findByRole("combobox", { name: "Language" });
    expect(language.textContent).toBe("Français (France)");
  });

  it("offers Start in the tray only once Sona starts at login", async () => {
    await renderSection(desktop({ autostart: false }));
    await screen.findByText("Start Sona when you log in");
    expect(screen.queryByText("Start in the tray")).toBeNull();
    cleanup();
    await renderSection(desktop({ autostart: true }));
    expect(await screen.findByText("Start in the tray")).toBeTruthy();
  });

  it("shows resuming the last page as Open on's choice", async () => {
    await renderSection(desktop({ resumeLastRoute: true, startPage: "/radio" }));
    const open = screen.getByRole("combobox", { name: "Open on" });
    expect(open.textContent).toBe("The page you were last on");
    cleanup();
    await renderSection(desktop({ resumeLastRoute: false, startPage: "/radio" }));
    expect(screen.getByRole("combobox", { name: "Open on" }).textContent).toBe("Radio");
  });

  it("keeps the rows the platform can't back, each saying why", async () => {
    await renderSection(desktop(), bareCapabilities());
    expect(headings()).toEqual(["Starting Sona", "Window", "While music plays", "Updates"]);
    const reasons: [string, string][] = [
      ["Start Sona when you log in", "Not available on macOS yet."],
      ["When the window is closed", "Sona quits: it can't stay in the system tray on macOS yet."],
      ["Track notifications", "Not available on macOS yet."],
      ["Stay awake while playing", "Not available on macOS yet."],
    ];
    for (const [title, reason] of reasons) {
      expect(screen.getByRole("group", { name: title }).textContent).toContain(reason);
    }
    expect(screen.queryByRole("switch", { name: "Start Sona when you log in" })).toBeNull();
    expect(screen.queryByRole("combobox", { name: "When the window is closed" })).toBeNull();
    expect(screen.queryByText("Start in the tray")).toBeNull();
  });
});
