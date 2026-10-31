import { describe, expect, it } from "vitest";

import {
  linuxCapabilities,
  macCapabilities,
  windowsCapabilities,
} from "@/shared/testing/capabilities";

import {
  MAC_START_ROOM,
  MAC_TITLE_BAR_HEIGHT,
  resolveWindowChrome,
  TITLE_BAR_HEIGHT,
} from "./window-chrome";

const normal = { maximised: false, fullscreen: false };
const gnome = linuxCapabilities().windowControls;
const tiling = { ...gnome, automatic: "desktop", desktop: "sway" };
const styled = (style: string, controls = gnome) => ({ ...controls, style });

describe("resolveWindowChrome", () => {
  it("draws Sona's title bar with the desktop's buttons where Automatic picks Sona's", () => {
    expect(resolveWindowChrome(gnome, normal, "linux")).toEqual({
      kind: "drawn",
      start: [],
      end: ["close"],
      startRoom: 0,
      height: TITLE_BAR_HEIGHT,
      captions: false,
      windowMenu: true,
    });
  });

  it("leaves a tiling compositor's windows alone, unless the setting asks for Sona's", () => {
    expect(resolveWindowChrome(tiling, normal, "linux").kind).toBe("none");
    const sona = resolveWindowChrome(styled("sona", tiling), normal, "linux");
    expect(sona.kind).toBe("drawn");
    expect(sona.end).toEqual(["close"]);
  });

  it("keeps Sona's title bar without buttons, and gives the desktop its own", () => {
    const plain = resolveWindowChrome(styled("sona-no-buttons"), normal, "linux");
    expect(plain).toMatchObject({ kind: "drawn", start: [], end: [], windowMenu: true });
    expect(resolveWindowChrome(styled("desktop"), normal, "linux").kind).toBe("none");
  });

  it("draws Windows' caption buttons on Windows", () => {
    const win = resolveWindowChrome(windowsCapabilities().windowControls, normal, "windows");
    expect(win).toMatchObject({
      kind: "drawn",
      end: ["minimize", "maximize", "close"],
      captions: true,
      windowMenu: false,
    });
  });

  it("keeps room for macOS's own buttons, whatever the setting, until full screen hides them", () => {
    const mac = macCapabilities().windowControls;
    expect(resolveWindowChrome(styled("sona", mac), normal, "darwin")).toEqual({
      kind: "native-inset",
      start: [],
      end: [],
      startRoom: MAC_START_ROOM,
      height: MAC_TITLE_BAR_HEIGHT,
      captions: false,
      windowMenu: false,
    });
    expect(
      resolveWindowChrome(mac, { maximised: false, fullscreen: true }, "darwin"),
    ).toMatchObject({ kind: "native-inset", startRoom: 0, height: TITLE_BAR_HEIGHT });
  });

  it("draws nothing before Go answers, and drops buttons it doesn't know", () => {
    expect(resolveWindowChrome(undefined, normal, undefined).kind).toBe("none");
    const odd = { ...gnome, start: ["help", "close"], end: ["menu", "minimize"] };
    expect(resolveWindowChrome(odd, normal, "linux")).toMatchObject({
      start: ["close"],
      end: ["minimize"],
    });
  });
});
