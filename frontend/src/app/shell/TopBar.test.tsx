import type { Capabilities } from "@bindings/services";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import {
  linuxCapabilities,
  macCapabilities,
  windowsCapabilities,
} from "@/shared/testing/capabilities";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { useLayout } from "./layout";
import { TopBar } from "./TopBar";
import { useWindowStateStore } from "./window-chrome";

const desktop = vi.hoisted(() => ({
  MinimiseWindow: vi.fn(() => Promise.resolve()),
  ToggleMaximiseWindow: vi.fn(() => Promise.resolve()),
  CloseWindow: vi.fn(() => Promise.resolve()),
  ShowWindowMenu: vi.fn(() => Promise.resolve()),
  WindowState: vi.fn(() => Promise.resolve({ maximised: false, fullscreen: false })),
}));
const env = vi.hoisted(() => ({ caps: undefined as Capabilities | undefined }));

vi.mock("@bindings/services", () => ({ DesktopService: desktop }));
vi.mock("@/shared/lib/capabilities", () => ({ useCapabilities: () => env.caps }));
vi.mock("../compose", () => ({ slots: [] }));
vi.mock("@/features/auth", () => ({ AccountMenu: () => <button type="button">Account</button> }));
vi.mock("@tanstack/react-router", () => ({
  useRouter: () => ({ history: { back: vi.fn(), forward: vi.fn() } }),
}));

function renderBar(caps: Capabilities, style = "auto") {
  env.caps = { ...caps, windowControls: { ...caps.windowControls, style } };
  render(
    <TooltipProvider>
      <TopBar />
    </TooltipProvider>,
    { wrapper: I18nRoot },
  );
  return screen.getByRole("banner");
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  useLayout.setState({ sidebar: true });
  useWindowStateStore.setState({ state: { maximised: false, fullscreen: false } });
});

describe("TopBar as the window's title bar", () => {
  it("draws the desktop's buttons at its end and drags the window from its empty parts", () => {
    const bar = renderBar(linuxCapabilities());
    const group = within(bar).getByRole("group", { name: "Window" });
    expect(
      within(group)
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Close"]);
    expect(bar.style.getPropertyValue("--wails-draggable")).toBe("drag");
    fireEvent.click(within(group).getByRole("button", { name: "Close" }));
    expect(desktop.CloseWindow).toHaveBeenCalledOnce();
  });

  it("maximises on a double click on the bar, not on its controls", () => {
    const bar = renderBar(windowsCapabilities());
    fireEvent.doubleClick(within(bar).getByRole("button", { name: "Back" }));
    fireEvent.doubleClick(within(bar).getByRole("button", { name: "Minimise" }));
    expect(desktop.ToggleMaximiseWindow).not.toHaveBeenCalled();
    fireEvent.doubleClick(bar);
    expect(desktop.ToggleMaximiseWindow).toHaveBeenCalledOnce();
  });

  it("opens the desktop's window menu on a right click on its empty parts only", () => {
    const bar = renderBar(linuxCapabilities());
    fireEvent.contextMenu(within(bar).getByRole("button", { name: "Back" }), { button: 2 });
    expect(desktop.ShowWindowMenu).not.toHaveBeenCalled();
    fireEvent.contextMenu(bar, { button: 2 });
    expect(desktop.ShowWindowMenu).toHaveBeenCalledOnce();
  });

  it("shows Windows' three caption buttons, Restore while maximised", () => {
    useWindowStateStore.setState({ state: { maximised: true, fullscreen: false } });
    const bar = renderBar(windowsCapabilities());
    const group = within(bar).getByRole("group", { name: "Window" });
    expect(
      within(group)
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Minimise", "Restore", "Close"]);
    fireEvent.click(within(group).getByRole("button", { name: "Restore" }));
    expect(desktop.ToggleMaximiseWindow).toHaveBeenCalledOnce();
    // Windows has no window menu Sona opens: a right click does nothing.
    fireEvent.contextMenu(bar, { button: 2 });
    expect(desktop.ShowWindowMenu).not.toHaveBeenCalled();
  });

  it("is an ordinary bar under the desktop's own title bar", () => {
    const bar = renderBar(linuxCapabilities(), "desktop");
    expect(within(bar).queryByRole("group", { name: "Window" })).toBeNull();
    expect(bar.style.getPropertyValue("--wails-draggable")).toBe("");
    fireEvent.doubleClick(bar);
    fireEvent.contextMenu(bar, { button: 2 });
    expect(desktop.ToggleMaximiseWindow).not.toHaveBeenCalled();
    expect(desktop.ShowWindowMenu).not.toHaveBeenCalled();
  });

  it("takes the leading buttons when the sidebar is hidden", () => {
    useLayout.setState({ sidebar: false });
    const caps = linuxCapabilities();
    caps.windowControls = { ...caps.windowControls, start: ["close"], end: ["minimize"] };
    const bar = renderBar(caps);
    const [first, last] = within(bar).getAllByRole("group", { name: "Window" });
    expect(within(first).getByRole("button").getAttribute("aria-label")).toBe("Close");
    expect(within(last).getByRole("button").getAttribute("aria-label")).toBe("Minimise");
  });

  it("leaves room for macOS's own buttons only while the sidebar doesn't", () => {
    const bar = renderBar(macCapabilities());
    expect(within(bar).queryByRole("group", { name: "Window" })).toBeNull();
    expect(bar.style.height).toBe("52px");
    expect(bar.style.paddingInlineStart).toBe("12px");
    cleanup();
    useLayout.setState({ sidebar: false });
    expect(renderBar(macCapabilities()).style.paddingInlineStart).toBe("84px");
  });
});
