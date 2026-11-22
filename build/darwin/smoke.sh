#!/usr/bin/env bash
# Smoke test for the macOS artefacts (runs on macos-15, which has a window
# server, and on a Mac).
#
#   build/darwin/smoke.sh OUTDIR   # bundle.sh's output directory
#
# Passes when the bundle names no build-machine path, the extracted update
# archive is Sona.app alone and passes codesign, the disk image is the
# designed install window (mounted read-only: its .DS_Store read back with
# dmgbuild's reader, dmgcheck.py), `sona --version` prints the version, and the
# app reaches "startup: first paint" with a private HOME and only the bundle's
# GStreamer (its plugins load, a registry is written, and nothing from the
# framework in /Library is opened). Plays nothing (SONA_PRIVATE=1). Needs
# dmgbuild where install-dmgbuild.sh puts it (DMGBUILD_DIR), and Python 3.11+
# (PYTHON).
set -euo pipefail
smoke_name="smoke(darwin)"
# shellcheck source=../lib/smoke.sh
. "$(dirname "$0")/../lib/smoke.sh"
here=$(cd "$(dirname "$0")" && pwd)
out=$(cd "${1:?usage: smoke.sh OUTDIR}" && pwd)
version=$(version)

work=$(mktemp -d)
dmgmnt=$(mktemp -d)
trap 'kill "${pid:-}" 2>/dev/null || true; hdiutil detach -force "$dmgmnt" >/dev/null 2>&1 || true; rm -rf "$work" "$dmgmnt"' EXIT
# The update archive is what users run after an update.
tar -C "$work" -xzf "$out/sona-$version-darwin-arm64.tar.gz"
[[ $(ls "$work") == Sona.app ]] || fail "update archive is not Sona.app alone: $(ls "$work")"
app=$work/Sona.app
exe=$app/Contents/MacOS/sona

# The disk image: mounted read-only, its window is the one dmg/settings.py
# sets (bundle.sh wrote it with dmgbuild, no Finder), and the Sona.app in it
# is the signed bundle, untouched by the tool that built the image.
dmg=$out/Sona-$version-macos-arm64.dmg
dmgtools=${DMGBUILD_DIR:-$smoke_repo/bin/dmgbuild}
[[ -f $dmg ]] || fail "no $dmg"
[[ -f $dmgtools/ds_store/__init__.py ]] || fail "no dmgbuild in $dmgtools (build/darwin/install-dmgbuild.sh)"
[[ $(hdiutil imageinfo -plist "$dmg" 2>/dev/null | plutil -extract Format raw -o - -) == ULMO ]] || fail "the disk image is not ULMO"
hdiutil attach -readonly -nobrowse -noverify -mountpoint "$dmgmnt" "$dmg" >/dev/null 2>&1 || fail "the disk image does not mount"
[[ $(diskutil info -plist "$dmgmnt" | plutil -extract VolumeName raw -o - -) == Sona ]] || fail "the volume is not named Sona"
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=$dmgtools "${PYTHON:-python3}" "$here/dmgcheck.py" "$dmgmnt" || fail "the disk image's window layout"
codesign --verify --deep --strict "$dmgmnt/Sona.app" || fail "codesign verification of the Sona.app in the disk image"
cmp <(cd "$dmgmnt/Sona.app" && find . -type f -exec shasum -a 256 {} + | sort) <(cd "$app" && find . -type f -exec shasum -a 256 {} + | sort) ||
  fail "the Sona.app in the disk image is not the update archive's"
hdiutil detach "$dmgmnt" >/dev/null 2>&1 || fail "the disk image does not detach"
echo "smoke(darwin): disk image layout"

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
