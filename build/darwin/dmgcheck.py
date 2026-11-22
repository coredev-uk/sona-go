#!/usr/bin/env python3
"""Checks the layout of a mounted Sona disk image (build/darwin/smoke.sh).

    PYTHONPATH=<dmgbuild folder> python3 dmgcheck.py MOUNTPOINT

Reads the window's .DS_Store with dmgbuild's own reader (ds_store, mac_alias)
and compares it with dmg/settings.py, which sets it: the window's bounds, the
view and its icon size, the two icon positions, the background's alias, and
the chrome that must be hidden. Then looks at the files: the background is a
1x and 2x TIFF at 72 and 144 dpi, the volume has Sona's icon, and the visible
contents are Sona.app and an Applications alias, nothing else. Prints every
mismatch and exits 1 if there is one.
"""
import os
import plistlib
import re
import subprocess
import sys

from ds_store import DSStore
from mac_alias import Alias

here = os.path.dirname(os.path.abspath(__file__))
mount = sys.argv[1]
errors = []


def check(ok, what):
    if not ok:
        errors.append(what)


# What settings.py asks for (it reads `defines`; the app is not opened here).
want = {"defines": {"app": "/Sona.app", "art": os.path.join(here, "dmg")}}
settings = os.path.join(here, "dmg", "settings.py")
with open(settings, encoding="utf-8") as f:
    exec(compile(f.read(), settings, "exec"), want, want)
(wx, wy), (ww, wh) = want["window_rect"]

entries = {}
with DSStore.open(os.path.join(mount, ".DS_Store"), "r") as ds:
    for e in ds:
        entries[(e.filename, e.code)] = e.value


def entry(name, code):
    v = entries.get((name, code))
    check(v is not None, f".DS_Store has no {code.decode()} entry for {name!r}")
    return v


def plist(v):
    return v if isinstance(v, dict) else plistlib.loads(v)


bwsp = plist(entry(".", b"bwsp") or {})
check(bwsp.get("WindowBounds") == f"{{{{{wx}, {wy}}}, {{{ww}, {wh}}}}}",
      f"window bounds are {bwsp.get('WindowBounds')!r}, want {want['window_rect']}")
for key in ("ShowToolbar", "ShowSidebar", "ShowPathbar", "ShowStatusBar", "ShowTabView", "ContainerShowSidebar"):
    check(bwsp.get(key) is False, f"{key} is {bwsp.get(key)!r}, want it hidden (False)")

check(entry(".", b"icvl") == b"icnv", f"the view is {entries.get(('.', b'icvl'))!r}, want icon view (icnv)")
icvp = plist(entry(".", b"icvp") or {})
check(icvp.get("iconSize") == 128.0, f"icon size is {icvp.get('iconSize')!r}, want 128")
check(icvp.get("textSize") == float(want["text_size"]), f"label size is {icvp.get('textSize')!r}, want {want['text_size']}")
check(icvp.get("arrangeBy") == "none", f"icons are arranged by {icvp.get('arrangeBy')!r}, want none (positions are fixed)")
check(icvp.get("labelOnBottom") is True, "labels are not under the icons")
check(icvp.get("backgroundType") == 2, f"background type is {icvp.get('backgroundType')!r}, want 2 (a picture)")
alias = icvp.get("backgroundImageAlias")
check(alias is not None, "the view names no background picture")
if alias is not None:
    a = Alias.from_bytes(alias)
    check(a.target.filename == ".background.tiff", f"the background is {a.target.filename!r}, want '.background.tiff'")
    check(a.volume.name == want["volume_name"], f"the background's volume is {a.volume.name!r}, want {want['volume_name']!r}")

locs = {name: entry(name, b"Iloc") for name in want["icon_locations"]}
for name, pos in want["icon_locations"].items():
    check(locs[name] == pos, f"{name} is at {locs[name]}, want {pos}")
check(len({p[1] for p in locs.values() if p}) == 1, f"the icons are not on one line: {locs}")
for name, p in locs.items():
    # A 128 pt icon wholly inside the picture, which sits at the window's top-left.
    check(p is not None and 64 <= p[0] <= want["ART_WIDTH"] - 64 and 64 <= p[1] <= want["ART_HEIGHT"] - 64,
          f"{name} at {p} does not sit on the {want['ART_WIDTH']} x {want['ART_HEIGHT']} picture")

# The background: one TIFF holding the 1x and the 2x picture.
tiff = os.path.join(mount, ".background.tiff")
check(os.path.isfile(tiff), "no .background.tiff")
if os.path.isfile(tiff):
    info = subprocess.run(["tiffutil", "-info", tiff], capture_output=True, text=True).stdout
    sizes = [(int(w), int(h)) for w, h in re.findall(r"Image Width: (\d+) Image Length: (\d+)", info)]
    dpis = [int(float(d)) for d in re.findall(r"Resolution: ([\d.]+), [\d.]+", info)]
    w, h = want["ART_WIDTH"], want["ART_HEIGHT"]
    check(sizes == [(w, h), (2 * w, 2 * h)], f"the background holds {sizes}, want 1x and 2x: {[(w, h), (2 * w, 2 * h)]}")
    check(dpis == [72, 144], f"the background's resolutions are {dpis}, want [72, 144] (a 2x picture is read by its dpi)")

# The volume's own icon, and the contents.
check(os.path.isfile(os.path.join(mount, ".VolumeIcon.icns")), "no .VolumeIcon.icns")
finder_info = subprocess.run(["xattr", "-px", "com.apple.FinderInfo", mount], capture_output=True, text=True).stdout.split()
check(len(finder_info) > 8 and int(finder_info[8], 16) & 0x04, "the volume is not flagged as having a custom icon")
# Sona.app is the signed bundle as built: no Finder information on it (a hidden-extension flag
# would fail `codesign --strict`, and Gatekeeper may then call the app damaged).
check(subprocess.run(["xattr", "-p", "com.apple.FinderInfo", os.path.join(mount, "Sona.app")], capture_output=True).returncode != 0,
      "Sona.app carries Finder information (codesign --strict refuses it)")
visible = sorted(n for n in os.listdir(mount) if not n.startswith("."))
check(visible == ["Applications", "Sona.app"], f"the image holds {visible}, want ['Applications', 'Sona.app']")
check(os.path.islink(os.path.join(mount, "Applications")) and os.readlink(os.path.join(mount, "Applications")) == "/Applications",
      "Applications is not an alias (symlink) to /Applications")

for e in errors:
    print(f"dmgcheck: {e}", file=sys.stderr)
sys.exit(1 if errors else 0)
