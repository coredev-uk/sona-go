#!/usr/bin/env bash
# Builds Sona for Windows x64 (run in Git Bash on windows-2025 with MinGW-w64
# gcc, Go and makensis on PATH).
#
#   build/windows/package.sh OUTDIR
#
# Writes into OUTDIR: Sona-<version>-windows-x64.zip (portable),
# Sona-<version>-windows-x64-setup.exe (per-user NSIS installer) and
# sona-<version>-windows-amd64.tar.gz (the update archive: the same files,
# at the top level).
#
# Environment: VERSION (default: the VERSION file); UPDATE_FEED, UPDATE_KEY
# (linked in for self-updates; unset = updates off); SONA_LASTFM_KEY,
# SONA_LASTFM_SECRET, SONA_DISCORD_CLIENT_ID as build/linux.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
out=${1:?usage: package.sh OUTDIR}
mkdir -p "$out"
out=$(cd "$out" && pwd)
version=${VERSION:-$(cat "$repo/VERSION")}

# makensis wants a four-part number: 0.3.0-rc.1 → 0.3.0.0.
numeric=$(echo "${version%%-*}.0.0.0.0" | cut -d. -f1-4)

ldflags="-w -s -H windowsgui -X main.version=$version -X github.com/egoist/mygo.production=1"
[[ -n ${UPDATE_FEED:-} ]] && ldflags+=" -X github.com/egoist/mygo.packageUpdateFeed=$UPDATE_FEED"
[[ -n ${UPDATE_KEY:-} ]] && ldflags+=" -X github.com/egoist/mygo.packageUpdateKey=$UPDATE_KEY"
[[ -n ${SONA_LASTFM_KEY:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=$SONA_LASTFM_KEY"
[[ -n ${SONA_LASTFM_SECRET:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=$SONA_LASTFM_SECRET"
[[ -n ${SONA_DISCORD_CLIENT_ID:-} ]] && ldflags+=" -X github.com/coredev-uk/sona-go/internal/discord.clientID=$SONA_DISCORD_CLIENT_ID"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
dir=$work/Sona
mkdir "$dir"
(
  cd "$repo"
  GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -tags mygo_noinspector,sqlite_fts5 -trimpath -buildvcs=false \
    -ldflags "$ldflags" -o "$dir/sona.exe" ./cmd/sona
)
cp "$here/sona.ico" "$dir/sona.ico"

# Portable zip (the folder is Sona\), the updater's tar.gz (no wrapper
# folder) and the installer.
rm -f "$out/Sona-$version-windows-x64.zip"
(cd "$work" && 7z a -tzip -bso0 -bsp0 "$out/Sona-$version-windows-x64.zip" Sona)
tar -C "$dir" -czf "$out/sona-$version-windows-amd64.tar.gz" .
makensis -V2 -DVERSION="$version" -DVERSION_NUMERIC="$numeric" -DSRCDIR="$(cygpath -w "$dir" 2>/dev/null || echo "$dir")" \
  -DOUTFILE="$(cygpath -w "$out/Sona-$version-windows-x64-setup.exe" 2>/dev/null || echo "$out/Sona-$version-windows-x64-setup.exe")" \
  "$(cygpath -w "$here/installer.nsi" 2>/dev/null || echo "$here/installer.nsi")"
ls -l "$out"
