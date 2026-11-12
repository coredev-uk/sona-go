#!/usr/bin/env bash
# Smoke test for the macOS artefacts (runs on macos-15, which has a window
# server, and on a Mac).
#
#   build/darwin/smoke.sh OUTDIR   # bundle.sh's output directory
#
# Passes when the bundle names no build-machine path, the extracted update
# archive is Sona.app alone and passes codesign, `sona --version` prints the
# version, and the app reaches "startup: first paint" with a private HOME and
# only the bundle's GStreamer (its plugins load, a registry is written, and
# nothing from the framework in /Library is opened). Plays nothing
# (SONA_PRIVATE=1).
set -euo pipefail
smoke_name="smoke(darwin)"
# shellcheck source=../lib/smoke.sh
. "$(dirname "$0")/../lib/smoke.sh"
out=$(cd "${1:?usage: smoke.sh OUTDIR}" && pwd)
version=$(version)

work=$(mktemp -d)
trap 'kill "${pid:-}" 2>/dev/null || true; rm -rf "$work"' EXIT
# The update archive is what users run after an update.
tar -C "$work" -xzf "$out/sona-$version-darwin-arm64.tar.gz"
[[ $(ls "$work") == Sona.app ]] || fail "update archive is not Sona.app alone: $(ls "$work")"
app=$work/Sona.app
exe=$app/Contents/MacOS/sona

codesign --verify --deep --strict "$app" || fail "codesign verification"
plutil -lint "$app/Contents/Info.plist" >/dev/null || fail "Info.plist"
for scheme in music musics itms itmss; do
  /usr/libexec/PlistBuddy -c Print:CFBundleURLTypes "$app/Contents/Info.plist" | grep -qE "^ +$scheme\$" || fail "Info.plist lacks the $scheme URL scheme"
done
if find "$app" -type f \( -perm -u+x -o -name '*.dylib' \) -exec otool -L {} + 2>/dev/null | grep -E '(^|[[:space:]])(/nix/store|/opt/homebrew|/usr/local|/Library/Frameworks)'; then
  fail "the bundle links build-machine paths"
fi

check_version "$version" "$exe"

export HOME=$work/home SONA_PRIVATE=1 SONA_PREVIEW=1
mkdir -p "$HOME"
log=$HOME/Library/Application\ Support/Sona/State/sona.log
"$exe" >"$work/sona.out" 2>&1 &
pid=$!
painted=0
wait_for_log "$log" '"startup: first paint"' 60 "$pid" && painted=1
stop_app "$pid"
if [[ $painted != 1 ]]; then
  cat "$work/sona.out" "$log" 2>/dev/null || true
  fail "sona never painted"
fi
echo "smoke(darwin): first paint"

# Preview playback from the bundle's framework, with the environment the app
# itself sets (cmd/sona/bundle_darwin.go): playspike in OUTDIR/smoke-tools
# (bundle.sh with SONA_SMOKE_TOOLS=1) links Sona.app's copy. Motion artwork
# needs none of it (AVFoundation plays it), so the bundle has no video plugin.
tools=$out/smoke-tools
if [[ -d $tools ]]; then
  root=$out/Sona.app/Contents/Frameworks/GStreamer
  for p in videoconvertscale videoparsersbad applemedia; do
    [[ ! -e $root/lib/plugins/libgst$p.dylib ]] || fail "the bundle has the $p plugin, which only motion artwork used to need"
  done
  export GST_PLUGIN_SYSTEM_PATH_1_0=$root/lib/plugins GST_PLUGIN_PATH_1_0='' \
    GST_PLUGIN_SCANNER=$root/libexec/gst-plugin-scanner GST_REGISTRY_1_0=$work/registry.bin
  "$tools/playspike" -preview -storefront us >"$work/preview.out" 2>&1 || { cat "$work/preview.out"; fail "preview playback"; }
  grep -q 'decoder=avdec_aac' "$work/preview.out" || { cat "$work/preview.out"; fail "the bundled libav did not decode the preview"; }
  echo "smoke(darwin): preview plays"
fi

# Whatever it opened must be inside the bundle (or the system): lsof is not
# needed, the registry's plugin paths say which GStreamer ran.
registry=$HOME/Library/Caches/Sona/gstreamer-1.0/registry.bin
if [[ -f $registry ]]; then
  ! strings "$registry" | grep -E '/Library/Frameworks/GStreamer.framework' || fail "GStreamer's registry names the system framework"
fi
echo "smoke(darwin): PASS"
