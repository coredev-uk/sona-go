import { DesktopService, type WindowControls, type WindowState } from "@bindings/services";
import { Events } from "@wailsio/runtime";
import { t } from "@lingui/core/macro";
import {
  useSyncExternalStore,
  type CSSProperties,
  type HTMLAttributes,
  type MouseEvent,
} from "react";
import { create } from "zustand";

import { useCapabilities } from "@/shared/lib/capabilities";
import { run } from "@/shared/lib/run";

/** A window button Sona draws. */
export type WindowButton = "minimize" | "maximize" | "close";

/**
 * How the main window's title bar looks, from Go's data (never from the user agent):
 * - `native-inset`: macOS's own buttons over Sona's top bar, which keeps `startRoom` for them;
 * - `drawn`: Sona's own title bar in a frameless window, with `start`/`end` buttons (maybe none);
 * - `none`: the desktop's own title bar (or none, on a tiling compositor) frames the window.
 */
export interface WindowChrome {
  kind: "native-inset" | "drawn" | "none";
  start: WindowButton[];
  end: WindowButton[];
  /** Pixels kept free at the window's leading edge for the system's buttons. */
  startRoom: number;
  /** The title bar's height in pixels (the sidebar header and the top bar). */
  height: number;
  /** Windows' caption buttons, else Sona's compact ones. */
  captions: boolean;
  /** A right click on the title bar opens the desktop's window menu. */
  windowMenu: boolean;
}

/** The top bar's height everywhere but macOS (docs/design.md, Shell). */
export const TITLE_BAR_HEIGHT = 48;
/**
 * macOS: AppKit centres the traffic lights in a unified toolbar's height
 * (Go's MacTitleBarHiddenInset), so the top bar takes that height and its
 * controls line up with them; the lights and their margin take `MAC_START_ROOM`.
 */
export const MAC_TITLE_BAR_HEIGHT = 52;
export const MAC_START_ROOM = 84;

const BUTTONS: readonly string[] = ["minimize", "maximize", "close"];
const buttons = (list: readonly string[] | undefined): WindowButton[] =>
  (list ?? []).filter((b): b is WindowButton => BUTTONS.includes(b));

/**
 * The main window's title bar from Go's platform data (with the title bar
 * style the window was created with: a change in Settings → General
 * applies from the next start) and the window's state. The same rule as
 * Go's windowchrome.Platform.Resolve, which made the window frameless.
 */
export function resolveWindowChrome(
  controls: WindowControls | undefined,
  state: WindowState,
  os: string | undefined,
): WindowChrome {
  const none: WindowChrome = {
    kind: "none",
    start: [],
    end: [],
    startRoom: 0,
    height: TITLE_BAR_HEIGHT,
    captions: false,
    windowMenu: false,
  };
  if (!controls) return none;
  if (controls.native === "inset") {
    // macOS hides its buttons in full screen.
    return state.fullscreen
      ? { ...none, kind: "native-inset" }
      : { ...none, kind: "native-inset", startRoom: MAC_START_ROOM, height: MAC_TITLE_BAR_HEIGHT };
  }
  const style = controls.style;
  const chosen = !style || style === "auto" ? controls.automatic : style;
  if (chosen !== "sona" && chosen !== "sona-no-buttons") return none;
  const withButtons = chosen === "sona";
  return {
    ...none,
    kind: "drawn",
    start: withButtons ? buttons(controls.start) : [],
    end: withButtons ? buttons(controls.end) : [],
    captions: os === "windows",
    windowMenu: controls.windowMenu,
  };
}

interface WindowStateStore {
  state: WindowState;
  set: (state: WindowState) => void;
}

/** The main window's state (maximised, full screen), mirrored from Go. */
export const useWindowStateStore = create<WindowStateStore>((set) => ({
  state: { maximised: false, fullscreen: false },
  set: (state) => set({ state }),
}));

/** Follows Go's window:state; call once per window (the shell does). Returns the unsubscribe. */
export function bindWindowState(): () => void {
  const { set } = useWindowStateStore.getState();
  void DesktopService.WindowState().then(set, () => {});
  return Events.On("window:state", ({ data }) => set(data));
}

/** The main window's title bar (see {@link resolveWindowChrome}). */
export function useWindowChrome(): WindowChrome {
  const caps = useCapabilities();
  const state = useWindowStateStore((s) => s.state);
  return resolveWindowChrome(caps?.windowControls, state, caps?.os);
}

function subscribeFocus(onChange: () => void) {
  window.addEventListener("focus", onChange);
  window.addEventListener("blur", onChange);
  return () => {
    window.removeEventListener("focus", onChange);
    window.removeEventListener("blur", onChange);
  };
}

/** Whether this window is the active one, so a title bar can dim when it isn't. */
export function useWindowActive(): boolean {
  return useSyncExternalStore(
    subscribeFocus,
    () => document.hasFocus(),
    () => true,
  );
}

/** What on the title bar is a control, not a place to grab the window. */
const CONTROLS = "button, a, input, textarea, select, [role='combobox'], [role='slider']";

/**
 * Props for a part of the main window's title bar (the top bar, the
 * sidebar's header) when Sona draws it, or macOS's buttons sit over it:
 * the window drags from its empty parts (Wails' runtime; controls opt out
 * through index.css), a double click there maximises or restores (macOS:
 * Wails does it as the system's title bar setting says), and a right click
 * opens the desktop's window menu where it has one.
 */
export function titleBarRegion(chrome: WindowChrome): HTMLAttributes<HTMLElement> & {
  "data-title-bar"?: "";
} {
  if (chrome.kind === "none") return {};
  const onEmpty = (e: MouseEvent) => !(e.target as Element).closest(CONTROLS);
  return {
    "data-title-bar": "",
    style: { "--wails-draggable": "drag" } as CSSProperties,
    onDoubleClick:
      chrome.kind === "drawn"
        ? (e) => {
            if (onEmpty(e))
              void run(t`Couldn't resize the window`, DesktopService.ToggleMaximiseWindow());
          }
        : undefined,
    onContextMenu: chrome.windowMenu
      ? (e) => {
          // A mouse's right button only: GTK opens the menu at that press.
          if (e.button !== 2 || !onEmpty(e)) return;
          e.preventDefault();
          void DesktopService.ShowWindowMenu().catch(() => {});
        }
      : undefined,
  };
}
