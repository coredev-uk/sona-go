---
title: Install Sona
description: Install Sona on Linux, macOS or Windows, and keep it up to date.
---

Sona runs on Linux (x86_64), macOS 14 or later (Apple silicon) and Windows (x64). It is in early development: every release so far is a pre-release. You need an Apple Music subscription. Sona is not affiliated with Apple or Google.

Download a file from the [releases page](https://github.com/coredev-uk/sona-go/releases). `SHA256SUMS` lists every file, and each has signed build provenance. To verify one:

```sh
gh attestation verify <file> --repo coredev-uk/sona-go
```

## Install on Linux

1. Download the package for your system: `.deb`, `.rpm`, Arch (`.pkg.tar.zst`) or AppImage.
2. Install it with your package manager, or make the AppImage executable and run it.

The packages pull in GTK3, GStreamer and `libayatana-appindicator3` (the tray icon). Your package manager updates them. The AppImage updates through AppImageUpdate.

On Ubuntu 23.10 and later, prefer the `.deb` to the AppImage. Ubuntu stops programs creating the user namespaces Sona's playback sandbox is built from, unless an AppArmor profile allows them. The `.deb` (and `.rpm`) install that profile, `/etc/apparmor.d/sona`, and nothing else of Sona is restricted by it. An AppImage can't, so on Ubuntu it still plays, but Settings → Diagnostics → Host sandbox says "Not fully sandboxed" and that the playback helper can reach your session bus.

### Install with Nix

The repository is a flake. Follow its `nix` branch:

```sh
nix run github:coredev-uk/sona-go/nix
nix profile add github:coredev-uk/sona-go/nix
```

In a NixOS or Home Manager configuration, set `inputs.sona.url = "github:coredev-uk/sona-go/nix"` and use `overlays.default` (`pkgs.sona`) or `packages.<system>.default`. `nix flake update sona` updates it. Outside NixOS, the GPU driver needs [nixGL](https://github.com/nix-community/nixGL).

The flake has three packages:

- `sona`: the latest release.
- `sona-nightly`: the newest nightly build of `main`, or the release when that is newer.
- `sona-source`: builds your checkout, without Last.fm scrobbling or Discord presence.

## Install on macOS

1. Download `Sona-<version>-macos-arm64.dmg`, open it and drag Sona to Applications.
2. Open Sona. On macOS 15 and later, macOS blocks the first open because the app is not notarised. Open System Settings → Privacy & Security and choose "Open Anyway", or run this once:

   ```sh
   xattr -dr com.apple.quarantine /Applications/Sona.app
   ```

Sona updates itself: Settings → General → Updates → "Install and restart".

## Install on Windows

1. Download `Sona-<version>-windows-x64-setup.exe` and run it. It installs for your user and needs no administrator rights. Or extract `Sona-<version>-windows-x64.zip` anywhere you can write to.
2. If SmartScreen warns about an unknown publisher, choose "More info", then "Run anyway".

Sona updates itself: Settings → General → Updates.

The uninstaller removes Sona's playback and add-on sandboxes and its start-at-login entry. If you delete a portable folder instead:

1. Turn off Settings → General → Start Sona when you log in, if it's on.
2. Quit Sona and run `sona.exe --remove-sandbox` from that folder.

Your data stays. A portable folder you move keeps working; turn Start Sona when you log in on again from the new place.

## Next

[Sign in and finish the first-run setup](/guide/first-run/).
