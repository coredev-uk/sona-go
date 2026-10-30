#!/usr/bin/env bash
# Builds and installs (--user) the Sona Flatpak from a release .deb.
#
#   build/linux/flatpak/build.sh [build-dir]       # default: /tmp/sona-flatpak/build
#   SONA_DEB=/path/to/sona_X_amd64.deb build/linux/flatpak/build.sh
#
# The .deb defaults to bin/release/sona_$(cat VERSION)_amd64.deb, produced by
# build/linux/release/release.sh. It is copied to bin/flatpak/sona.deb, the
# fixed name the manifest reads, so VERSION stays the only version source.
# flatpak-builder's state dir goes next to the build dir, outside the repo.
set -euo pipefail

root=$(cd "$(dirname "$0")/../../.." && pwd)
build=$(realpath -m "${1:-/tmp/sona-flatpak/build}")
version=$(<"$root/VERSION")
deb=${SONA_DEB:-$root/bin/release/sona_${version}_amd64.deb}

if [[ ! -f $deb ]]; then
  echo "flatpak: $deb not found; run build/linux/release/release.sh first or set SONA_DEB" >&2
  exit 1
fi
mkdir -p "$root/bin/flatpak"
cp "$deb" "$root/bin/flatpak/sona.deb"

builder=(flatpak-builder)
command -v flatpak-builder >/dev/null 2>&1 || builder=(nix shell nixpkgs#flatpak nixpkgs#flatpak-builder -c flatpak-builder)

"${builder[@]}" --user --install --force-clean --disable-rofiles-fuse \
  --state-dir="$build.state" "$build" "$root/build/linux/flatpak/dev.coredev.Sona.yml"
