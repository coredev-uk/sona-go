#!/usr/bin/env bash
# Installs one of the Chocolatey packages the Windows build runs (nsis for
# makensis, pkgconfiglite for pkg-config), at the version tools.sha256 pins,
# and checks the files it installed against that file's tree hash. A later
# package version, or one whose installer fetched something else, fails the
# job before the tool can touch a release artefact (run in Git Bash on
# windows-2025).
#
#   build/windows/install-tool.sh nsis|pkgconfiglite
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
pkg=${1:?usage: install-tool.sh nsis|pkgconfiglite}
read -r version want < <(awk -v p="$pkg" '$1==p {print $2, $3}' "$here/tools.sha256")
[[ -n ${version:-} ]] || { echo "install-tool: $pkg is not pinned in tools.sha256" >&2; exit 1; }
case $pkg in
nsis) dir="/c/Program Files (x86)/NSIS" ;;
pkgconfiglite) dir="/c/ProgramData/chocolatey/lib/pkgconfiglite/tools" ;;
*) echo "install-tool: unknown package $pkg" >&2; exit 1 ;;
esac

choco install "$pkg" --version "$version" --yes --no-progress --limit-output
[[ -d $dir ]] || { echo "install-tool: $pkg installed no $dir" >&2; exit 1; }
# From stdin: Git Bash's sha256sum puts a backslash before the hash of a path with one.
got=$(cd "$dir" && LC_ALL=C find . -type f -print0 | LC_ALL=C sort -z | while IFS= read -r -d '' f; do
  printf '%s  %s\n' "$(sha256sum <"$f" | awk '{print $1}')" "$f"
done | sha256sum | awk '{print $1}')
[[ $got == "$want" ]] || { echo "install-tool: $pkg $version tree sha256 $got, want $want" >&2; exit 1; }
echo "install-tool: $pkg $version ok"
