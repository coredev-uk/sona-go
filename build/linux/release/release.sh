#!/usr/bin/env bash
# Builds Sona's release packages (.deb, .rpm, Arch, AppImage) on the Ubuntu
# 24.04 baseline in a container, then smoke-tests them on clean Ubuntu and
# Fedora containers (build/linux/release/smoke.sh).
#
#   build/linux/release/release.sh [output-dir]     # default: bin/release
#   SONA_SKIP_SMOKE=1 build/linux/release/release.sh
#
# The build runs on a snapshot of the working tree, so the host's
# node_modules, frontend/dist and bin/ are never touched. Secrets (.env,
# *.p8) and local databases are excluded from the snapshot.
#
# Needs podman or docker; falls back to `nix run nixpkgs#podman`.
set -euo pipefail

root=$(cd "$(dirname "$0")/../../.." && pwd)
out=$(realpath -m "${1:-$root/bin/release}")
here="$root/build/linux/release"
image=localhost/sona-release:ubuntu24.04

if command -v podman >/dev/null 2>&1; then
  engine=(podman)
elif command -v docker >/dev/null 2>&1; then
  engine=(docker)
else
  engine=(nix run nixpkgs#podman --)
fi

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

pull docker.io/library/ubuntu:24.04
"${engine[@]}" build "${policy[@]}" --pull=never -t "$image" -f "$here/Containerfile" "$here"

mkdir -p "$out"
rm -f "$out"/sona[-_]*

echo "release: building packages in $image"
tar -C "$root" \
  --exclude=./.git --exclude=./.devenv --exclude=./.direnv --exclude=./.task \
  --exclude=./bin --exclude=./build/bin --exclude=./build/linux/appimage \
  --exclude=./frontend/node_modules --exclude=./frontend/dist \
  --exclude='./.env' --exclude='./.env.*' --exclude='*.p8' \
  --exclude='*.db' --exclude='*.db-wal' --exclude='*.db-shm' \
  -cf - . |
  "${engine[@]}" run --rm -i \
    -v "$out:/out" \
    -v sona-release-cache:/cache \
    -e GOMODCACHE=/cache/gomod -e GOCACHE=/cache/gobuild -e npm_config_store_dir=/cache/pnpm \
    -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
    "$image" bash -euo pipefail -c '
      mkdir -p /src && tar -xf - -C /src && cd /src
      go tool wails3 package
      cp bin/sona_*.deb bin/sona-*.rpm bin/sona-*.pkg.tar.zst bin/sona-*.AppImage /out/
      ldd bin/sona >/out/sona.ldd.txt
      # docker writes as root; hand the files back (no-op under rootless podman).
      chown "$HOST_UID:$HOST_GID" /out/* 2>/dev/null || :
    '

ls -l "$out"

if [[ ${SONA_SKIP_SMOKE:-} == 1 ]]; then
  exit 0
fi

smoke() { # image, kind
  pull "$1"
  echo "release: smoke test ($2) on $1"
  "${engine[@]}" run --rm -v "$out:/pkg:ro" -v "$here/smoke.sh:/smoke.sh:ro" "$1" bash /smoke.sh "$2"
}
smoke docker.io/library/ubuntu:24.04 deb
smoke registry.fedoraproject.org/fedora:45 rpm
smoke registry.fedoraproject.org/fedora:45 appimage
echo "release: all smoke tests passed; packages in $out"
