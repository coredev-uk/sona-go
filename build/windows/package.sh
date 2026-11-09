#!/usr/bin/env bash
# Builds Sona for Windows x64 (run in Git Bash on windows-2025 with MinGW-w64
# gcc and objdump, Go, pkg-config and makensis on PATH).
#
#   build/windows/package.sh OUTDIR
#
# Writes into OUTDIR: Sona-<version>-windows-x64.zip (portable),
# Sona-<version>-windows-x64-setup.exe (per-user NSIS installer) and
# sona-<version>-windows-amd64.tar.gz (the update archive: the same files,
# at the top level). Every one holds
#
#   sona.exe, sona.ico, sona_cdm_shim.dll (the CDM host's MSVC shim)
#   *.dll                                  the GStreamer libraries sona.exe, the plugins and the scanner import
#   lib\gstreamer-1.0\libgst*.dll          only the plugins Sona uses
#   libexec\gstreamer-1.0\gst-plugin-scanner.exe
#   licenses\                              Sona's, Chromium's, GStreamer's
#
# and never the Widevine CDM (Sona downloads that from Google at run time).
# internal/playback/gstbundle points GStreamer at lib\ and libexec\ at start.
#
# Like build/darwin/bundle.sh it builds against the official GStreamer
# package (build/windows/install-gstreamer.sh, pinned) and builds the shim
# (build/windows/build-shim.sh). The CI action .github/actions/setup-windows
# does both first; its GSTREAMER_ROOT, PKG_CONFIG_PATH and SONA_CDM_SHIM are
# used when set, else this script does it itself.
#
# Environment: VERSION (default: the VERSION file); UPDATE_FEED, UPDATE_KEY
# (linked in for self-updates; unset = updates off); SONA_LASTFM_KEY,
# SONA_LASTFM_SECRET, SONA_DISCORD_CLIENT_ID as build/linux;
# SONA_SMOKE_TOOLS=1 also builds OUTDIR/smoke-tools/playspike.exe and
# media.test.exe (the motion artwork decoder's tests) for smoke.sh.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
out=${1:?usage: package.sh OUTDIR}
mkdir -p "$out"
out=$(cd "$out" && pwd)
version=${VERSION:-$(cat "$repo/VERSION")}
die() { echo "package: $*" >&2; exit 1; }

# makensis wants a four-part number: 0.3.0-rc.1 → 0.3.0.0.
numeric=$(echo "${version%%-*}.0.0.0.0" | cut -d. -f1-4)

# Plugins Sona uses. Playback is appsrc/appsink around decodebin (the CDM
# host's lanes, the preview and local files): coreelements (queue, fakesink),
# typefindfunctions, playback, app, audioparsers and isomp4 (AAC in fMP4/M4A,
# MP3, FLAC), libav (avdec_aac and the rest), the output tail (audioconvert,
# audioresample, volume, equalizer) and the WASAPI sink. Music video pictures
# come from the CDM's own decoder, not GStreamer's. Motion artwork
# (internal/ui/media: file ! qtdemux ! h264parse|h265parse ! decoder !
# videoconvert ! appsink) adds videoparsersbad (the parsers), videoconvertscale
# and d3d11 (Direct3D 11 hardware decoders of H.264 and HEVC, which decodebin
# ranks above libav where a GPU has them); where it has none, libav's
# avdec_h264/avdec_h265 decode. Media Foundation has encoders and capture only,
# so there is no mediafoundation plugin here.
# required ones must exist; optional ones are kept when present.
required=(coreelements typefindfunctions playback app audioconvert audioresample volume equalizer
  audioparsers isomp4 libav wasapi2 videoparsersbad videoconvertscale d3d11)
optional=(wasapi directsound autodetect audiomixer audiorate gio rawparse tcp id3demux apetag wavparse aiff)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# 1. GStreamer and the shim.
if [[ -z ${GSTREAMER_ROOT:-} || -z ${PKG_CONFIG_PATH:-} ]]; then
  setup=$("$here/install-gstreamer.sh" | tail -n 2)
  GSTREAMER_ROOT=$(sed -n 's/^GSTREAMER_ROOT=//p' <<<"$setup")
  PKG_CONFIG_PATH=$(sed -n 's/^PKG_CONFIG_PATH=//p' <<<"$setup")
fi
gst=$(cygpath -u "$GSTREAMER_ROOT")
[[ -f $gst/bin/libgstreamer-1.0-0.dll && -d $gst/lib/gstreamer-1.0 ]] || die "no GStreamer installation at $GSTREAMER_ROOT"
shim=
[[ -n ${SONA_CDM_SHIM:-} ]] && shim=$(cygpath -u "$SONA_CDM_SHIM")
if [[ -z $shim || ! -f $shim ]]; then
  "$here/build-shim.sh" "$work/shim"
  shim=$work/shim/sona_cdm_shim.dll
fi
export PKG_CONFIG=${PKG_CONFIG:-pkg-config}
command -v "$PKG_CONFIG" >/dev/null || die "no pkg-config (the CI action installs pkg-config-lite)"
command -v objdump >/dev/null || die "no objdump (MinGW-w64's binutils, with gcc)"
PKG_CONFIG_PATH=$PKG_CONFIG_PATH "$PKG_CONFIG" --exists gstreamer-1.0 gstreamer-app-1.0 gstreamer-video-1.0 || die "pkg-config does not find GStreamer in $PKG_CONFIG_PATH"

# 2. sona.exe, linked against GStreamer's import libraries.
ldflags="-w -s -H windowsgui -X main.version=$version -X github.com/egoist/mygo.production=1"
[[ -n ${UPDATE_FEED:-} ]] && ldflags+=" -X github.com/egoist/mygo.packageUpdateFeed=$UPDATE_FEED"
[[ -n ${UPDATE_KEY:-} ]] && ldflags+=" -X github.com/egoist/mygo.packageUpdateKey=$UPDATE_KEY"
[[ -n ${SONA_LASTFM_KEY:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=$SONA_LASTFM_KEY"
[[ -n ${SONA_LASTFM_SECRET:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=$SONA_LASTFM_SECRET"
[[ -n ${SONA_DISCORD_CLIENT_ID:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/discord.clientID=$SONA_DISCORD_CLIENT_ID"
gocmd() { # go ARGS...: for Windows, with GStreamer
  (
    cd "$repo"
    export GOOS=windows GOARCH=amd64 CGO_ENABLED=1 PKG_CONFIG_PATH
    go "$@"
  )
}
dir=$work/Sona
mkdir "$dir"
gocmd build -tags mygo_noinspector,sqlite_fts5 -trimpath -buildvcs=false -ldflags "$ldflags" -o "$dir/sona.exe" ./cmd/sona
# smoke.sh runs playspike and the motion decoder's tests against the bundle's
# GStreamer; they are built here, beside the package, never in it.
if [[ -n ${SONA_SMOKE_TOOLS:-} ]]; then
  mkdir -p "$out/smoke-tools"
  gocmd build -tags sqlite_fts5 -trimpath -buildvcs=false -o "$out/smoke-tools/playspike.exe" ./cmd/playspike
  gocmd test -c -tags sqlite_fts5 -trimpath -buildvcs=false -o "$out/smoke-tools/media.test.exe" ./internal/ui/media
fi

# 3. The files.
cp "$here/sona.ico" "$dir/sona.ico"
cp "$shim" "$dir/sona_cdm_shim.dll"
mkdir -p "$dir/lib/gstreamer-1.0" "$dir/libexec/gstreamer-1.0" "$dir/licenses"
scanner=$gst/libexec/gstreamer-1.0/gst-plugin-scanner.exe
[[ -f $scanner ]] || die "GStreamer has no gst-plugin-scanner.exe at $scanner"
cp "$scanner" "$dir/libexec/gstreamer-1.0/"
for p in "${required[@]}"; do
  [[ -f $gst/lib/gstreamer-1.0/libgst$p.dll ]] || die "required plugin $p is missing from $GSTREAMER_ROOT"
  cp "$gst/lib/gstreamer-1.0/libgst$p.dll" "$dir/lib/gstreamer-1.0/"
done
for p in "${optional[@]}"; do
  if [[ -f $gst/lib/gstreamer-1.0/libgst$p.dll ]]; then cp "$gst/lib/gstreamer-1.0/libgst$p.dll" "$dir/lib/gstreamer-1.0/"; fi
done

# The DLLs the program, the scanner and the plugins import, and what those
# import, from GStreamer's bin\ (the MinGW runtime's libwinpthread and libgcc
# among them) into the folder of sona.exe, where Windows looks first. What
# Windows has itself is not copied; anything else is an error.
declare -A avail # lower-case name → file in GStreamer's bin\
while IFS= read -r f; do avail[$(basename "${f,,}")]=$f; done < <(find "$gst/bin" -maxdepth 1 -type f -iname '*.dll')
sysdir=$(cygpath -u "${SYSTEMROOT:-C:\\Windows}")/System32
imports() { # FILE: the DLLs it imports, by name
  local dump
  dump=$(objdump -p "$1") || die "objdump cannot read $1"
  sed -n 's/^[[:space:]]*DLL Name: //p' <<<"$dump" | tr -d '\r'
}
is_system() { [[ $1 == api-ms-win-* || $1 == ext-ms-win-* || -e $sysdir/$1 ]]; }
queue=("$dir/sona.exe" "$dir"/lib/gstreamer-1.0/*.dll "$dir/libexec/gstreamer-1.0/gst-plugin-scanner.exe")
declare -A seen
for ((i = 0; i < ${#queue[@]}; i++)); do
  deps=$(imports "${queue[i]}") # in this shell: a failure stops the script
  while IFS= read -r d; do
    n=${d,,}
    [[ -n ${seen[$n]:-} ]] && continue
    seen[$n]=1
    if [[ -n ${avail[$n]:-} ]]; then
      cp "${avail[$n]}" "$dir/$(basename "${avail[$n]}")"
      queue+=("$dir/$(basename "${avail[$n]}")")
    elif ! is_system "$d"; then
      die "${queue[i]##*/} imports $d, which is neither in GStreamer's bin nor Windows'"
    fi
  done <<<"$deps"
done
# playspike and media.test (smoke) import a subset: nothing they need may be missing.
for tool in playspike media.test; do
  if [[ -f $out/smoke-tools/$tool.exe ]]; then
    deps=$(imports "$out/smoke-tools/$tool.exe")
    while IFS= read -r d; do
      [[ -e $dir/$d ]] || is_system "$d" || die "$tool.exe imports $d, which the package lacks"
    done <<<"$deps"
  fi
done

# Licences: Sona's, Chromium's (the CDM interface header), and those of the
# packages of the official installation that the files copied above come from
# (share\licenses\<package>\: GStreamer's own, GLib, FFmpeg behind the libav
# plugin, the MinGW runtime, and the small libraries): not the licences of the
# installation's other packages, whose code is not in this package.
cp "$repo/LICENSE" "$dir/licenses/LICENSE.txt"
cp "$repo/internal/playback/widevine/LICENSE.chromium" "$dir/licenses/LICENSE.chromium"
cp "$repo/THIRD_PARTY_NOTICES.md" "$dir/licenses/THIRD_PARTY_NOTICES.md"
for p in gstreamer-1.0 gst-plugins-base-1.0 gst-plugins-good-1.0 gst-plugins-bad-1.0 ffmpeg glib libffi libiconv \
  proxy-libintl pcre2 orc bzip2 zlib mingw-runtime; do
  if [[ -d $gst/share/licenses/$p ]]; then
    mkdir -p "$dir/licenses/GStreamer"
    cp -r "$gst/share/licenses/$p" "$dir/licenses/GStreamer/$p"
  fi
done

# Never the CDM, whatever a future edit of the lists above does.
if bad=$(find "$dir" -iname '*widevinecdm*') && [[ -n $bad ]]; then
  die "the package would contain the Widevine CDM: $bad"
fi
echo "package: $(find "$dir" -type f | wc -l) files, $(find "$dir" -type f -printf '%s\n' | awk '{s += $1} END {printf "%.1f", s / 1048576}') MB; GStreamer plugins: $(cd "$dir/lib/gstreamer-1.0" && echo libgst*.dll | sed 's/libgst//g; s/\.dll//g')"

# 4. The artefacts. The portable zip's folder is Sona\, the updater's
# tar.gz has no wrapper folder, and the installer takes the folder as it is.
rm -f "$out/Sona-$version-windows-x64.zip"
(cd "$work" && 7z a -tzip -bso0 -bsp0 "$(cygpath -w "$out/Sona-$version-windows-x64.zip")" Sona)
tar -C "$dir" -czf "$out/sona-$version-windows-amd64.tar.gz" .
if tar -tzf "$out/sona-$version-windows-amd64.tar.gz" | grep -qi widevinecdm; then die "the update archive contains the Widevine CDM"; fi
makensis -V2 -DVERSION="$version" -DVERSION_NUMERIC="$numeric" -DSRCDIR="$(cygpath -w "$dir")" \
  -DOUTFILE="$(cygpath -w "$out/Sona-$version-windows-x64-setup.exe")" \
  "$(cygpath -w "$here/installer.nsi")"
ls -l "$out"
