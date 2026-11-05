#!/usr/bin/env bash
# Builds the Sona Flatpak from the working tree and installs it (--user).
#
#   build/linux/flatpak/build.sh [build-dir]       # default: /tmp/sona-flatpak/build
#   SONA_FLATPAK_BUNDLE=sona.flatpak build/linux/flatpak/build.sh
#
# The build is offline: Go modules come from go-sources.json (regenerate it
# after a go.mod change, see docs/flatpak.md), the toolchain from
# org.freedesktop.Sdk.Extension.golang and the GNOME runtime from Flathub. Install
# those once first:
#
#   flatpak --user remote-add --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
#   flatpak --user install flathub org.gnome.Platform//51 org.gnome.Sdk//51 \
#     org.freedesktop.Sdk.Extension.golang//26.08
#
# flatpak-builder's state dir goes next to the build dir, outside the repo.
# SONA_FLATPAK_BUNDLE also writes a single-file bundle to that path.
set -euo pipefail

root=$(cd "$(dirname "$0")/../../.." && pwd)
build=$(realpath -m "${1:-/tmp/sona-flatpak/build}")

builder=(flatpak-builder)
if ! command -v flatpak-builder >/dev/null 2>&1; then
  # NixOS, without flatpak installed. devenv's LD_LIBRARY_PATH carries a
  # newer glibc's libraries than nixpkgs' flatpak, so drop it.
  builder=(env -u LD_LIBRARY_PATH nix shell nixpkgs#flatpak nixpkgs#flatpak-builder nixpkgs#bubblewrap -c flatpak-builder)
fi

args=(--user --install --force-clean --disable-rofiles-fuse --state-dir="$build.state")
[[ -n ${SONA_FLATPAK_BUNDLE:-} ]] && args+=(--repo="$build.repo")

"${builder[@]}" "${args[@]}" "$build" "$root/build/linux/flatpak/dev.coredev.Sona.yml"

if [[ -n ${SONA_FLATPAK_BUNDLE:-} ]]; then
  bundler=(flatpak)
  command -v flatpak >/dev/null 2>&1 || bundler=(env -u LD_LIBRARY_PATH nix shell nixpkgs#flatpak -c flatpak)
  "${bundler[@]}" build-bundle "$build.repo" "$SONA_FLATPAK_BUNDLE" dev.coredev.Sona
fi
