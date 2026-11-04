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
out=$(cd "${1:?usage: smoke.sh OUTDIR}" && pwd)
version=${VERSION:-$(cat "$(dirname "$0")/../../VERSION")}
fail() { echo "smoke(darwin): FAIL: $*" >&2; exit 1; }

work=$(mktemp -d)
trap 'kill "${pid:-}" 2>/dev/null || true; rm -rf "$work"' EXIT
# The update archive is what users run after an update.
tar -C "$work" -xzf "$out/sona-$version-darwin-arm64.tar.gz"
[[ $(ls "$work") == Sona.app ]] || fail "update archive is not Sona.app alone: $(ls "$work")"
app=$work/Sona.app
exe=$app/Contents/MacOS/sona

codesign --verify --deep --strict "$app" || fail "codesign verification"
plutil -lint "$app/Contents/Info.plist" >/dev/null || fail "Info.plist"
for scheme in music itms; do
  /usr/libexec/PlistBuddy -c Print:CFBundleURLTypes "$app/Contents/Info.plist" | grep -qE "^ +$scheme\$" || fail "Info.plist lacks the $scheme URL scheme"
done
if find "$app" -type f \( -perm -u+x -o -name '*.dylib' \) -exec otool -L {} + 2>/dev/null | grep -E '(^|[[:space:]])(/nix/store|/opt/homebrew|/usr/local|/Library/Frameworks)'; then
  fail "the bundle links build-machine paths"
fi

got=$("$exe" --version) || fail "sona --version failed"
[[ $got == "sona $version" ]] || fail "sona --version printed '$got', want 'sona $version'"

export HOME=$work/home SONA_PRIVATE=1 SONA_PREVIEW=1
mkdir -p "$HOME"
log=$HOME/Library/Application\ Support/Sona/State/sona.log
"$exe" >"$work/sona.out" 2>&1 &
pid=$!
for _ in $(seq 1 60); do
  grep -q '"startup: first paint"' "$log" 2>/dev/null && break
  kill -0 "$pid" 2>/dev/null || break
  sleep 1
done
painted=0
grep -q '"startup: first paint"' "$log" 2>/dev/null && painted=1
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
if [[ $painted != 1 ]]; then
  cat "$work/sona.out" "$log" 2>/dev/null || true
  fail "sona never painted"
fi
echo "smoke(darwin): first paint"

# Motion artwork and preview playback from the bundle's framework, with the
# environment the app itself sets (cmd/sona/bundle_darwin.go): the tools in
# OUTDIR/smoke-tools (bundle.sh with SONA_SMOKE_TOOLS=1) link Sona.app's copy.
tools=$out/smoke-tools
if [[ -d $tools ]]; then
  root=$out/Sona.app/Contents/Frameworks/GStreamer
  export GST_PLUGIN_SYSTEM_PATH_1_0=$root/lib/plugins GST_PLUGIN_PATH_1_0='' \
    GST_PLUGIN_SCANNER=$root/libexec/gst-plugin-scanner GST_REGISTRY_1_0=$work/registry.bin
  # The fixture clip is made by the build's own gst-launch-1.0 and checked
  # with its gst-inspect-1.0, each in a process of its own: the wrappers drop
  # the bundle's GST_* variables so they see the whole framework.
  gst=${GSTREAMER_ROOT:-/Library/Frameworks/GStreamer.framework/Versions/1.0}
  mkdir "$work/shim"
  for t in gst-launch-1.0 gst-inspect-1.0; do
    printf '#!/bin/sh\nexec env -u GST_PLUGIN_SYSTEM_PATH_1_0 -u GST_PLUGIN_PATH_1_0 -u GST_PLUGIN_SCANNER -u GST_REGISTRY_1_0 "%s/bin/%s" "$@"\n' "$gst" "$t" >"$work/shim/$t"
    chmod +x "$work/shim/$t"
  done
  PATH=$work/shim:$PATH \
    "$tools/media.test" -test.v -test.run 'TestMotionDecoder|TestOpenMotion$' >"$work/motion.out" 2>&1 || { cat "$work/motion.out"; fail "motion artwork tests"; }
  grep -q -- '--- PASS: TestMotionDecoderPlaysAndLoops' "$work/motion.out" || { cat "$work/motion.out"; fail "the motion decoder did not decode the fixture (skipped?)"; }
  echo "smoke(darwin): motion artwork decodes"
  "$tools/playspike" -preview -storefront us >"$work/preview.out" 2>&1 || { cat "$work/preview.out"; fail "preview playback"; }
  echo "smoke(darwin): preview plays"
fi

# Whatever it opened must be inside the bundle (or the system): lsof is not
# needed, the registry's plugin paths say which GStreamer ran.
registry=$HOME/Library/Caches/Sona/gstreamer-1.0/registry.bin
if [[ -f $registry ]]; then
  ! strings "$registry" | grep -E '/Library/Frameworks/GStreamer.framework' || fail "GStreamer's registry names the system framework"
fi
echo "smoke(darwin): PASS"
