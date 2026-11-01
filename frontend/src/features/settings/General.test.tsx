import type { Settings } from "@bindings/config";
import type { Capabilities } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { GeneralSection } from "./General";

const capabilities = vi.fn<() => Promise<Capabilities>>();
let settings: Settings;

vi.mock("@bindings/services", () => ({
  SystemService: { Settings: () => Promise.resolve(settings), UpdateSettings: vi.fn() },
  PlatformService: { Capabilities: () => capabilities() },
  DesktopService: { ResetWindowSize: vi.fn() },
  IntegrationsService: { Status: () => new Promise(() => {}) },
  UpdatesService: { Status: () => new Promise(() => {}), Check: vi.fn() },
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
  cleanup();
  vi.clearAllMocks();
});

describe("GeneralSection", () => {
  it("groups starting Sona, the window, what happens while music plays and updates", async () => {
    await renderSection(desktop());
    await screen.findByText("Track notifications");
    expect(headings()).toEqual(["Starting Sona", "Window", "While music plays", "Updates"]);
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

  it("leaves out rows and groups the platform can't back", async () => {
    await renderSection(desktop(), bareCapabilities());
    expect(screen.queryByText("Start Sona when you log in")).toBeNull();
    expect(screen.queryByText("When the window is closed")).toBeNull();
    expect(headings()).toEqual(["Starting Sona", "Window", "Updates"]);
  });
});
