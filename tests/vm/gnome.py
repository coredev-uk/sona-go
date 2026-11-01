# GNOME helpers (after common.py) for gnome.nix and appimage.nix.
APPINDICATOR = "appindicatorsupport@rgcjonas.gmail.com"


def shell_eval(js):
    """Evaluates js in GNOME Shell and returns the JSON-decoded result."""
    ok, result = busctl_json(f"call org.gnome.Shell /org/gnome/Shell org.gnome.Shell Eval s {shlex.quote(js)}")["data"]
    assert ok, f"Shell.Eval failed: {result}"
    return json.loads(result) if result else None


def wait_for_gnome():
    """Waits for alice's GNOME session and leaves the overview."""
    machine.wait_for_unit("display-manager.service")
    machine.wait_for_file("/run/user/1000/wayland-0")
    machine.wait_for_unit("default.target", "alice")
    machine.wait_until_succeeds(
        as_alice(
            "busctl --user call org.gnome.Shell /org/gnome/Shell org.gnome.Shell Eval s Main.layoutManager._startingUp"
            " | grep -q 'true \"false\"'"
        ),
        timeout=180,
    )
    machine.send_key("esc")
    machine.log(user("systemctl --user show-environment"))


def check_gnome_tray():
    """Sona's tray item, and the AppIndicator extension showing it in the panel."""
    state = user(f"gnome-extensions info {APPINDICATOR}")
    assert "ACTIVE" in state, state
    names, _ = check_tray()
    # The extension puts each item it shows in the panel's status area.
    keys = shell_eval("Object.keys(Main.panel.statusArea)")
    machine.log(f"panel status area: {keys}")
    assert any(n in k for k in keys for n in names), (keys, names)
