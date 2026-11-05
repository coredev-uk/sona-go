# Sona in a KDE Plasma 6 (Wayland) session: tests/vm/README.md lists the checks.
{ pkgs, sona, ... }:

{
  name = "sona-plasma";
  enableOCR = true;

  nodes.machine = {
    imports = [ ./common.nix ];

    services.displayManager.plasma-login-manager.enable = true;
    services.desktopManager.plasma6.enable = true;
    environment.plasma6.excludePackages = [ pkgs.kdePackages.elisa ];

    environment.systemPackages = [
      sona
      pkgs.kdotool # KWin's window list (app id, title, geometry)
      pkgs.xsettingsd # dump_xsettings: what kde-gtk-config serves GTK clients on X11
    ];

    # Never lock or dim the screen during the test.
    environment.etc."xdg/kscreenlockerrc".text = ''
      [Daemon]
      Autolock=false
      LockOnResume=false
    '';
    environment.etc."xdg/powerdevilrc".text = ''
      [AC][Display]
      DimDisplayWhenIdle=false
      TurnOffDisplayWhenIdle=false
    '';
    # A notification popup stays long enough for the test to read it
    # (Plasma's default is 5 s).
    environment.etc."xdg/plasmanotifyrc".text = ''
      [Notifications]
      PopupTimeout=30000
    '';
  };

  testScript = ''
    ${builtins.readFile ./common.py}

    def plasma_popup():
        """Where plasmashell's popup is (x, y, w, h), None when it has none:
        Kickoff, a notification. KWin lists them with the class
        org.kde.plasmashell, the desktop and the panels as plasmashell."""
        for wid in user("KDE_SESSION_VERSION=6 kdotool search --class org.kde.plasmashell || true").split():
            geometry = user(f"KDE_SESSION_VERSION=6 kdotool getwindowgeometry {shlex.quote(wid)}")
            m = re.search(r"Position: (-?\d+),(-?\d+)\s+Geometry: (\d+)x(\d+)", geometry)
            if m:
                return tuple(int(v) for v in m.groups())
        return None

    def kwin_buttons(left, right):
        """Sets KWin's title bar buttons (X close, I minimise, A maximise; None
        for its default). kded's gtkconfig module (kde-gtk-config) and KWin read
        kwinrc again when the writer notifies them (--notify, as System Settings
        does), which reads the whole file: the first key is written without it,
        so that one notification carries both, and a quick second one cannot
        be missed."""
        def write(key, value, notify):
            flags = "--notify " if notify else ""
            user(f"kwriteconfig6 {flags}--file kwinrc --group org.kde.kdecoration2 --key {key} " + ("--delete" if value is None else value))
        write("ButtonsOnRight", right, False)
        write("ButtonsOnLeft", left, True)

    start_all()

    with subtest("Plasma session"):
        machine.wait_until_succeeds("pgrep plasmashell", timeout=180)
        machine.wait_until_succeeds(as_alice("systemctl --user show-environment | grep -q '^WAYLAND_DISPLAY='"), timeout=120)
        machine.wait_until_succeeds(as_alice("busctl --user status org.kde.StatusNotifierWatcher"), timeout=120)
        # The desktop and the panel are windows KWin lists once plasmashell has drawn them.
        wait_until(lambda: len(user("KDE_SESSION_VERSION=6 kdotool search --class plasmashell || true").split()) >= 2, timeout=120, what="Plasma's desktop and panel")
        machine.log(user("systemctl --user show-environment"))

    with subtest("D1: the package is installed"):
        out = user("sona --version").strip()
        assert out == "sona ${sona.version}", out

    seed_queue()

    with subtest("D2: start from the launcher (Kickoff search)"):
        offset = log_size()
        # Kickoff may miss the first shortcut while Plasma is still busy
        # starting; try a few times.
        for attempt in range(4):
            machine.send_key("alt-f1")
            try:
                wait_until(plasma_popup, timeout=15, what="Kickoff")
                machine.sleep(1)  # the search field takes the focus
                machine.send_chars("Sona")
                wait_screen_text("Music Player", region=plasma_popup, timeout=30, zoom=2)
                break
            except Exception:
                if attempt == 3:
                    raise
                machine.send_key("esc")
                machine.sleep(2)
        machine.screenshot("plasma-launcher")
        machine.send_key("ret")
        w, frame = check_running(offset)
        machine.log(json.dumps(w))
        machine.sleep(3)
        machine.screenshot("plasma-wayland")
        # KWin's window for Sona carries the desktop entry's name as its
        # app id, which the task manager groups by.
        ids = user("KDE_SESSION_VERSION=6 kdotool search --class dev.coredev.Sona").split()
        assert len(ids) == 1, ids
        cls = user(f"KDE_SESSION_VERSION=6 kdotool getwindowclassname {ids[0]}").strip()
        assert cls == "dev.coredev.Sona", cls

    with subtest("Title bar: KWin's own on Wayland"):
        # Plasma under Wayland leaves the title bar to KWin (Automatic is
        # the desktop's), which draws it outside the window's content: no
        # window button of Sona's or GTK's is there to read.
        assert w["buttons"] == [], w["buttons"]

    with subtest("Offline and private"):
        check_offline(offset)

    with subtest("D5: MPRIS"):
        check_mpris_bus()

    with subtest("D7: tray"):
        names, items = check_tray()
        assert any(i.split("/")[0] in names for i in items), (items, names)
        machine.screenshot("plasma-tray")

    with subtest("D6: notifications reach Plasma"):
        assert "plasmashell" in notification_server(), notification_server()
        start_notifyprobe()
        wait_screen_text("VM Song Two", region=plasma_popup, timeout=60, zoom=2)
        machine.screenshot("plasma-notification")
        user("systemctl --user stop notifyprobe || true")  # cleanup only: the probe may have reached its -hold already

    with subtest("D8: single instance"):
        check_single_instance()

    with subtest("Quit through MPRIS"):
        quit_sona()

    with subtest("D11: X11 (XWayland) and GTK's buttons in the layout kwinrc says"):
        # Under X11 Sona hides the title bar and MyGo shows GTK's buttons, in
        # GTK's gtk-decoration-layout. On Plasma GTK gets that from kded's
        # gtkconfig module (kde-gtk-config), which derives it from kwinrc and
        # serves it as the XSettings value Gtk/DecorationLayout, which GTK
        # prefers to settings.ini. The module keeps kwinrc in memory and reads
        # it again only when told it changed: a plain write is not seen, nor
        # is loading the module again (kwin_buttons).
        # KWin's ButtonsOnLeft=X, ButtonsOnRight=IA is "close:minimize,maximize".
        kwin_buttons("X", "IA")
        layout = 'Gtk/DecorationLayout "close:minimize,maximize"'
        machine.wait_until_succeeds(as_alice(f"{x11_env()} dump_xsettings | grep -qxF {shlex.quote(layout)}"), timeout=60)
        machine.log(user(f"{x11_env()} dump_xsettings | grep Gtk/"))
        offset = start_sona("sona-x11", env={"GDK_BACKEND": "x11"})
        w, frame = check_running(offset)
        machine.log(json.dumps(w))
        check_x11_window()
        buttons = sorted(w["buttons"], key=lambda b: b["x"])
        assert [b["name"] for b in buttons] == ["Close", "Minimize", "Maximize"], buttons
        # Close leads on the left; Minimize and Maximize trail on the right.
        assert buttons[0]["x"] < frame["w"] / 2 < buttons[1]["x"], (buttons, frame)
        check_mpris_bus()
        check_tray()
        check_single_instance()
        machine.sleep(3)
        machine.screenshot("plasma-x11")
        quit_sona()
        kwin_buttons(None, None)

    def set_scale(scale):
        # kscreen-doctor talks to KWin over Wayland.
        doctor = f"WAYLAND_DISPLAY={session_env('WAYLAND_DISPLAY')} kscreen-doctor"
        outputs = json.loads(user(f"{doctor} -j"))["outputs"]
        name = [o["name"] for o in outputs if o.get("enabled")][0]
        user(f"{doctor} output.{name}.scale.{scale}")
        machine.sleep(3)
        machine.log(user(f"{doctor} -o"))

    with subtest("D10: scale 200 %, then 150 % while running"):
        set_scale(2)
        offset = start_sona("sona-hidpi")
        w, frame = check_running(offset)
        machine.log(json.dumps(w))
        # In logical pixels: at 200 % the screen is 1280×800 and the window's
        # width fits it, nowhere near its 2560 physical pixels. KWin leaves a
        # window taller than the screen as it is, where Mutter (the GNOME test)
        # shrinks it: Sona's default 1280×820 is 20 px taller than the screen
        # (README, "What the VMs can't show"), so the height is only held to
        # be logical.
        assert 0 < frame["w"] <= 1280 and 0 < frame["h"] < SCREEN[1], frame
        machine.sleep(3)
        wait_screen_text("Welcome to Sona")
        machine.screenshot("plasma-scale-2")
        set_scale(1.5)
        machine.sleep(3)
        wait_screen_text("Welcome to Sona")
        machine.screenshot("plasma-scale-150")
        quit_sona()
  '';
}
