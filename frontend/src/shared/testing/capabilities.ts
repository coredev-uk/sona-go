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
  };
}
