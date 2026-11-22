#!/usr/bin/env bash
# Renders the .dmg window's background (background.svg) to the 1x and 2x PNGs
# that sit beside it. Run it after editing the SVG and commit all three:
#
#   devenv shell -- build/darwin/dmg/render.sh
#
# resvg comes from devenv. The type is the Schibsted Grotesk Sona embeds, loaded
# from the file (no system font is used, so the render is the same everywhere).
# Finder reads a 2x picture by its resolution, so the 2x PNG is marked 144 dpi
# (sips, which ships with macOS); dmg.sh's tiffutil check fails on a mismatch.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
font=$repo/internal/ui/theme/fonts/SchibstedGrotesk-Variable.ttf
command -v resvg >/dev/null || { echo "render: no resvg (run this in devenv shell)" >&2; exit 1; }
command -v sips >/dev/null || { echo "render: no sips (run this on a Mac)" >&2; exit 1; }
for scale in 1 2; do
  out=$here/background.png
  [[ $scale == 2 ]] && out=$here/background@2x.png
  resvg --skip-system-fonts --use-font-file "$font" -z "$scale" "$here/background.svg" "$out"
  sips -s dpiWidth $((72 * scale)) -s dpiHeight $((72 * scale)) "$out" >/dev/null
done
echo "render: background.png and background@2x.png written to $here"
