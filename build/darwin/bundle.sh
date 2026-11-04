#!/usr/bin/env bash
# Builds Sona.app for Apple silicon (docs/release-checklist.md, "macOS").
#
#   build/darwin/bundle.sh OUTDIR
#
# Writes into OUTDIR: Sona.app (ad-hoc signed), Sona-<version>-macos-arm64.dmg
# and sona-<version>-darwin-arm64.tar.gz (Sona.app alone, the archive the
# updater installs). Plain Go and Xcode's clang, linked against the official
# GStreamer framework (build/darwin/install-gstreamer.sh), whose lib/ and
# libexec/ are copied into Sona.app/Contents/Frameworks/GStreamer, trimmed to
# the plugins Sona uses and made relocatable.
#
# Environment:
#   GSTREAMER_ROOT  the framework's Versions/1.0 directory
#                   (default /Library/Frameworks/GStreamer)
#   VERSION         default: the VERSION file
#   SONA_SMOKE_TOOLS=1   also build OUTDIR/smoke-tools (playspike, media.test) for smoke.sh
#   UPDATE_FEED, UPDATE_KEY   linked into the binary for self-updates (the
#                   release job sets them; unset = updates off)
#   SONA_LASTFM_KEY, SONA_LASTFM_SECRET, SONA_DISCORD_CLIENT_ID   as build/linux
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
out=${1:?usage: bundle.sh OUTDIR}
mkdir -p "$out"
out=$(cd "$out" && pwd)
version=${VERSION:-$(cat "$repo/VERSION")}
gst=${GSTREAMER_ROOT:-/Library/Frameworks/GStreamer}
minos=$(tr -d '[:space:]' <"$here/MIN_MACOS")
[[ -d $gst/lib/gstreamer-1.0 ]] || { echo "bundle: no GStreamer framework at $gst (build/darwin/install-gstreamer.sh)" >&2; exit 1; }

# Plugins Sona uses: playback (decodebin, the preview and CDM host engines),
# motion artwork (H.264/HEVC through vtdec or libav, videoconvert) and
# osxaudiosink. required ones must exist; optional ones are kept when present.
required=(coreelements typefindfunctions playback app audioconvert audioresample volume audioparsers
  isomp4 videoconvertscale videoparsersbad applemedia osxaudio libav)
optional=(audiomixer audiorate autodetect videorate gio rawparse tcp)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# 1. Build against the framework wherever it is installed: its pkg-config
# files say ${pcfiledir}/../.. (which only pkgconf resolves) or name the
# default prefix, so point both at gst.
mkdir "$work/pc"
for f in "$gst"/lib/pkgconfig/*.pc; do
  sed -e "s|/Library/Frameworks/GStreamer|$gst|g" -e "s|\${pcfiledir}/../..|$gst|g" "$f" >"$work/pc/$(basename "$f")"
done
ldflags="-w -s -X main.version=$version -X github.com/egoist/mygo.production=1 -extldflags=-mmacosx-version-min=$minos"
[[ -n ${UPDATE_FEED:-} ]] && ldflags+=" -X github.com/egoist/mygo.packageUpdateFeed=$UPDATE_FEED"
[[ -n ${UPDATE_KEY:-} ]] && ldflags+=" -X github.com/egoist/mygo.packageUpdateKey=$UPDATE_KEY"
[[ -n ${SONA_LASTFM_KEY:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=$SONA_LASTFM_KEY"
[[ -n ${SONA_LASTFM_SECRET:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=$SONA_LASTFM_SECRET"
[[ -n ${SONA_DISCORD_CLIENT_ID:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/discord.clientID=$SONA_DISCORD_CLIENT_ID"
gocmd() { # go ARGS...: with the framework and the deployment target
  (
    cd "$repo"
    export GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=$minos
    export PKG_CONFIG_PATH=$work/pc CGO_CFLAGS="-mmacosx-version-min=$minos" CGO_LDFLAGS="-mmacosx-version-min=$minos"
    go "$@"
  )
}
gocmd build -tags mygo_noinspector,sqlite_fts5 -trimpath -buildvcs=false -ldflags "$ldflags" -o "$work/sona" ./cmd/sona
# smoke.sh also runs playspike and the motion decoder's tests against the
# bundle's framework; they are built here (beside the bundle, never in it).
if [[ -n ${SONA_SMOKE_TOOLS:-} ]]; then
  gocmd build -tags sqlite_fts5 -trimpath -buildvcs=false -o "$work/playspike" ./cmd/playspike
  gocmd test -c -tags sqlite_fts5 -o "$work/media.test" ./internal/ui/media
fi

# 2. The bundle.
app=$out/Sona.app
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$work/sona" "$app/Contents/MacOS/sona"
sed -e "s|@VERSION@|$version|g" -e "s|@MINOS@|$minos|g" "$here/Info.plist.in" >"$app/Contents/Info.plist"
printf 'APPL????' >"$app/Contents/PkgInfo"
iconset=$work/Sona.iconset
mkdir "$iconset"
for s in 16 32 128 256 512; do
  sips -z $s $s "$repo/build/appicon.png" --out "$iconset/icon_${s}x${s}.png" >/dev/null
  sips -z $((s * 2)) $((s * 2)) "$repo/build/appicon.png" --out "$iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/Sona.icns"

# 3. The framework, trimmed. Dependencies of the binary and of the kept
# plugins are copied to lib/; everything else is left out.
fw=$app/Contents/Frameworks/GStreamer
# (no dots in folder names: codesign takes "gstreamer-1.0" for a bundle)
mkdir -p "$fw/lib/plugins" "$fw/libexec"
cp "$gst/libexec/gstreamer-1.0/gst-plugin-scanner" "$fw/libexec/"
for p in "${required[@]}"; do
  [[ -f $gst/lib/gstreamer-1.0/libgst$p.dylib ]] || { echo "bundle: required plugin $p is missing from $gst" >&2; exit 1; }
  cp "$gst/lib/gstreamer-1.0/libgst$p.dylib" "$fw/lib/plugins/"
done
for p in "${optional[@]}"; do
  [[ -f $gst/lib/gstreamer-1.0/libgst$p.dylib ]] && cp "$gst/lib/gstreamer-1.0/libgst$p.dylib" "$fw/lib/plugins/"
done

# deps FILE prints each dependency's install name (not the file's own id).
deps() {
  local id
  id=$(otool -D "$1" | tail -n +2)
  otool -L "$1" | tail -n +2 | awk '{print $1}' | { grep -vxF -- "${id:-/nonexistent}" || true; }
}
# fwpath resolves an install name to the file in the framework, or nothing
# for a system library; a path outside the framework and the system fails.
fwpath() {
  case $1 in
  "$gst"/*) echo "$1" ;;
  /Library/Frameworks/GStreamer/*) echo "$gst/${1#/Library/Frameworks/GStreamer/}" ;;
  @rpath/*) echo "$gst/lib/${1##*/}" ;;
  /usr/lib/* | /System/*) ;;
  # A devenv build (SONA_DEVENV_BUILD=1, for trying this script on a Mac that
  # builds with devenv's clang) links the nix copies of macOS's own libraries,
  # which relocate() points back at /usr/lib. CI builds with Xcode: never set.
  /nix/store/*/lib/libresolv.* | /nix/store/*/lib/libiconv.* | /nix/store/*/lib/libc++*)
    [[ -n ${SONA_DEVENV_BUILD:-} ]] || { echo "bundle: $2 links $1, outside the framework and the system" >&2; return 1; } ;;
  *) echo "bundle: $2 links $1, outside the framework and the system" >&2; return 1 ;;
  esac
}
queue=("$fw"/lib/plugins/*.dylib "$fw/libexec/gst-plugin-scanner" "$work/sona")
[[ -n ${SONA_SMOKE_TOOLS:-} ]] && queue+=("$work/playspike" "$work/media.test")
declare -A seen
for ((i = 0; i < ${#queue[@]}; i++)); do
  f=${queue[i]}
  while read -r d; do
    s=$(fwpath "$d" "$f") || exit 1
    [[ -n $s ]] || continue
    n=${s##*/}
    [[ -n ${seen[$n]:-} ]] && continue
    seen[$n]=1
    [[ -e $fw/lib/$n ]] || cp "$s" "$fw/lib/$n"
    queue+=("$fw/lib/$n")
  done < <(deps "$f")
done

# Apple silicon only: the framework's libraries are universal.
while IFS= read -r -d '' f; do
  if [[ $(lipo -archs "$f" 2>/dev/null) == *x86_64*arm64* || $(lipo -archs "$f" 2>/dev/null) == *arm64*x86_64* ]]; then
    lipo -thin arm64 "$f" -output "$f.thin" && mv "$f.thin" "$f"
  fi
done < <(find "$fw" -type f \( -name '*.dylib' -o -name gst-plugin-scanner \) -print0)

# 4. Relocate: every framework dependency becomes a path relative to the
# file that loads it.
relocate() { # file prefix-for-lib-dir
  local f=$1 prefix=$2 d s
  while read -r d; do
    s=$(fwpath "$d" "$f") || exit 1
    if [[ $d == /nix/store/* ]]; then
      install_name_tool -change "$d" "/usr/lib/${d##*/}" "$f"
      continue
    fi
    [[ -n $s ]] || continue
    install_name_tool -change "$d" "$prefix/${s##*/}" "$f"
  done < <(deps "$f")
}
chmod -R u+w "$app"
for f in "$fw"/lib/*.dylib; do
  install_name_tool -id "@loader_path/${f##*/}" "$f"
  relocate "$f" @loader_path
done
for f in "$fw"/lib/plugins/*.dylib; do
  install_name_tool -id "@loader_path/${f##*/}" "$f"
  relocate "$f" @loader_path/..
done
relocate "$fw/libexec/gst-plugin-scanner" @loader_path/../lib
relocate "$app/Contents/MacOS/sona" @executable_path/../Frameworks/GStreamer/lib

if [[ -n ${SONA_SMOKE_TOOLS:-} ]]; then
  mkdir -p "$out/smoke-tools"
  for t in playspike media.test; do
    relocate "$work/$t" @executable_path/../Sona.app/Contents/Frameworks/GStreamer/lib
    cp "$work/$t" "$out/smoke-tools/$t"
    codesign --force -s - "$out/smoke-tools/$t"
  done
fi

# The bundle must not point at a build machine.
if bad=$(find "$app" -type f \( -perm -u+x -o -name '*.dylib' \) -exec sh -c 'otool -L "$1" 2>/dev/null | grep -E "(^|[[:space:]])(/nix/store|/opt/homebrew|/usr/local|/Library/Frameworks)" && echo "  in $1"' _ {} \;) && [[ -n $bad ]]; then
  echo "bundle: build-machine paths remain:" >&2
  echo "$bad" >&2
  exit 1
fi

# 5. Apple silicon runs only signed code: sign ad hoc after the edits, with no
# hardened runtime so the Google-signed Widevine library loads. The copied
# GStreamer is a plain folder (a .framework would be a bundle codesign wants an
# Info.plist and Versions/Current for), so every Mach-O is signed by itself.
while IFS= read -r -d '' f; do
  if file -b "$f" | grep -q Mach-O; then codesign --force -s - "$f" 2>/dev/null; fi
done < <(find "$app/Contents/Frameworks" -type f -print0)
codesign --force -s - "$app"
codesign --verify --deep --strict "$app"

# 6. The artefacts. Sona.app/ is the only top-level entry of the update
# archive (MyGo's updater requires it).
stage=$work/dmg
mkdir "$stage"
ditto "$app" "$stage/Sona.app"
ln -s /Applications "$stage/Applications"
rm -f "$out/Sona-$version-macos-arm64.dmg"
hdiutil create -quiet -volname "Sona" -srcfolder "$stage" -ov -format UDZO "$out/Sona-$version-macos-arm64.dmg"
tar -C "$out" --no-xattrs -czf "$out/sona-$version-darwin-arm64.tar.gz" Sona.app
echo "bundle: $app, $out/Sona-$version-macos-arm64.dmg, $out/sona-$version-darwin-arm64.tar.gz"
