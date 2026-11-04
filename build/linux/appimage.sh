#!/usr/bin/env bash
# Builds Sona's AppImage from a release binary.
#
#   build/linux/appimage.sh <binary> <output-dir> <version>
#
# MyGo loads GTK3, libepoxy, the tray's appindicator, AT-SPI, libsecret and
# X11's XTest by soname at run time (purego), so linuxdeploy cannot see them
# in the binary: this script names each one (--library) and linuxdeploy
# bundles it with its dependencies. linuxdeploy's GTK plugin adds what GTK3
# reads from disk (GSettings schemas, pixbuf loaders, input methods). The
# GStreamer plugins the playback host uses are dlopened too: they are copied
# in, GStreamer is pointed at the bundled copies only, and their own
# libraries (libavcodec, libpulse, ...) are deployed. No WebKitGTK, GTK4 or
# libsoup. EGL/GL stay the host's (libglvnd, the GPU driver).
#
# Audio output inside the AppImage is pulsesink, which talks to PipeWire
# through pipewire-pulse or to PulseAudio. pipewiresink is not bundled: a
# private libpipewire would have to match the host's PipeWire modules.
#
# libwidevinecdm.so is never bundled: Sona downloads it from Google into its
# data directory at runtime (internal/playback/cdm). The build fails if one
# appears in the AppDir.
#
# Run it on the release baseline (build/linux/release/Containerfile), not on
# NixOS: it bundles the build machine's libraries. Needs network on first
# use (linuxdeploy, its GTK and AppImage plugins).
set -euo pipefail

bin=$(realpath "$1")
out=$(realpath -m "$2")
version=$3
root=$(cd "$(dirname "$0")/../.." && pwd)
work="$root/build/linux/appimage" # gitignored scratch
arch=x86_64
appdir="$work/Sona.AppDir"
tools="$work/tools"

rm -rf "$work"
mkdir -p "$tools" "$appdir/usr/bin" "$appdir/usr/share/applications" "$out"

# linuxdeploy, its AppImage plugin (must sit next to it) and the GTK plugin.
dl() { curl -fsSL --retry 3 -o "$tools/$2" "$1" && chmod +x "$tools/$2"; }
dl "https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-$arch.AppImage" "linuxdeploy-$arch.AppImage"
dl "https://github.com/linuxdeploy/linuxdeploy-plugin-appimage/releases/download/continuous/linuxdeploy-plugin-appimage-$arch.AppImage" "linuxdeploy-plugin-appimage-$arch.AppImage"
dl "https://raw.githubusercontent.com/linuxdeploy/linuxdeploy-plugin-gtk/master/linuxdeploy-plugin-gtk.sh" "linuxdeploy-plugin-gtk.sh"
export APPIMAGE_EXTRACT_AND_RUN=1 # no FUSE in containers

cp "$bin" "$appdir/usr/bin/sona"
cp "$root/build/linux/dev.coredev.Sona.desktop" "$appdir/usr/share/applications/"
# Icons at every size, for desktop integration (appimaged, AppImageLauncher).
mkdir -p "$appdir/usr/share/icons"
cp -r "$root/build/linux/icons/hicolor" "$appdir/usr/share/icons/"

# The sonames MyGo loads (it is the list in mygo's Linux backends: GTK3 and
# its stack, GL loader, tray, accessibility, Secret Service, X11 placement).
sonames=(
  libgtk-3.so.0 libgdk-3.so.0 libepoxy.so.0
  libayatana-appindicator3.so.1 libdbusmenu-gtk3.so.4
  libatk-1.0.so.0 libatk-bridge-2.0.so.0 libsecret-1.so.0
  libXtst.so.6 libgdk_pixbuf-2.0.so.0 libpangocairo-1.0.so.0 libpangoft2-1.0.so.0
)
libs=()
for so in "${sonames[@]}"; do
  path=$(ldconfig -p | awk -v so="$so" '$1 == so && /x86-64/ {print $NF; exit}')
  [[ -n $path ]] || { echo "appimage.sh: $so not found on the build machine" >&2; exit 1; }
  libs+=(--library "$path")
done

# GStreamer: appsrc ! avdec_aac ! audioconvert ! audioresample ! volume ! pulsesink
# (autoaudiosink as the fallback sink), plus the registry scanner. Motion
# artwork decodes through Sona's own pipeline: playbin3/decodebin3
# (playback), typefind (typefindfunctions), qtdemux (isomp4), h264parse
# (videoparsersbad), videoconvert (videoconvertscale), videoflip
# (videofilter) and avdec_h264 (libav). The bundled set is all GStreamer the
# AppImage sees.
gst_plugins=$(pkg-config --variable=pluginsdir gstreamer-1.0)
gst_scanner="$(pkg-config --variable=pluginscannerdir gstreamer-1.0)/gst-plugin-scanner"
mkdir -p "$appdir/usr/lib/gstreamer-1.0" "$appdir/usr/libexec/gstreamer-1.0"
for p in coreelements app audioconvert audioresample volume autodetect libav pulseaudio \
  playback typefindfunctions isomp4 videoparsersbad videoconvertscale videofilter; do
  cp "$gst_plugins/libgst$p.so" "$appdir/usr/lib/gstreamer-1.0/"
done
cp "$gst_scanner" "$appdir/usr/libexec/gstreamer-1.0/"

# Our own AppRun: the bundled libraries first, GStreamer pointed at the
# bundled plugins only (with a registry of their own so the host's is never
# mixed in), then the GTK plugin's hook (schemas, pixbuf loaders, IM
# modules), then Sona.
cat >"$appdir/AppRun" <<'RUN'
#!/bin/sh
here=$(dirname "$(readlink -f "$0")")
usr="$here/usr"
export APPDIR="$here"
export LD_LIBRARY_PATH="$usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export GST_PLUGIN_SYSTEM_PATH_1_0="$usr/lib/gstreamer-1.0"
export GST_PLUGIN_PATH_1_0=""
export GST_PLUGIN_SCANNER_1_0="$usr/libexec/gstreamer-1.0/gst-plugin-scanner"
export GST_REGISTRY_1_0="${XDG_CACHE_HOME:-$HOME/.cache}/sona/gstreamer-registry-appimage.bin"
for hook in "$here"/apprun-hooks/*.sh; do
  [ -f "$hook" ] && . "$hook"
done
exec "$usr/bin/sona" "$@"
RUN
chmod +x "$appdir/AppRun"

# Licence notices (as in the deb/rpm/Arch packages).
mkdir -p "$appdir/usr/share/doc/sona"
cp "$root/LICENSE" "$root/internal/playback/widevine/LICENSE.chromium" "$appdir/usr/share/doc/sona/"

# Deploy: the dlopened libraries, the GStreamer plugins' and scanner's
# dependencies, then the GTK plugin; repack with update information, so
# AppImageUpdate can fetch the newest GitHub release (the plugin writes
# <name>.zsync next to the AppImage, a release asset alongside it).
deps=()
for f in "$appdir"/usr/lib/gstreamer-1.0/*.so "$appdir/usr/libexec/gstreamer-1.0/gst-plugin-scanner"; do
  deps+=(--deploy-deps-only "$f")
done
name="sona-$version-$arch.AppImage"
(
  cd "$work"
  NO_STRIP=1 OUTPUT="$name" DEPLOY_GTK_VERSION=3 PATH="$tools:$PATH" \
    LDAI_UPDATE_INFORMATION="gh-releases-zsync|coredev-uk|sona-go|latest|sona-*-$arch.AppImage.zsync" \
    "$tools/linuxdeploy-$arch.AppImage" --appdir "$appdir" \
    --executable "$appdir/usr/bin/sona" \
    --desktop-file "$appdir/usr/share/applications/dev.coredev.Sona.desktop" \
    --icon-file "$root/build/linux/icons/hicolor/512x512/apps/sona.png" --icon-filename sona \
    "${libs[@]}" "${deps[@]}" --plugin gtk --output appimage
)

# linuxdeploy replaced our AppRun with a symlink only if none existed; make
# sure ours is the one that shipped, and that no CDM got in.
[[ -x $appdir/AppRun && ! -L $appdir/AppRun ]] || { echo "appimage.sh: AppRun was replaced" >&2; exit 1; }
if find "$appdir" -iname '*widevine*' | grep -q .; then
  echo "appimage.sh: a Widevine CDM ended up in the AppDir; refusing to package it" >&2
  exit 1
fi
if find "$appdir" \( -iname 'libwebkit*' -o -iname 'libgtk-4*' -o -iname 'libsoup*' \) | grep -q .; then
  echo "appimage.sh: WebKitGTK, GTK4 or libsoup ended up in the AppDir" >&2
  exit 1
fi
mv "$work/$name" "$out/$name"
mv "$work/$name.zsync" "$out/$name.zsync"
echo "appimage.sh: $out/$name"
