"""Reads Sona's windows through AT-SPI for the desktop VM tests.

    vmcheck [--wait SECONDS]

Prints one JSON object: Sona's top-level windows ("frames", with their
size), the title bar's window buttons (GTK's own, which MyGo shows on a
hidden title bar: Close, Minimize, Maximize or Restore, with their position
in the window; none where the compositor decorates the window) and the page's
headings. It waits until the main window's page has rendered (a heading or
a window button), or fails after --wait seconds.
"""

import argparse
import json
import sys
import time

import gi

gi.require_version("Atspi", "2.0")
from gi.repository import Atspi, GLib  # noqa: E402

APP_NAMES = {"dev.coredev.Sona", "Sona", "sona"}
WINDOW_BUTTONS = {"Close", "Minimize", "Maximize", "Restore"}


def sona_apps():
    desktop = Atspi.get_desktop(0)
    for i in range(desktop.get_child_count()):
        app = desktop.get_child_at_index(i)
        try:
            if app is not None and app.get_name() in APP_NAMES:
                yield app
        except GLib.Error:
            continue


def walk(obj, depth=0):
    yield obj
    if depth > 60:
        return
    try:
        count = obj.get_child_count()
    except GLib.Error:
        return
    for i in range(count):
        try:
            child = obj.get_child_at_index(i)
        except GLib.Error:
            continue
        if child is not None:
            yield from walk(child, depth + 1)


def extents(obj, coords):
    r = obj.get_extents(coords)
    return {"x": r.x, "y": r.y, "w": r.width, "h": r.height}


def snapshot():
    frames, buttons, headings = [], [], []
    for app in sona_apps():
        for i in range(app.get_child_count()):
            frame = app.get_child_at_index(i)
            if frame is None or frame.get_role() != Atspi.Role.FRAME:
                continue
            frames.append({"name": frame.get_name(), **extents(frame, Atspi.CoordType.SCREEN)})
            for obj in walk(frame):
                try:
                    role, name = obj.get_role(), obj.get_name()
                except GLib.Error:
                    continue
                if role == Atspi.Role.BUTTON and name in WINDOW_BUTTONS:
                    buttons.append({"frame": frame.get_name(), "name": name, **extents(obj, Atspi.CoordType.WINDOW)})
                elif role == Atspi.Role.HEADING and name:
                    headings.append(name)
    return {"frames": frames, "buttons": buttons, "headings": headings}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--wait", type=float, default=0)
    args = parser.parse_args()
    Atspi.init()
    deadline = time.monotonic() + args.wait
    while True:
        snap = snapshot()
        if snap["headings"] or snap["buttons"]:
            print(json.dumps(snap))
            return 0
        if time.monotonic() >= deadline:
            print(json.dumps(snap))
            print("vmcheck: Sona's page is not on the accessibility bus", file=sys.stderr)
            return 1
        time.sleep(2)


if __name__ == "__main__":
    sys.exit(main())
