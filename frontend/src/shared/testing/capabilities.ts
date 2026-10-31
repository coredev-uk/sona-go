import type { Capabilities } from "@bindings/services";

/** PlatformService.Capabilities on Linux today: everything Sona does works. */
export function linuxCapabilities(): Capabilities {
  return {
    os: "linux",
    playback: {
      available: true,
      engine: "cdm",
      volume: true,
      equalizer: true,
      soundCheck: true,
      crossfade: true,
      outputDevice: true,
      privateListening: true,
    },
    mediaControls: true,
    notifications: true,
    tray: true,
    autostart: true,
    sleepInhibit: true,
    powerState: true,
    fileChooser: true,
    cli: true,
    discordPresence: true,
    webkitGtk: true,
    // GNOME with its default button layout ("appmenu:close").
    windowControls: {
      native: "frame",
      automatic: "sona",
      desktop: "GNOME",
      style: "auto",
      start: [],
      end: ["close"],
      windowMenu: true,
    },
  };
}

/** A platform with no playback engine and no desktop services: every capability false. */
export function bareCapabilities(os = "darwin"): Capabilities {
  return {
    os,
    playback: {
      available: false,
      engine: "",
      volume: false,
      equalizer: false,
      soundCheck: false,
      crossfade: false,
      outputDevice: false,
      privateListening: false,
    },
    mediaControls: false,
    notifications: false,
    tray: false,
    autostart: false,
    sleepInhibit: false,
    powerState: false,
    fileChooser: false,
    cli: false,
    discordPresence: false,
    webkitGtk: false,
    windowControls: {
      native: "frame",
      automatic: "desktop",
      desktop: "",
      style: "auto",
      start: [],
      end: [],
      windowMenu: false,
    },
  };
}

/**
 * macOS (a development build): the CDM engine without the output device
 * choice, the tray, and no other desktop services.
 */
export function macCapabilities(): Capabilities {
  return {
    ...bareCapabilities("darwin"),
    playback: { ...linuxCapabilities().playback, outputDevice: false },
    tray: true,
    discordPresence: true,
    windowControls: {
      native: "inset",
      automatic: "desktop",
      desktop: "",
      style: "auto",
      start: ["close", "minimize", "maximize"],
      end: [],
      windowMenu: false,
    },
  };
}

/** Windows: no playback engine yet, the tray, and Sona's title bar with Windows' caption buttons. */
export function windowsCapabilities(): Capabilities {
  return {
    ...bareCapabilities("windows"),
    tray: true,
    windowControls: {
      native: "frame",
      automatic: "sona",
      desktop: "",
      style: "auto",
      start: [],
      end: ["minimize", "maximize", "close"],
      windowMenu: false,
    },
  };
}
