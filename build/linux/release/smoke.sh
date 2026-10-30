#!/usr/bin/env bash
# Smoke test for one release artifact on a clean distro container; run by
# release.sh with the packages mounted at /pkg.
#
#   smoke.sh deb        # Ubuntu/Debian: install the .deb with its declared deps
#   smoke.sh rpm        # Fedora: install the .rpm with its declared deps
#   smoke.sh appimage   # Fedora: run the AppImage with no GTK/WebKit installed
#   smoke.sh arch       # Arch Linux: install the .pkg.tar.zst with its declared deps
#
# Passes when `sona --version` prints SONA_VERSION (when set), the binary
# resolves every shared library, the GStreamer elements the playback host
# needs exist, and the app, run headless under Xvfb, creates its window and
# the frontend reports its first paint.
set -euo pipefail
kind=$1

fail() {
  echo "smoke($kind): FAIL: $*" >&2
  exit 1
}

case $kind in
deb)
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  # Only the package's declared Depends (no Recommends): proves they suffice.
  apt-get install -y -qq --no-install-recommends /pkg/sona_*.deb >/dev/null
  apt-get install -y -qq --no-install-recommends xvfb xauth dbus gstreamer1.0-tools >/dev/null
  exe=/usr/bin/sona
  ;;
rpm)
  dnf install -y -q --setopt=install_weak_deps=False /pkg/sona-*.rpm
  dnf install -y -q --setopt=install_weak_deps=False xorg-x11-server-Xvfb xorg-x11-xauth dbus-daemon gstreamer1 gstreamer1-plugins-base-tools which
  exe=/usr/bin/sona
  ;;
appimage)
  # Only what any desktop has: X/Xvfb, D-Bus, fonts, and the libraries the
  # AppImage excludelist leaves to the host (fontconfig, EGL/GLES, the
  # Wayland client, fribidi, ALSA). No GTK, WebKit or GStreamer from the
  # distro; the AppImage must bring them.
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
    [[ -f /usr/share/licenses/sona-bin/LICENSE.chromium ]] || fail "licence notice missing"
    cd /
  else
    pacman -U --noconfirm --noprogressbar /pkg/sona-*.pkg.tar.zst >/dev/null
  fi
  exe=/usr/bin/sona
  ;;
*) fail "unknown kind" ;;
esac

if [[ -n ${SONA_VERSION:-} ]]; then
  got=$("$exe" --version) || fail "sona --version failed"
  [[ $got == "sona $SONA_VERSION" ]] || fail "sona --version printed '$got', want 'sona $SONA_VERSION'"
  echo "smoke($kind): $got"
fi

# Every shared library resolves.
if [[ $kind == appimage ]]; then
  missing=$(cd "$root/usr/lib" && LD_LIBRARY_PATH="$root/usr/lib" ldd "$root/usr/bin/sona" | grep 'not found' || true)
else
  missing=$(ldd "$exe" | grep 'not found' || true)
fi
[[ -z $missing ]] || fail "unresolved libraries: $missing"

# GStreamer elements for appsrc ! avdec_aac ! audioconvert ! audioresample ! volume ! <sink>.
if [[ $kind == appimage ]]; then
  # The bundled registry: scan the bundled plugin dir with the bundled scanner.
  for p in app libav audioconvert audioresample volume pulseaudio; do
    [[ -f $root/usr/lib/gstreamer-1.0/libgst$p.so ]] || fail "plugin $p not bundled"
  done
  missing=$(cd "$root/usr/lib" && for f in gstreamer-1.0/*.so; do LD_LIBRARY_PATH="$root/usr/lib" ldd "$f" | grep 'not found' || true; done)
  [[ -z $missing ]] || fail "plugin libraries unresolved: $missing"
else
  for e in appsrc avdec_aac audioconvert audioresample volume; do
    gst-inspect-1.0 "$e" >/dev/null 2>&1 || fail "GStreamer element $e missing"
  done
  gst-inspect-1.0 pipewiresink >/dev/null 2>&1 || gst-inspect-1.0 pulsesink >/dev/null 2>&1 || fail "no PipeWire/Pulse sink"
fi

# Run it headless. WebKit's bubblewrap sandbox cannot start in an
# unprivileged container, so it is disabled for this test only.
export HOME=/tmp/home XDG_RUNTIME_DIR=/tmp/run
mkdir -p "$HOME" "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 LIBGL_ALWAYS_SOFTWARE=1
log=$HOME/.local/state/sona/sona.log
set +e
timeout 40 dbus-run-session -- xvfb-run -a -s "-screen 0 1280x800x24" "$exe" >/tmp/sona.out 2>&1 &
pid=$!
for _ in $(seq 1 35); do
  sleep 1
  grep -q '"startup: first paint"' "$log" 2>/dev/null && break
done
kill "$pid" 2>/dev/null
wait "$pid" 2>/dev/null
set -e

grep -q '"startup: process start"' "$log" 2>/dev/null || { cat /tmp/sona.out; fail "no startup log"; }
grep -q '"startup: window created"' "$log" || { cat /tmp/sona.out "$log"; fail "window not created"; }
grep -q '"startup: first paint"' "$log" || { cat /tmp/sona.out "$log"; fail "frontend never painted"; }
echo "smoke($kind): log excerpt:"
grep -o '"msg":"startup[^"]*"[^}]*' "$log" | head -8
echo "smoke($kind): PASS"
