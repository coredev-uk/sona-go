# dmgbuild settings for Sona's disk image (build/darwin/dmg.sh): the window a
# user sees on opening it. dmgbuild writes the window's .DS_Store and the
# background's alias itself, so no Finder and no AppleScript is involved and
# the build needs no GUI session. smoke.sh reads the .DS_Store back against
# these values (dmgcheck.py executes this file), so they live here only.
#
# Defines (dmg.sh passes them): app, the Sona.app to ship; art, this folder.
import os

app = defines["app"]  # noqa: F821
art = defines["art"]  # noqa: F821

# The picture is 640 x 400 points (background.png, and background@2x.png at
# 1280 x 800; dmgbuild joins them into one TIFF with tiffutil -cathidpicheck).
# Finder draws it from the top-left corner of the window's content, and a
# window's height includes its title bar: 32 pt on macOS 26 and later (toolbar
# and path bar hidden; measured), about 28 pt on macOS 14 and 15. The frame is
# made for 28, so macOS 26 crops the picture's last 4 pt, which the art leaves
# empty.
ART_WIDTH, ART_HEIGHT = 640, 400
TITLE_BAR = 28
WINDOW_POSITION = (200, 120)

# A label is drawn in black over any picture, in light and dark mode alike, so
# the board is light. Sona.app is not touched in the image: hiding its
# extension would set Finder information on the bundle, which makes
# `codesign --verify --strict` fail ("detritus not allowed") and risks
# Gatekeeper calling the app damaged after a drag install.
volume_name = "Sona"
files = [app]
symlinks = {"Applications": "/Applications"}
icon = os.path.join(app, "Contents", "Resources", "Sona.icns")  # the volume's
background = os.path.join(art, "background.png")

# ULMO (LZMA) is a quarter smaller than UDZO and every macOS Sona runs on
# (build/darwin/MIN_MACOS, 14) opens it.
format = "ULMO"
filesystem = "HFS+"

window_rect = (WINDOW_POSITION, (ART_WIDTH, ART_HEIGHT + TITLE_BAR))
default_view = "icon-view"
show_toolbar = False
show_pathbar = False
show_sidebar = False
show_status_bar = False
show_tab_view = False

# Two 128 pt icons on one line, the arrow in the art between them.
icon_size = 128
text_size = 13
label_pos = "bottom"
arrange_by = None
icon_locations = {
    "Sona.app": (165, 180),
    "Applications": (475, 180),
}
