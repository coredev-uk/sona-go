#!/usr/bin/env bash
# Wraps a macOS build of Sona in a minimal app bundle, so that macOS gives it
# what it ties to a bundle: track notifications (UNUserNotificationCenter) and
# Settings → General → Start Sona when you log in (SMAppService). A bare
# bin/sona has neither, and Settings says "Only when Sona runs from Sona.app".
#
#   build/darwin/devbundle.sh BINARY OUT.app
#   OUT.app/Contents/MacOS/sona        # run the inside, so devenv's GStreamer applies
#
# This is not the release bundle (bundle.sh): no GStreamer is copied, so
# playback needs the devenv shell's, and it is ad-hoc signed. It has its own
# identifier (dev.coredev.Sona.dev), so the notification permission and login
# item are separate from an installed Sona.app's, and it claims no music://
# or itms:// links. Remove a login item you enabled in it (turn the setting
# off) before deleting the bundle.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
bin=${1:?usage: devbundle.sh BINARY OUT.app}
app=${2:?usage: devbundle.sh BINARY OUT.app}
[[ -x $bin ]] || { echo "devbundle: $bin is not an executable" >&2; exit 1; }
[[ $app == *.app ]] || { echo "devbundle: $app must end in .app" >&2; exit 1; }
version=$(cat "$repo/VERSION")
minos=$(tr -d '[:space:]' <"$here/MIN_MACOS")

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$bin" "$app/Contents/MacOS/sona"
sed -e "s|@VERSION@|$version|g" -e "s|@MINOS@|$minos|g" "$here/Info.plist.in" >"$app/Contents/Info.plist"
plutil -replace CFBundleIdentifier -string dev.coredev.Sona.dev "$app/Contents/Info.plist"
plutil -remove CFBundleURLTypes "$app/Contents/Info.plist"
printf 'APPL????' >"$app/Contents/PkgInfo"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
iconset=$work/Sona.iconset
mkdir "$iconset"
for s in 16 32 128 256 512; do
  sips -z $s $s "$repo/build/appicon.png" --out "$iconset/icon_${s}x${s}.png" >/dev/null
  sips -z $((s * 2)) $((s * 2)) "$repo/build/appicon.png" --out "$iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/Sona.icns"

codesign --force -s - "$app"
echo "devbundle: $app (dev.coredev.Sona.dev)"
