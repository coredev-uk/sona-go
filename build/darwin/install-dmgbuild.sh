#!/usr/bin/env bash
# Installs dmgbuild, the tool that writes the .dmg window's layout without
# Finder or AppleScript (dmg.sh), at the versions dmgbuild.sha256 pins. Each
# package is a pure-Python wheel; a wheel is downloaded from PyPI, checked
# against its SHA-256 and unpacked, with no pip, no resolver and nothing built
# at install time. A later version, or any other bytes, fail the build before
# the tool can touch a release artefact. bundle.sh runs this first; run it
# by hand to use dmg.sh or smoke.sh alone.
#
#   build/darwin/install-dmgbuild.sh [DIR]
#
# DIR defaults to $DMGBUILD_DIR, else bin/dmgbuild; dmg.sh and smoke.sh look
# there. Needs Python 3.11 or later (PYTHON, default python3; the macOS runners
# have one). Installing again on an unchanged pin file does nothing.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
dir=${1:-${DMGBUILD_DIR:-$repo/bin/dmgbuild}}
pins=$here/dmgbuild.sha256
python=${PYTHON:-python3}

"$python" -c 'import sys; sys.exit(sys.version_info < (3, 11))' 2>/dev/null ||
  { echo "install-dmgbuild: dmgbuild needs Python 3.11 or later ($python is $("$python" --version 2>&1))" >&2; exit 1; }

stamp=$(shasum -a 256 "$pins" | awk '{print $1}')
if [[ -f $dir/.pins && $(cat "$dir/.pins") == "$stamp" ]]; then
  echo "install-dmgbuild: $dir is up to date"
  exit 0
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir "$work/site"
while read -r pkg version want; do
  [[ -n $pkg && $pkg != '#'* ]] || continue
  wheel=$pkg-$version-py3-none-any.whl
  # PyPI's short form of a wheel's URL (it redirects to the hashed path).
  curl -fsSL --retry 5 --retry-all-errors --connect-timeout 30 \
    -o "$work/$wheel" "https://files.pythonhosted.org/packages/py3/${pkg:0:1}/$pkg/$wheel"
  got=$(shasum -a 256 "$work/$wheel" | awk '{print $1}')
  [[ $got == "$want" ]] || { echo "install-dmgbuild: $wheel sha256 $got, want $want" >&2; exit 1; }
  unzip -q "$work/$wheel" -d "$work/site"
  echo "install-dmgbuild: $pkg $version ok"
done <"$pins"
[[ -f $work/site/dmgbuild/__main__.py && -d $work/site/ds_store && -d $work/site/mac_alias ]] ||
  { echo "install-dmgbuild: the wheels did not unpack to dmgbuild, ds_store and mac_alias" >&2; exit 1; }
# Everything the tool imports resolves inside the unpacked folder. This is the
# one time the packages' code runs here, with PATH and nothing else of the
# environment (a release build holds secrets).
env -i PATH="$PATH" PYTHONPATH="$work/site" PYTHONDONTWRITEBYTECODE=1 "$python" -c 'import dmgbuild, ds_store, mac_alias' ||
  { echo "install-dmgbuild: the pinned packages do not import" >&2; exit 1; }

rm -rf "$dir"
mkdir -p "$(dirname "$dir")"
mv "$work/site" "$dir"
echo "$stamp" >"$dir/.pins"
echo "install-dmgbuild: installed in $dir"
