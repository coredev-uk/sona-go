#!/usr/bin/env bash
# Builds the drag-to-install disk image (docs/maintainers/release.md, "Platforms
# in a release"): Sona.app and an Applications alias, on the background in
# build/darwin/dmg, in a window of the size and layout dmg/settings.py sets.
#
#   build/darwin/dmg.sh Sona.app OUT.dmg
#
# dmgbuild (install-dmgbuild.sh, pinned in dmgbuild.sha256) writes the window's
# .DS_Store and the background's alias record itself, so the build needs no
# Finder, no AppleScript and no GUI session: it is the same bytes of layout on
# the CI runner as on a Mac. smoke.sh reads the layout back.
#
# Environment:
#   DMGBUILD_DIR   where install-dmgbuild.sh put dmgbuild (default bin/dmgbuild)
#   PYTHON         Python 3.11 or later (default python3)
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
app=${1:?usage: dmg.sh Sona.app OUT.dmg}
out=${2:?usage: dmg.sh Sona.app OUT.dmg}
tools=${DMGBUILD_DIR:-$repo/bin/dmgbuild}
[[ -f $app/Contents/Resources/Sona.icns ]] || { echo "dmg: $app is not Sona.app" >&2; exit 1; }
[[ -f $tools/dmgbuild/__main__.py ]] || { echo "dmg: no dmgbuild in $tools (build/darwin/install-dmgbuild.sh)" >&2; exit 1; }
for f in background.png background@2x.png; do
  [[ -f $here/dmg/$f ]] || { echo "dmg: no $f (build/darwin/dmg/render.sh)" >&2; exit 1; }
done

rm -f "$out"
# dmgbuild runs with PATH and HOME and nothing else of this environment: none
# of the release secrets bundle.sh may hold, and no devenv DEVELOPER_DIR, whose
# Nix SDK has no SetFile (macOS's own, which dmgbuild runs for the volume's
# icon, is found through /usr/bin).
env -i PATH="$PATH" HOME="${HOME:-/var/empty}" TMPDIR="${TMPDIR:-/tmp}" PYTHONUTF8=1 PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$tools" \
  "${PYTHON:-python3}" -m dmgbuild -s "$here/dmg/settings.py" -D "app=$app" -D "art=$here/dmg" Sona "$out"
echo "dmg: $out"
