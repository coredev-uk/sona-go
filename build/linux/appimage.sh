#!/usr/bin/env bash
# Builds Sona's AppImage from a release binary.
#
#   build/linux/appimage.sh <binary> <output-dir> <version>
#
# `wails3 generate appimage` bundles GTK4 and WebKitGTK (with its helper
# processes relocated into the AppDir). The GStreamer plugins the playback
# host uses are dlopened, so linuxdeploy cannot see them: this script copies
# them in, points GStreamer at the bundled copies only, deploys their own
# libraries (libavcodec, libpulse, …) and repacks.
#
# Audio output inside the AppImage is pulsesink, which talks to PipeWire
# through pipewire-pulse or to PulseAudio. pipewiresink is not bundled: a
# private libpipewire would have to match the host's PipeWire modules.
#
# libwidevinecdm.so is never bundled or downloaded; Sona finds the user's
# installed CDM (Google Chrome) at runtime. The build fails if one appears in
# the AppDir.
#
# Run it on the release baseline (build/linux/release/Containerfile), not on
# NixOS: it bundles the build machine's libraries. Needs network on first
# use (linuxdeploy download).
set -euo pipefail

bin=$(realpath "$1")
out=$(realpath -m "$2")
version=$3
root=$(cd "$(dirname "$0")/../.." && pwd)
work="$root/build/linux/appimage" # gitignored scratch
arch=x86_64

rm -rf "$work"
mkdir -p "$work" "$out"
cp "$bin" "$work/sona"
cp "$root/build/appicon.png" "$work/sona.png"

go tool wails3 generate appimage \
  -binary "$work/sona" \
  -icon "$work/sona.png" \
  -desktopfile "$root/build/linux/dev.coredev.Sona.desktop" \
  -outputdir "$work" \
  -builddir "$work/build"

appdir="$work/build/sona-$arch.AppDir"
linuxdeploy="$work/build/linuxdeploy-$arch.AppImage"

# Icons at every size, for desktop integration (appimaged, AppImageLauncher).
cp -r "$root/build/linux/icons/hicolor" "$appdir/usr/share/icons/"

# GStreamer: appsrc ! avdec_aac ! audioconvert ! audioresample ! volume ! pulsesink
# (autoaudiosink as the fallback sink), plus the registry scanner. From
# gst-plugins-bad, subenc: WebKit's media players need its webvttenc and
# warn once per player without it. The bundled set is all GStreamer the
# AppImage sees, WebKit's web process included.
gst_plugins=$(pkg-config --variable=pluginsdir gstreamer-1.0)
gst_scanner="$(pkg-config --variable=pluginscannerdir gstreamer-1.0)/gst-plugin-scanner"
mkdir -p "$appdir/usr/lib/gstreamer-1.0" "$appdir/usr/libexec/gstreamer-1.0"
for p in coreelements app audioconvert audioresample volume autodetect libav pulseaudio subenc; do
  cp "$gst_plugins/libgst$p.so" "$appdir/usr/lib/gstreamer-1.0/"
done
cp "$gst_scanner" "$appdir/usr/libexec/gstreamer-1.0/"

# Wails' AppRun ends with `cd "$usr/lib"` + exec; the GStreamer environment
# goes just before it. Only the bundled plugins are scanned, with a registry
# of their own so the host's registry is never mixed in.
apprun="$appdir/AppRun"
gst_env='export GST_PLUGIN_SYSTEM_PATH_1_0="$usr/lib/gstreamer-1.0"\
export GST_PLUGIN_PATH_1_0=""\
export GST_PLUGIN_SCANNER_1_0="$usr/libexec/gstreamer-1.0/gst-plugin-scanner"\
export GST_REGISTRY_1_0="${XDG_CACHE_HOME:-$HOME/.cache}/sona/gstreamer-registry-appimage.bin"'
sed -i "/^cd \"\$usr\/lib\"\$/i $gst_env" "$apprun"
grep -q GST_PLUGIN_SYSTEM_PATH_1_0 "$apprun" || { echo "appimage.sh: AppRun layout changed; GStreamer env not inserted" >&2; exit 1; }

if find "$appdir" -iname '*widevine*' | grep -q .; then
  echo "appimage.sh: a Widevine CDM ended up in the AppDir; refusing to package it" >&2
  exit 1
fi

# Licence notices (as in the deb/rpm/Arch packages).
mkdir -p "$appdir/usr/share/doc/sona"
cp "$root/LICENSE" "$root/internal/playback/widevine/LICENSE.chromium" "$appdir/usr/share/doc/sona/"

# Repack, deploying the plugins' and the scanner's shared-library deps. The
# update information lets AppImageUpdate fetch the newest GitHub release;
# linuxdeploy's AppImage plugin embeds it and writes <name>.zsync next to
# the AppImage (a release asset alongside it).
deps=()
for f in "$appdir"/usr/lib/gstreamer-1.0/*.so "$appdir/usr/libexec/gstreamer-1.0/gst-plugin-scanner"; do
  deps+=(--deploy-deps-only "$f")
done
name="sona-$version-$arch.AppImage"
(
  cd "$work/build"
  rm -f ./*.AppImage.tmp ./*.zsync
  NO_STRIP=1 OUTPUT="$name" \
    LDAI_UPDATE_INFORMATION="gh-releases-zsync|coredev-uk|sona-go|latest|sona-*-$arch.AppImage.zsync" \
    "$linuxdeploy" --appimage-extract-and-run --appdir "$appdir" "${deps[@]}" --output appimage
)
mv "$work/build/$name" "$out/$name"
mv "$work/build/$name.zsync" "$out/$name.zsync"
echo "appimage.sh: $out/$name"
