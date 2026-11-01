# Helpers both desktop tests share (prepended to their test scripts).
import json
import re
import shlex
import time

BUS = "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus XDG_RUNTIME_DIR=/run/user/1000"
LOG = "/home/alice/.local/state/sona/sona.log"
# The wrapper execs .sona-wrapped under the name it was started as, so
# match the process name (its CDM and GStreamer children are short-lived).
SONA_PIDS = "pgrep -u alice -x '[.]sona-wrapped'"
QUEUE_TITLE = "VM Queue Song"


def as_alice(cmd):
    """The shell command running cmd as alice, on her session bus."""
    return f"su - alice -c {shlex.quote(BUS + ' ' + cmd)}"


def user(cmd):
    """Runs cmd as alice and returns its stdout; fails the test on an error."""
    return machine.succeed(as_alice(cmd))


def busctl_json(args):
    return json.loads(user(f"busctl --user --json=short {args}"))


def session_env(name):
    """A variable of the session's environment (systemd --user's)."""
    for line in user("systemctl --user show-environment").splitlines():
        key, _, value = line.partition("=")
        if key == name:
            return value
    raise Exception(f"{name} is not in the session's environment")


def check_x11_window():
    """Sona's window is an X11 window on the session's XWayland."""
    env = f"DISPLAY={session_env('DISPLAY')}"
    try:
        env += f" XAUTHORITY={session_env('XAUTHORITY')}"
    except Exception:
        # Mutter's and KWin's XWayland cookies.
        auth = machine.succeed("ls /run/user/1000/.mutter-Xwaylandauth.* /run/user/1000/xauth_* 2>/dev/null || true").split()
        if auth:
            env += f" XAUTHORITY={auth[0]}"
    tree = user(f"{env} xwininfo -root -tree")
    assert '"Sona"' in tree, tree


def log_size():
    return int(machine.succeed(f"stat -c %s {LOG} 2>/dev/null || echo 0").strip())


def log_since(offset):
    return machine.succeed(f"tail -c +{offset + 1} {LOG} 2>/dev/null || true")


def wait_log(offset, pattern, timeout=120):
    """Waits until Sona's log, after offset, matches pattern."""
    deadline = time.monotonic() + timeout
    while True:
        text = log_since(offset)
        if re.search(pattern, text):
            return text
        if time.monotonic() > deadline:
            raise Exception(f"Sona's log never matched {pattern!r}; it reads:\n{text}")
        time.sleep(1)


def sona_pids():
    return machine.succeed(f"{SONA_PIDS} || true").split()


def seed_queue():
    """Gives Sona a saved queue to restore (paused): MPRIS shows its track."""
    user("install -D -m600 /etc/sona-vm/queue.json ~/.local/state/sona/queue.json")


def start_sona(unit, env=None, cmd="sona"):
    """Starts Sona as a transient user unit in the session's environment;
    returns the log offset its lines start after."""
    flags = " ".join(f"-E {k}={v}" for k, v in (env or {}).items())
    offset = log_size()
    user(f"systemd-run --user --collect --unit={unit} {flags} {cmd}")
    return offset


def window(wait=120):
    """Sona's windows, title bar buttons and headings, read over AT-SPI."""
    return json.loads(user(f"vmcheck --wait {wait}"))


def mpris_names():
    names = busctl_json("call org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus ListNames")["data"][0]
    return [n for n in names if n.startswith("org.mpris.MediaPlayer2.")]


def check_running(offset, cli="sona"):
    """The checks every start shares: first paint, private mode, offline."""
    text = wait_log(offset, r"startup: first paint")
    paint = re.search(r".*startup: first paint.*", text)
    assert paint is not None
    machine.log("first paint: " + paint.group(0))
    # The last line: appimage-run prints its own before the command's.
    out = user(f"{cli} private status").strip().splitlines()[-1]
    assert out == "private listening: on (forced by SONA_PRIVATE=1)", out
    w = window()
    sona_frames = [f for f in w["frames"] if f["name"] == "Sona"]
    assert len(sona_frames) == 1, w
    return w, sona_frames[0]


def check_offline(offset):
    """Offline: the CDM download fails and is logged; Sona keeps running."""
    text = wait_log(offset, r'"msg":"widevine: [^"]*(fail|unreach|offline|err)', timeout=180)
    machine.log("CDM offline:\n" + "\n".join(l for l in text.splitlines() if "widevine" in l.lower()))
    assert sona_pids(), "Sona exited after the CDM download failed"


def check_mpris_bus():
    assert mpris_names() == ["org.mpris.MediaPlayer2.sona"], mpris_names()
    assert "sona" in user("playerctl -l").split()
    title = user("playerctl -p sona metadata title").strip()
    assert title == QUEUE_TITLE, title
    status = user("playerctl -p sona status").strip()
    assert status in ("Paused", "Stopped"), status
    entry = busctl_json(
        "get-property org.mpris.MediaPlayer2.sona /org/mpris/MediaPlayer2 org.mpris.MediaPlayer2 DesktopEntry"
    )["data"]
    assert entry == "dev.coredev.Sona", entry


def tray_items():
    """The StatusNotifierItems the shell's watcher knows."""
    return busctl_json(
        "get-property org.kde.StatusNotifierWatcher /StatusNotifierWatcher "
        "org.kde.StatusNotifierWatcher RegisteredStatusNotifierItems"
    )["data"]


def sona_bus_names():
    """The session bus names (unique and well-known) Sona's process owns."""
    pids = {int(p) for p in sona_pids()}
    names = busctl_json("call org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus ListNames")["data"][0]
    owned = []
    for name in names:
        try:
            pid = busctl_json(
                "call org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus "
                f"GetConnectionUnixProcessID s {name}"
            )["data"][0]
        except Exception:
            continue
        if pid in pids:
            owned.append(name)
    return owned


def check_tray():
    """Sona exports its StatusNotifierItem (Title "Sona", its menu) and the
    shell's watcher lists an item; returns (Sona's item names, the watcher's
    list) for the desktop's own check that the shell took Sona's."""
    machine.wait_until_succeeds(as_alice("busctl --user status org.kde.StatusNotifierWatcher"), timeout=60)
    deadline = time.monotonic() + 60
    while True:
        names = sona_bus_names()
        items = tray_items()
        for name in names:
            try:
                title = busctl_json(f"get-property {name} /StatusNotifierItem org.kde.StatusNotifierItem Title")["data"]
            except Exception:
                continue
            if title == "Sona" and items:
                menu = busctl_json(f"get-property {name} /StatusNotifierItem org.kde.StatusNotifierItem Menu")["data"]
                # Depth 8 rather than -1 (all), which busctl would read as an option.
                layout = user(f"busctl --user call {name} {menu} com.canonical.dbusmenu GetLayout iias 0 8 0")
                for label in ["Show Sona", "Mini player", "Quit Sona"]:
                    assert label in layout, layout
                machine.log(f"tray: Sona's names {names}, watcher's items {items}; menu {layout.strip()}")
                return names, items
        if time.monotonic() > deadline:
            raise Exception(f"no tray item: Sona owns {names}, the watcher lists {items}")
        time.sleep(1)


def check_single_instance(cli="sona"):
    """A second launch with a link forwards it and exits; one Sona stays."""
    pids = sona_pids()
    offset = log_size()
    t0 = time.monotonic()
    user(f"timeout 30 {cli} music://music.apple.com/us/album/1440857781")
    took = time.monotonic() - t0
    wait_log(offset, r"forwarded to the running instance", timeout=10)
    machine.log(f"second launch exited after {took:.2f} s")
    time.sleep(2)
    assert sona_pids() == pids, (pids, sona_pids())
    assert mpris_names() == ["org.mpris.MediaPlayer2.sona"], mpris_names()
    frames = [f for f in window()["frames"] if f["name"] == "Sona"]
    assert len(frames) == 1, frames


def start_notifyprobe(unit="notifyprobe"):
    """Shows two track notifications through Sona's client, the second
    replacing the first; returns the probe's output file."""
    out = "/home/alice/notifyprobe.out"
    user(
        f"systemd-run --user --collect --unit={unit} -p StandardOutput=truncate:{out} "
        "notifyprobe -image /run/current-system/sw/share/icons/hicolor/256x256/apps/sona.png "
        "-hold 5m 'VM Song One' 'VM Song Two'"
    )
    machine.wait_until_succeeds(f"grep -q 'shown VM Song Two' {out}", timeout=60)
    machine.log("notifyprobe: " + machine.succeed(f"cat {out}"))
    return out


def notification_server():
    """The process owning org.freedesktop.Notifications."""
    pid = busctl_json(
        "call org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus "
        "GetConnectionUnixProcessID s org.freedesktop.Notifications"
    )["data"][0]
    return machine.succeed(f"tr '\\0' ' ' < /proc/{pid}/cmdline").strip()


def quit_sona():
    """Quits through MPRIS (Quit), as a shell's media widget would."""
    offset = log_size()
    user("busctl --user call org.mpris.MediaPlayer2.sona /org/mpris/MediaPlayer2 org.mpris.MediaPlayer2 Quit")
    wait_log(offset, r"shutdown complete", timeout=60)
    machine.wait_until_fails(SONA_PIDS, timeout=30)
