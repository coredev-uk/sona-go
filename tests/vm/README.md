# Desktop VM tests

NixOS VMs (QEMU with KVM, headless) that log a user into a real desktop
session, start Sona built from this checkout, and check what the desktop
rows of `docs/release-checklist.md` ask. The VMs have no network
(`virtualisation.restrictNetwork`): no Apple, no sign-in, no playback, no
CDM download. Every Sona starts in private mode (`SONA_PRIVATE=1` in the
session).

## Running

Linux with KVM and Nix (`system-features` including `kvm nixos-test`); no
devenv needed. nixpkgs is the revision `devenv.lock` pins.

```sh
nix-build tests/vm -A gnome          # GNOME 50 (Wayland)
nix-build tests/vm -A plasma         # KDE Plasma 6.8 (Wayland)
nix-build tests/vm -A all            # both
nix-build tests/vm -A sona           # only the package the VMs install
# The release AppImage (build it with `release` first):
nix-build tests/vm -A appimage --arg appimage ./bin/release/sona-<version>-x86_64.AppImage
```

Each test writes its screenshots to `result/`. A failed build keeps none;
to keep them, run the driver outside the sandbox:

```sh
$(nix-build tests/vm -A plasma.driver)/bin/nixos-test-driver --output_directory /some/dir
$(nix-build tests/vm -A gnome.driverInteractive)/bin/nixos-test-driver   # Python shell: start_all(), machine.screenshot("x"), …
```

A run takes 3–5 minutes (GNOME) and 7–15 minutes (Plasma) on a warm
store. CI runs them only when started by hand: `.github/workflows/vm-tests.yml`
(`workflow_dispatch`). A run takes about an hour of runner time, so it is
meant for major releases; run them locally for everything else.

### Updating the package

`sona.nix` builds the frontend (pnpm, `fetchPnpmDeps`) and Sona
(`buildGoModule`, tags `production,sqlite_fts5`, VERSION as `main.version`,
the committed `frontend/bindings`) and installs the desktop entry, icons and
licence notices as nfpm does. After a change to `go.sum` or
`frontend/pnpm-lock.yaml`, set the hash it names to an empty string, run
`nix-build tests/vm -A sona`, and copy the hash the error prints. Only
tracked files are built: `git add` new files first.

## What runs

| File | |
| ---- | - |
| `default.nix` | Entry point: the package, the tests, the AppImage test |
| `sona.nix` | Sona as a Nix package |
| `common.nix`, `common.py` | The VM (user alice, autologin, 2560×1600 screen, offline, a saved queue) and the shared checks |
| `gnome-session.nix`, `gnome.py`, `gnome.nix` | GNOME with the AppIndicator extension; Shell state read through `org.gnome.Shell.Eval` (unsafe mode) |
| `plasma.nix` | Plasma |
| `appimage.nix` | The release AppImage under `appimage-run` in GNOME |
| `vmcheck.py` | Reads Sona's windows over AT-SPI: frames, the title bar's buttons and their positions, the page's headings |
| `notifyprobe/` | Sona's notification client (`internal/desktop/notify`, Sona's options) showing two track notifications |

A saved queue (`queue.json`, one track, "VM Queue Song") gives Sona a track
to restore paused, so MPRIS, the tray menu and the shells' media widgets
have something to show without playback.

### Checks and checklist rows

| Row | GNOME | Plasma |
| --- | ----- | ------ |
| D1 package | `sona --version` from the Nix package | same |
| D2 launcher | Shell's app system finds `dev.coredev.Sona.desktop` (name Sona, icon `sona`) and starts it; the window belongs to that app | Kickoff search for "Sona" shows it ("Music Player"), Enter starts it; KWin's window class is `dev.coredev.Sona` |
| First paint | log `startup: first paint`; AT-SPI shows the page; screenshot | same |
| Title bar | Close only, at the right (`appmenu:close`); `close,minimize,maximize:` puts all three at the left | Minimise, Maximise, Close at the right (KWin's default); kwinrc `X` / `IA` puts Close left, the others right |
| Offline | the CDM update check fails with `code=network`; Sona keeps running | same |
| `sona private status` | `private listening: on (forced by SONA_PRIVATE=1)` | same |
| D5 MPRIS | one `org.mpris.MediaPlayer2.sona`; `playerctl` title, status, `DesktopEntry`; the calendar panel's media card shows the track (OCR) | same bus checks; the tray's media player icon appears (screenshot only: opening Plasma's media popup needs a pointer click, so the widget's contents stay a manual check) |
| D6 notifications | through Sona's client: the shell's notification service owns the name, holds one Sona notification (the second replaced the first), shows it in the notification list (OCR), and a click (`activate()`) reaches the client as `default` | Plasma owns the name and pops the notification up (OCR) |
| D7 tray | AppIndicator extension active; Sona's item (Title "Sona", menu with Show Sona, Mini player, Quit Sona) is in the panel | Sona's item is registered with Plasma's watcher, same menu |
| D8 single instance | `sona music://…` exits at once, logs `forwarded to the running instance`; one process, one window, one MPRIS name | same |
| D10 scaling | scale 200 % (`gdctl`): the window fits the 1280×800 logical screen; text sharp (screenshot) | 200 % at start, then 150 % live (`kscreen-doctor`); screenshots |
| D11 X11 | `GDK_BACKEND=x11` on XWayland: an X11 client (Mutter), in `xwininfo`; title bar, MPRIS, tray, single instance again | same on KWin's XWayland |
| Quit | MPRIS `Quit`; `shutdown complete` | same |

### What the VMs can't show

- **Native X11 sessions.** nixpkgs ships GNOME 50 and Plasma 6.8, both
  Wayland-only; D11 runs on XWayland instead.
- **Sona's own track notification** needs playback. The probe sends what
  Sona sends through the same client, so the server side is real, but the
  app's "announce on track change, not while focused" logic is not
  exercised here. GNOME shows Sona's notifications (low urgency) only in
  the notification list, without a banner.
- **Playback, sign-in, the CDM** (D3, D4, D12–D14, D16–D18): no network.
- **D9** (`music://` from a browser), **D15** (uninstall: NixOS installs
  declaratively), mixed-scale monitors, Orca speech, a physical mouse and
  real file-chooser picks stay manual.
- **Release packages.** The .deb, .rpm and Arch packages don't install on
  NixOS; `build/linux/release/release.sh` smoke-tests them on their
  distributions. The AppImage runs under `appimage-run`, which needs extra
  host libraries (`appimage.nix`), and WebKit's own bubblewrap sandbox
  can't start inside appimage-run's, so that test turns WebKit's sandbox
  off.
