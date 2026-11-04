#!/usr/bin/env bash
# Fetches the official GStreamer macOS packages (runtime and development),
# checks them against gstreamer.sha256 and installs them.
#
#   build/darwin/install-gstreamer.sh            # sudo installer: /Library/Frameworks
#   build/darwin/install-gstreamer.sh --extract DIR
#       unpacks without installing or sudo; GSTREAMER_ROOT for bundle.sh is then
#       DIR/Library/Frameworks/GStreamer.framework/Versions/1.0 (printed)
#
# The pinned version and the SHA-256 of each package are in gstreamer.sha256;
# change both together (same 1.28 minor as devenv's GStreamer).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
version=$(awk '$1=="version"{print $2}' "$here/gstreamer.sha256")
cache=${GSTREAMER_PKG_CACHE:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gstreamer-pkg}
mkdir -p "$cache"
for kind in "" "devel-"; do
  name=gstreamer-1.0-${kind}${version}-universal.pkg
  if [[ ! -f $cache/$name ]]; then
    curl -fsSL --retry 3 -o "$cache/$name" "https://gstreamer.freedesktop.org/data/pkg/osx/$version/$name"
  fi
  want=$(awk -v n="$name" '$2==n{print $1}' "$here/gstreamer.sha256")
  got=$(shasum -a 256 "$cache/$name" | awk '{print $1}')
  [[ -n $want && $got == "$want" ]] || { echo "install-gstreamer: $name sha256 $got, want ${want:-<none>}" >&2; exit 1; }
done
if [[ ${1:-} == --extract ]]; then
  dest=${2:?--extract DIR}
  root=$dest/Library/Frameworks/GStreamer.framework/Versions/1.0
  rm -rf "$dest"
  mkdir -p "$root"
  for kind in "" "devel-"; do
    pkgutil --expand-full "$cache/gstreamer-1.0-${kind}${version}-universal.pkg" "$dest/${kind}pkgs"
    # Each component's payload is rooted at its install-location.
    for pkg in "$dest/${kind}pkgs"/*.pkg; do
      loc=$(sed -n 's|.*install-location="\([^"]*\)".*|\1|p' "$pkg/PackageInfo" | head -n 1)
      mkdir -p "$dest$loc"
      ditto "$pkg/Payload" "$dest$loc"
    done
    rm -rf "$dest/${kind}pkgs"
  done
  echo "$root"
else
  for kind in "" "devel-"; do
    sudo installer -pkg "$cache/gstreamer-1.0-${kind}${version}-universal.pkg" -target /
  done
fi
