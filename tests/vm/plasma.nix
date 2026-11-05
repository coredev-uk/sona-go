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
      pkgs.kdotool # KWin's window list (app id, title)
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
  };

  testScript = ''
    ${builtins.readFile ./common.py}

    start_all()

    with subtest("Plasma session"):
        machine.wait_until_succeeds("pgrep plasmashell", timeout=180)
        machine.wait_until_succeeds(as_alice("systemctl --user show-environment | grep -q '^WAYLAND_DISPLAY='"), timeout=120)
        machine.wait_until_succeeds(as_alice("busctl --user status org.kde.StatusNotifierWatcher"), timeout=120)
        machine.sleep(10)
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
            machine.sleep(5)
            machine.send_chars("Sona")
            try:
                machine.wait_for_text("Music Player", timeout=60)
                break
            except Exception:
                if attempt == 3:
                    raise
                machine.send_key("esc")
                machine.sleep(5)
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
        machine.wait_for_text("VM Song Two", timeout=120)
        machine.screenshot("plasma-notification")
        user("systemctl --user stop notifyprobe || true")  # cleanup only: the probe may have reached its -hold already

    with subtest("D8: single instance"):
        check_single_instance()

    with subtest("Quit through MPRIS"):
        quit_sona()

    with subtest("D11: X11 (XWayland) and GTK's buttons in the layout kwinrc says"):
        # Under X11 Sona hides the title bar and MyGo shows GTK's buttons,
        # in GTK's gtk-decoration-layout. kded's gtkconfig module (kde-gtk-config)
        # derives it from kwinrc and serves it as an XSettings value, which GTK
        # prefers to settings.ini; a session reads kwinrc when it starts and
        # when KWin's settings change, so after writing kwinrc here the module
        # is loaded again. KWin's ButtonsOnLeft=X, ButtonsOnRight=IA is
        # "close:minimize,maximize".
        user(
            "kwriteconfig6 --file kwinrc --group org.kde.kdecoration2 --key ButtonsOnLeft X"
            " && kwriteconfig6 --file kwinrc --group org.kde.kdecoration2 --key ButtonsOnRight IA"
        )
        user("busctl --user call org.kde.kded6 /kded org.kde.kded6 unloadModule s gtkconfig")
        user("busctl --user call org.kde.kded6 /kded org.kde.kded6 loadModule s gtkconfig")
        # The unit has the session's variables Plasma exports to its children,
        # which systemd-run does not carry.
        offset = start_sona("sona-x11", env={"GDK_BACKEND": "x11", "XDG_CURRENT_DESKTOP": "KDE"})
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
        user(
            "kwriteconfig6 --file kwinrc --group org.kde.kdecoration2 --key ButtonsOnLeft --delete"
            " && kwriteconfig6 --file kwinrc --group org.kde.kdecoration2 --key ButtonsOnRight --delete"
        )

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
        assert 0 < frame["w"] <= 1280 and 0 < frame["h"] <= 800, frame
        machine.sleep(3)
        machine.wait_for_text("Recommendations need Apple Music", timeout=120)
        machine.screenshot("plasma-scale-2")
        set_scale(1.5)
        machine.sleep(3)
        machine.wait_for_text("Recommendations need Apple Music", timeout=120)
        machine.screenshot("plasma-scale-150")
        quit_sona()
  '';
}
