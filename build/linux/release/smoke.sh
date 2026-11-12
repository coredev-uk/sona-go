#!/usr/bin/env bash
# Smoke test for one release artifact on a clean distro container; run by
# release.sh with the packages mounted at /pkg.
#
#   smoke.sh deb        # Ubuntu/Debian: install the .deb with its declared deps
#   smoke.sh rpm        # Fedora: install the .rpm with its declared deps
#   smoke.sh appimage   # Fedora: run the AppImage with no GTK installed
#   smoke.sh arch       # Arch Linux: install the .pkg.tar.zst with its declared deps
#
# Passes when `sona --version` prints SONA_VERSION (when set), `sona status`
# says Sona isn't running (exit status 3) and the packages carry the
# command's completions and manual page, the binary
# resolves every shared library, the GStreamer elements the playback host
# needs exist, and the app, run headless under Xvfb, creates its window and
# logs its first paint (MyGo's CPU renderer, MYGO_GPU=0).
#
# MyGo loads GTK3, libepoxy, EGL, the tray, AT-SPI, libsecret and XTest by
# soname (purego), so ldd never lists them: the smoke test checks each one
# resolves, from the package's declared dependencies or from the AppImage.
set -euo pipefail
kind=$1
smoke_name="smoke($kind)"
# build/lib/smoke.sh, which release.sh mounts beside this script.
# shellcheck source=../../lib/smoke.sh
. "$(dirname "$0")/smoke-lib.sh"

case $kind in
deb)
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  # Only the package's declared Depends (no Recommends): proves they suffice.
  apt-get install -y -qq --no-install-recommends /pkg/sona_*.deb >/dev/null
  apt-get install -y -qq --no-install-recommends xvfb xauth dbus gstreamer1.0-tools >/dev/null
  exe=/usr/bin/sona
  contents() { dpkg-deb -c /pkg/sona_*.deb | awk '{print $6}' | sed 's|^\./|/|'; }
  ;;
rpm)
  dnf install -y -q --setopt=install_weak_deps=False /pkg/sona-*.rpm
  dnf install -y -q --setopt=install_weak_deps=False xorg-x11-server-Xvfb xorg-x11-xauth dbus-daemon gstreamer1 gstreamer1-plugins-base-tools which
  exe=/usr/bin/sona
  contents() { rpm -qlp /pkg/sona-*.rpm; }
  ;;
appimage)
  # Only what any desktop has: X/Xvfb, D-Bus, fonts, and the libraries the
  # AppImage excludelist leaves to the host (fontconfig, EGL/GLES, the
  # Wayland client, fribidi, ALSA). No GTK or GStreamer from the distro; the
  # AppImage must bring them.
  dnf install -y -q --setopt=install_weak_deps=False xorg-x11-server-Xvfb xorg-x11-xauth dbus-daemon which findutils file \
    fontconfig dejavu-sans-fonts libwayland-client libglvnd-egl libglvnd-gles mesa-libEGL fribidi alsa-lib
  cp /pkg/sona-*.AppImage /tmp/sona.AppImage
  chmod +x /tmp/sona.AppImage
  (cd /tmp && ./sona.AppImage --appimage-extract >/dev/null)
  root=/tmp/squashfs-root
  find "$root" -iname '*widevine*' | grep -q . && fail "AppImage contains a Widevine CDM"
  exe=$root/AppRun
  ;;
arch)
  pacman -Syu --noconfirm --noprogressbar >/dev/null
  pacman -S --noconfirm --noprogressbar --needed base-devel xorg-server-xvfb xorg-xauth dbus which >/dev/null
  if [[ -f /pkg/aur/PKGBUILD ]]; then
    # The AUR sona-bin route: makepkg (as a user) repackages the release's
    # Arch package, with the release file standing in for the download.
    useradd -m builder
    install -d -o builder /tmp/aur
    install -o builder /pkg/aur/PKGBUILD /pkg/aur/.SRCINFO /pkg/sona-*.pkg.tar.zst /tmp/aur/
    cd /tmp/aur
    runuser -u builder -- makepkg --printsrcinfo | diff -u .SRCINFO - || fail ".SRCINFO is not makepkg's"
    # --nodeps: the builder cannot install packages; repackaging needs none
    # of the runtime depends, and pacman -U below resolves them from the repos.
    runuser -u builder -- makepkg --nodeps --noconfirm >/dev/null || fail "makepkg"
    pacman -U --noconfirm --noprogressbar /tmp/aur/sona-bin-*.pkg.tar.zst >/dev/null
    pkgfile=$(echo /tmp/aur/sona-bin-*.pkg.tar.zst)
    [[ -f /usr/share/licenses/sona-bin/LICENSE.chromium ]] || fail "licence notice missing"
    cd /
  else
    pacman -U --noconfirm --noprogressbar /pkg/sona-*.pkg.tar.zst >/dev/null
    pkgfile=$(echo /pkg/sona-*.pkg.tar.zst)
  fi
  exe=/usr/bin/sona
  contents() { bsdtar -tf "$pkgfile" | sed 's|^|/|'; }
  ;;
*) fail "unknown kind" ;;
esac

if [[ -n ${SONA_VERSION:-} ]]; then
  check_version "$SONA_VERSION" "$exe"
fi

# The command line: no Sona runs here (no session bus either), so status
# says so with exit status 3.
rc=0
out=$("$exe" status 2>&1) || rc=$?
[[ $rc == 3 && $out == "Sona isn't running" ]] || fail "sona status printed '$out' (exit $rc), want 'Sona isn't running' (exit 3)"
"$exe" help >/dev/null || fail "sona help failed"
# The completions and the manual page are in the package. Checked in its
# file list, not on disk: the distro images drop documentation on install
# (Ubuntu's dpkg path-exclude for /usr/share/man, Fedora's nodocs, Arch's
# NoExtract), as a desktop install does not.
if [[ $kind != appimage ]]; then
  list=$(contents)
  for f in /usr/share/bash-completion/completions/sona /usr/share/zsh/site-functions/_sona \
    /usr/share/fish/vendor_completions.d/sona.fish /usr/share/man/man1/sona.1.gz; do
    grep -qxF "$f" <<<"$list" || fail "$f not in the package"
  done
fi

# Every shared library resolves.
if [[ $kind == appimage ]]; then
  missing=$(cd "$root/usr/lib" && LD_LIBRARY_PATH="$root/usr/lib" ldd "$root/usr/bin/sona" | grep 'not found' || true)
else
  missing=$(ldd "$exe" | grep 'not found' || true)
fi
[[ -z $missing ]] || fail "unresolved libraries: $missing"

# The libraries MyGo loads by soname: each must resolve on this clean
# system (installed by the package's dependencies) or be bundled.
sonames=(libgtk-3.so.0 libgdk-3.so.0 libepoxy.so.0 libayatana-appindicator3.so.1
  libatk-1.0.so.0 libatk-bridge-2.0.so.0 libsecret-1.so.0 libXtst.so.6)
if [[ $kind == appimage ]]; then
  for so in "${sonames[@]}"; do
    [[ -e $root/usr/lib/$so ]] || fail "$so not bundled"
  done
  find "$root" \( -iname 'libwebkit*' -o -iname 'libgtk-4*' -o -iname 'libsoup*' \) | grep -q . && fail "AppImage bundles WebKitGTK, GTK4 or libsoup"
else
  sonames+=(libEGL.so.1)
  ldconfig # a container may not have run the distro's cache hook after the install
  # Captured first: `grep -q` closing the pipe early makes ldconfig die of
  # SIGPIPE, which pipefail reports as a missing library.
  cache=$(ldconfig -p)
  for so in "${sonames[@]}"; do
    grep -qF "$so" <<<"$cache" || fail "$so not installed by the package's dependencies"
  done
  # The package has no GTK4, WebKitGTK or libsoup 3 dependency.
  if ldd "$exe" | grep -qE 'libgtk-4|libwebkit|libsoup'; then fail "binary links GTK4, WebKitGTK or libsoup"; fi
fi

# GStreamer elements for appsrc ! avdec_aac ! audioconvert ! audioresample ! volume ! <sink>,
# and what Sona's motion artwork pipeline needs: playbin3/decodebin3
# (playback), typefind, qtdemux (isomp4), h264parse (videoparsersbad),
# videoconvert (videoconvertscale), videoflip (videofilter), avdec_h264 (libav).
if [[ $kind == appimage ]]; then
  # The bundled registry: scan the bundled plugin dir with the bundled scanner.
  for p in app libav audioconvert audioresample volume pulseaudio \
    playback typefindfunctions isomp4 videoparsersbad videoconvertscale videofilter; do
    [[ -f $root/usr/lib/gstreamer-1.0/libgst$p.so ]] || fail "plugin $p not bundled"
  done
  missing=$(cd "$root/usr/lib" && for f in gstreamer-1.0/*.so; do LD_LIBRARY_PATH="$root/usr/lib" ldd "$f" | grep 'not found' || true; done)
  [[ -z $missing ]] || fail "plugin libraries unresolved: $missing"
else
  for e in appsrc avdec_aac audioconvert audioresample volume \
    playbin3 decodebin3 qtdemux h264parse videoconvert videoflip; do
    gst-inspect-1.0 "$e" >/dev/null 2>&1 || fail "GStreamer element $e missing"
  done
  # Fedora's gst-libav is built on ffmpeg-free, which has no H.264 decoder:
  # motion artwork needs RPM Fusion's there (Diagnostics says so).
  if gst-inspect-1.0 avdec_h264 >/dev/null 2>&1; then
    echo "smoke($kind): avdec_h264 present"
  elif [[ $kind == rpm ]]; then
    echo "smoke($kind): no avdec_h264 (ffmpeg-free); motion artwork needs RPM Fusion's gst-libav"
  else
    fail "GStreamer element avdec_h264 missing"
  fi
  gst-inspect-1.0 pipewiresink >/dev/null 2>&1 || gst-inspect-1.0 pulsesink >/dev/null 2>&1 || fail "no PipeWire/Pulse sink"
fi

# Run it headless under Xvfb: MyGo's CPU renderer (MYGO_GPU=0), as no GPU
# is here, and private mode so nothing reaches Apple's servers or a
# scrobbler. The log path is the same on every platform: ~/.local/state.
export HOME=/tmp/home XDG_RUNTIME_DIR=/tmp/run
mkdir -p "$HOME" "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
export MYGO_GPU=0 LIBGL_ALWAYS_SOFTWARE=1 SONA_PRIVATE=1
log=$HOME/.local/state/sona/sona.log
timeout 40 dbus-run-session -- xvfb-run -a -s "-screen 0 1280x800x24" "$exe" >/tmp/sona.out 2>&1 &
pid=$!
wait_for_log "$log" '"startup: first paint"' 35 "$pid" || true
stop_app "$pid"

grep -q '"startup: process start"' "$log" 2>/dev/null || { cat /tmp/sona.out; fail "no startup log"; }
grep -q '"startup: window created"' "$log" || { cat /tmp/sona.out "$log"; fail "window not created"; }
grep -q '"startup: first paint"' "$log" || { cat /tmp/sona.out "$log"; fail "no first paint"; }
echo "smoke($kind): log excerpt:"
grep -o '"msg":"startup[^"]*"[^}]*' "$log" | head -8
echo "smoke($kind): PASS"
