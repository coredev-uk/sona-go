#!/usr/bin/env bash
# Builds Sona's release packages (.deb, .rpm, Arch, AppImage + .zsync) on the
# Ubuntu 24.04 baseline in a container, writes SHA256SUMS, then smoke-tests
# each package on a clean distro container (build/linux/release/smoke.sh):
# the .deb on Ubuntu 24.04, the .rpm and the AppImage on Fedora, the Arch
# package on Arch Linux.
#
#   build/linux/release/release.sh [output-dir]     # default: bin/release
#   SONA_SKIP_SMOKE=1 build/linux/release/release.sh
#   SONA_RELEASE_CACHE=~/.cache/sona-release build/linux/release/release.sh
#
# The build image is named by the hash of the Containerfile. It is taken from
# the local store if it is there, else pulled from
# ghcr.io/coredev-uk/sona-release:<hash> (.github/workflows/images.yml
# publishes it when the Containerfile changes; SONA_RELEASE_IMAGE_REGISTRY
# changes or empties the repository), and built here only if neither has it.
#
# The Go module and build caches live in the podman/docker
# volume sona-release-cache, or in the directory SONA_RELEASE_CACHE names
# (CI keeps that one with actions/cache).
#
# The version is VERSION's. SONA_LASTFM_KEY / SONA_LASTFM_SECRET and
# SONA_DISCORD_CLIENT_ID, when set, are passed to the build
# (build/linux/Taskfile.yml) and never printed.
#
# The build is MyGo's production build (mygo_noinspector,sqlite_fts5, with
# -X main.version from VERSION; build/linux/Taskfile.yml) on a snapshot of
# the working tree, so the host's bin/ is never touched. Secrets (.env,
# *.p8) and local databases are excluded from the snapshot.
#
# Needs podman or docker; falls back to `nix run nixpkgs#podman`.
set -euo pipefail

root=$(cd "$(dirname "$0")/../../.." && pwd)
out=$(realpath -m "${1:-$root/bin/release}")
here="$root/build/linux/release"
if command -v sha256sum >/dev/null 2>&1; then
  cfhash=$(sha256sum "$here/Containerfile" | cut -c1-16)
else
  cfhash=$(shasum -a 256 "$here/Containerfile" | cut -c1-16)
fi
image=localhost/sona-release:ubuntu24.04-$cfhash
registry=${SONA_RELEASE_IMAGE_REGISTRY-ghcr.io/coredev-uk/sona-release}
version=$(<"$root/VERSION")

if command -v podman >/dev/null 2>&1; then
  engine=(podman)
elif command -v docker >/dev/null 2>&1; then
  engine=(docker)
else
  engine=(nix run nixpkgs#podman --)
fi
# devenv exports LD_LIBRARY_PATH (GTK, GLib of its own nixpkgs pin); the
# container engine's helpers (conmon) must not load it: they are built against
# another glibc and fail with "GLIBC_2.xx not found".
engine=(env -u LD_LIBRARY_PATH "${engine[@]}")

# Rootless podman without a policy.json (e.g. NixOS without
# virtualisation.podman) refuses to pull or build; give it an accept-all
# policy for the public base images, for this script only.
policy=()
if [[ ${engine[0]} != docker && ! -e /etc/containers/policy.json && ! -e ${XDG_CONFIG_HOME:-$HOME/.config}/containers/policy.json ]]; then
  policy_file=$(mktemp)
  trap 'rm -f "$policy_file"' EXIT
  echo '{"default":[{"type":"insecureAcceptAnything"}]}' >"$policy_file"
  policy=(--signature-policy "$policy_file")
fi
pull() { "${engine[@]}" pull "${policy[@]}" "$1"; }

cache=sona-release-cache
if [[ -n ${SONA_RELEASE_CACHE:-} ]]; then
  mkdir -p "$SONA_RELEASE_CACHE"
  cache=$(realpath "$SONA_RELEASE_CACHE")
fi

if "${engine[@]}" image inspect "$image" >/dev/null 2>&1; then
  echo "release: build image $image is in the local store"
elif [[ -n $registry ]] && pull "$registry:$cfhash"; then
  "${engine[@]}" tag "$registry:$cfhash" "$image"
else
  echo "release: building $image (no published image for this Containerfile)"
  pull docker.io/library/ubuntu:24.04
  "${engine[@]}" build "${policy[@]}" --pull=never -t "$image" -f "$here/Containerfile" "$here"
fi

mkdir -p "$out"
rm -rf "$out"/sona[-_]* "$out/SHA256SUMS" "$out/aur"

echo "release: building $version packages in $image"
tar -C "$root" \
  --exclude=./.git --exclude=./.devenv --exclude=./.direnv --exclude=./.task \
  --exclude=./bin --exclude=./build/bin --exclude=./build/linux/appimage \
  --exclude='./.env' --exclude='./.env.*' --exclude='*.p8' \
  --exclude='*.db' --exclude='*.db-wal' --exclude='*.db-shm' \
  -cf - . |
  "${engine[@]}" run --rm -i \
    -v "$out:/out" \
    -v "$cache:/cache" \
    -e GOMODCACHE=/cache/gomod -e GOCACHE=/cache/gobuild \
    -e SONA_LASTFM_KEY -e SONA_LASTFM_SECRET -e SONA_DISCORD_CLIENT_ID \
    -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
    "$image" bash -euo pipefail -c '
      mkdir -p /src && tar -xf - -C /src && cd /src
      task linux:package
      cp bin/sona_*.deb bin/sona-*.rpm bin/sona-*.pkg.tar.zst bin/sona-*.AppImage bin/sona-*.AppImage.zsync /out/
      ldd bin/sona >/out/sona.ldd.txt
      # docker writes as root; hand the files back (no-op under rootless podman).
      chown "$HOST_UID:$HOST_GID" /out/* 2>/dev/null || :
    '

# Every artifact carries VERSION in its name.
for f in "sona_${version}_amd64.deb" "sona-$version-1.x86_64.rpm" "sona-$version-1-x86_64.pkg.tar.zst" \
  "sona-$version-x86_64.AppImage" "sona-$version-x86_64.AppImage.zsync"; do
  [[ -f $out/$f ]] || { echo "release: $f missing" >&2; exit 1; }
done
# AUR sona-bin files for this release's Arch package (attached to the
# release; the AUR maintainer copies them into the AUR repo).
bash "$root/build/linux/arch/update.sh" "$out/sona-$version-1-x86_64.pkg.tar.zst" "$out/aur"
tar -C "$out/aur" -czf "$out/sona-bin-$version-aur.tar.gz" PKGBUILD .SRCINFO
(cd "$out" && sha256sum sona[-_]* >SHA256SUMS)
ls -l "$out"

if [[ ${SONA_SKIP_SMOKE:-} == 1 ]]; then
  exit 0
fi

smoke() { # image, kind: output prefixed with the kind, the runs being parallel
  {
    pull "$1"
    echo "release: smoke test ($2) on $1"
    "${engine[@]}" run --rm -e SONA_VERSION="$version" -v "$out:/pkg:ro" -v "$here/smoke.sh:/smoke.sh:ro" "$1" bash /smoke.sh "$2"
  } 2>&1 | sed -u "s/^/[$2] /"
}
# Each package is smoke-tested on its own clean container, side by side.
pids=()
smoke docker.io/library/ubuntu:24.04 deb & pids+=($!)
smoke registry.fedoraproject.org/fedora:45 rpm & pids+=($!)
smoke registry.fedoraproject.org/fedora:45 appimage & pids+=($!)
smoke docker.io/library/archlinux:latest arch & pids+=($!)
failed=0
for pid in "${pids[@]}"; do wait "$pid" || failed=1; done
if [[ $failed == 1 ]]; then
  echo "release: a smoke test failed" >&2
  exit 1
fi
echo "release: all smoke tests passed; packages in $out"
