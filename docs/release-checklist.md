# Release checklist

Run before tagging `v<VERSION>`. Automated gates come first; the desktop
matrix needs real sessions (GNOME and KDE in VMs, Hyprland on the dev host).
Record each run in the results table at the end: date, Sona version,
distro/session, result, and notes for anything not passed.

## 1. Automated gates

- [ ] `VERSION` holds the new version; nothing else carries it
      (`build/linux/Taskfile.yml` injects it into the binary and nfpm).
- [ ] CI is green on the commit to tag (`.github/workflows/ci.yml`: Go vet +
      `go test -race` on Ubuntu 24.04 with apt GStreamer and user namespaces
      allowed, so the CDM host sandbox tests run; frontend tsc, eslint,
      prettier, vitest, build).
- [ ] Locally in devenv: `go test -race ./internal/playback/...` with the real
      CDM (`SONA_WIDEVINE_CDM` set) and `go run ./cmd/playspike -seek 90`.
- [ ] `bash build/linux/release/release.sh` passes: builds .deb, .rpm, Arch
      package, AppImage + .zsync, the AUR files and SHA256SUMS, and
      smoke-tests the .deb (Ubuntu 24.04), .rpm and AppImage (Fedora 45) and
      the Arch package through the AUR PKGBUILD (Arch Linux): `sona --version`
      prints the version, every library resolves, the GStreamer elements
      exist, and the app paints its first frame under Xvfb.
- [ ] The tag workflow (`.github/workflows/release.yml`) re-runs the same and
      publishes the GitHub release. Afterwards copy `aur/PKGBUILD` and
      `aur/.SRCINFO` from `sona-bin-<v>-aur.tar.gz` into the AUR `sona-bin`
      repo and push there by hand.

## 2. Desktop matrix

Install the package for that distro on a clean VM (or the dev host for
Hyprland), sign in, and play something. Each check names how to verify it.

| #   | Check | How |
| --- | ----- | --- |
| D1  | Package installs with only its declared dependencies | `.deb` on Ubuntu 24.04 GNOME (`sudo apt install ./sona_*.deb`), `.rpm` on Fedora KDE (`sudo dnf install ./sona-*.rpm`), Arch package or `sona-bin` on Arch; AppImage on any. `sona --version` prints the version |
| D2  | Launcher entry and icon | Sona appears in the app grid / launcher with its icon; the running window groups under it (app id `dev.coredev.Sona`) |
| D3  | CDM detected | Settings → Diagnostics shows the CDM version from the user's Google Chrome; with Chrome absent the playback area says so plainly |
| D4  | Full-track playback | Play an album track to the end; seek; next/previous; audio on the default PipeWire/Pulse output |
| D5  | MPRIS in the shell's media widget | GNOME: the media card in the notification/calendar panel shows title, artist, artwork and working play/pause/next/previous. KDE: the Media Player plasmoid / system-tray media controls do the same. Hyprland: `playerctl -p sona metadata`, bar widget. Media keys work |
| D6  | Notifications | Track-change notification appears with artwork (Settings → Desktop → Notifications on); none while the window is focused if that is the setting |
| D16 | Sign-in flows | Native sign-in with 2FA; loopback and token-paste fallbacks reachable |
| D17 | Large-library sync | An account with a large library syncs fully; lists scroll smoothly; restart shows it from cache |
| D18 | Queue, stations, live radio | Queue add/reorder/remove; start a song station and a live radio station; both play and advance |
| D7  | Tray | GNOME needs the AppIndicator extension (state if absent: no tray, close quits or minimises per setting). KDE: tray icon, menu Play/Next/Previous/Show/Mini player/Quit work. Hyprland: in the bar's tray (StatusNotifierWatcher) |
| D8  | Single instance across two real launches | Launch from the launcher, then launch again (launcher and `sona music://music.apple.com/…` in a terminal): the second exits at once, the first window comes forward and opens the link; still one window and one `org.mpris.MediaPlayer2.sona` |
| D9  | `music://` links | Clicking a `music://` / `itms://` link in a browser opens it in Sona |
| D10 | Wayland scaling | Native Wayland session at 100 %, 150 % (fractional) and 200 %: text and artwork sharp, no blurry upscaling, layout fits; move between monitors with different scales |
| D11 | X11 session | GNOME on Xorg / Plasma X11 (or `GDK_BACKEND=x11` under XWayland): window, playback, MPRIS, tray, single instance all work |
| D12 | Secrets | Token stored in GNOME Keyring / KWallet (Settings shows Secret Service); sign out removes it; restart stays signed in |
| D13 | Sleep inhibit | While playing, the session does not suspend on idle (`systemd-inhibit --list` shows Sona); paused, it does |
| D14 | Restart persistence | Quit and relaunch: signed in, library from cache before any network, last route/queue restored per settings |
| D15 | Uninstall | Removing the package leaves no launcher entry; user data stays in XDG dirs |

## 3. Results

GNOME and KDE rows need VMs (not available on the dev host, a NixOS
Hyprland machine). They are open until someone runs them.

| Date | Version | Distro / session | Checks | Result | Notes |
| ---- | ------- | ---------------- | ------ | ------ | ----- |
| 2026-10-30 | 0.1.0 | Ubuntu 24.04, Fedora 45, Arch (containers, Xvfb) | §1 release smoke, D1 (install + `--version`) | pass | `release.sh` on a clean clone: deb, rpm, AppImage, Arch via AUR PKGBUILD all PASS (first paint 689–971 ms under Xvfb) |
| 2026-10-30 | 0.1.0 | NixOS, Hyprland 0.56.2 (Wayland), devenv build | D5 (MPRIS bus), D7, D8, D11 (XWayland), D10 (1.5× in headless sway) | pass | See evidence below |
| 2026-10-30 | 0.1.0 | NixOS, Hyprland 0.56.2 + Quickshell, devenv build (Integrations' live run) | D6, D13 | pass | Track-change notification with artwork, title and "artist — album"; the next track replaces it; none while Sona's window is focused; its Next button advanced the track. `systemd-inhibit --list` shows "Sona … sleep Playing music block" only while playing (gone after pause and after quit) |
| 2026-10-30 | 0.1.0 | NixOS, Hyprland | D2–D4, D9, D12, D14–D18 | open | Need an installed package and a hands-on signed-in session; not run by the automated pass |
| — | — | Ubuntu 24.04 GNOME (Wayland + Xorg) VM | all of §2 | open | No VM available on the dev host |
| — | — | Fedora / Kubuntu KDE Plasma 6 (Wayland + X11) VM | all of §2 | open | No VM available on the dev host |

### Evidence: Hyprland, 2026-10-30

Binary: devenv build of a clean clone (`go tool wails3 build`, `sona 0.1.0`),
run with private XDG directories on the real session bus and compositor.

- D8 single instance: first launch painted at 1002 ms; second launch
  `sona music://music.apple.com/us/album/1440857781` exited 0 after 0.13 s,
  log `"forwarded to the running instance"`; still one `dev.coredev.Sona`
  client, which became the active window; after quitting, neither
  `dev.coredev.Sona` nor `org.mpris.MediaPlayer2.sona` stays on the bus.
- D5 MPRIS (bus side): `playerctl -l` lists `sona`; `Identity` "Sona",
  `DesktopEntry` "dev.coredev.Sona", `CanRaise` true, status `Stopped`. The
  bar's media widget with real playback is part of the open row.
- D7 tray (working-tree build, which has the tray; the clone predates it):
  Sona registers `org.kde.StatusNotifierItem-<pid>-1` with the
  bar's `org.kde.StatusNotifierWatcher` (Quickshell); Title "Sona"; menu
  labels "Not playing, Play, Next, Previous, Show Sona, Mini player, Quit
  Sona". Closing the window (`hl.dsp.window.close`) logs "shell: window
  closed to the background": the client disappears and the process stays.
  The item's `Activate` brings the window back (mapped). Clicking menu
  entries by hand is part of the open row.
- D11 X11: `GDK_BACKEND=x11` → Hyprland client `xwayland: true`, first paint
  709 ms, `playerctl -l` lists `sona`. A native Xorg session is still to do
  in the VMs.
- D10 scaling: headless sway output at scale 1.5 (1920×1080): first paint
  1578 ms, window 1276×692 logical, screenshot text and icons sharp. The dev
  host's monitors are at scale 1, so mixed-scale monitor moves are open.

## 4. Known issues

Upstream bugs Sona ships with, tracked here until they are reported and
fixed upstream. Re-check each one on every release.

### Blank mini player on sway (WebKitGTK second web view)

- **Symptom:** on sway, the mini player window opens at its size with its
  title, but shows only the background colour. Its page loads (the buttons
  are in the DOM and respond), but `innerWidth` and `innerHeight` stay 0.
  The main window is fine. On Hyprland the mini player renders.
- **Versions:** WebKitGTK 2.54.1 (ABI 6.0), GTK 4.22.4, sway 1.12; Wails
  v3.0.0 is not involved (see the repro).
- **Repro without Sona or Wails:** `two.c`, about 70 lines (re-create it
  from this description): a
  `GtkApplication` with two `GtkApplicationWindow`s, each holding one
  `WebKitWebView` (hexpand/vexpand in a `GtkBox`). Each loads an HTML page
  that writes `innerWidth`x`innerHeight` into `document.title` every 500 ms;
  `notify::title` prints it. The second window is created 3 s after
  `activate`. Build it with
  `cc two.c $(pkg-config --cflags --libs webkitgtk-6.0 gtk4)`.
- **Results:** on sway (headless with the pixman or GLES renderer, and
  sway nested in Hyprland on the real GPU), the first view reports its size
  and the second reports `0x0`, every run, while GTK has allocated and
  mapped its widget (for example 638×1379). This holds whether the first
  window is shown, hidden, or never shown. A process with a single web
  view created after 3 s gets its size. On Hyprland 0.56.2 both views get
  their size.
- **Tried, no effect:** creating the window hidden, shown, or before the
  app runs; loading the page after the window is mapped; hiding and
  re-showing it; `related-view`; a separate `WebKitWebContext`; GPU policy
  never; `WEBKIT_DISABLE_DMABUF_RENDERER=1`;
  `WEBKIT_DISABLE_COMPOSITING_MODE=1`; `GSK_RENDERER=cairo`;
  `GDK_SCALE=1`; `GDK_DISABLE=fractional-scale`.
- **Next step:** report it to WebKitGTK (bugs.webkit.org, WebKitGTK
  component) with the repro. Mention sway too, since it does not happen on
  Hyprland. The only Sona-side workaround would be to drop the second web
  view, with the main window turning into the mini player. That was
  decided against.
