# Sona in a GNOME (Wayland) session: tests/vm/README.md lists the checks.
{ sona, ... }:

{
  name = "sona-gnome";
  enableOCR = true;

  nodes.machine = {
    imports = [ ./gnome-session.nix ];
    environment.systemPackages = [ sona ];
  };

  testScript = ''
    ${builtins.readFile ./common.py}
    ${builtins.readFile ./gnome.py}

    start_all()

    with subtest("GNOME session"):
        wait_for_gnome()

    with subtest("D1: the package is installed"):
        out = user("sona --version").strip()
        assert out == "sona ${sona.version}", out

    with subtest("D2: launcher entry"):
        app = shell_eval(
            "(() => { const a = imports.gi.Shell.AppSystem.get_default().lookup_app('dev.coredev.Sona.desktop');"
            " return a && [a.get_name(), a.get_icon().to_string()]; })()"
        )
        assert app == ["Sona", "sona"], app

    seed_queue()

    with subtest("D2: start from the shell's app system"):
        offset = log_size()
        shell_eval("imports.gi.Shell.AppSystem.get_default().lookup_app('dev.coredev.Sona.desktop').activate()")
        w, frame = check_running(offset)
        machine.log(json.dumps(w))
        machine.sleep(3)
        machine.screenshot("gnome-wayland")
        windows = shell_eval(
            "global.get_window_actors().map(a => a.meta_window).map(w => [w.get_wm_class(), w.get_client_type(),"
            " imports.gi.Shell.WindowTracker.get_default().get_window_app(w)?.get_id() ?? null, w.get_title()])"
        )
        machine.log(f"windows: {windows}")
        sona_windows = [x for x in windows if x[3] == "Sona"]
        assert len(sona_windows) == 1, windows
        assert sona_windows[0][2] == "dev.coredev.Sona.desktop", windows

    with subtest("Title bar: GTK's own buttons in GNOME's layout (appmenu:close)"):
        buttons = [b["name"] for b in w["buttons"]]
        assert buttons == ["Close"], w["buttons"]
        assert w["buttons"][0]["x"] > frame["w"] / 2, (w["buttons"], frame)

    with subtest("Offline and private"):
        check_offline(offset)

    with subtest("D5: MPRIS"):
        check_mpris_bus()
        shell_eval("Main.panel.statusArea.dateMenu.menu.open()")
        machine.wait_for_text(QUEUE_TITLE, timeout=120)
        machine.screenshot("gnome-media-card")
        shell_eval("Main.panel.statusArea.dateMenu.menu.close()")

    with subtest("D7: tray (AppIndicator extension)"):
        check_gnome_tray()
        machine.screenshot("gnome-tray")

    with subtest("D6: notifications reach GNOME Shell"):
        # GNOME Shell's own service (a gjs process) owns the name and hands
        # the notifications to the shell.
        assert "org.gnome.Shell.Notifications" in notification_server(), notification_server()
        out = start_notifyprobe()
        shown = shell_eval(
            "Main.messageTray.getSources().filter(s => s.title === 'Sona').map(s => s.notifications.map(n => n.title))"
        )
        assert shown == [["VM Song Two"]], shown
        # Sona's notifications are low urgency, which GNOME shows in the
        # notification list without a banner: look there.
        shell_eval("Main.panel.statusArea.dateMenu.menu.open()")
        machine.wait_for_text("VM Song Two", timeout=120)
        machine.screenshot("gnome-notification")
        shell_eval("Main.panel.statusArea.dateMenu.menu.close()")
        # A click on the notification: the shell reports the default action.
        shell_eval(
            "Main.messageTray.getSources().find(s => s.title === 'Sona').notifications[0].activate()"
        )
        machine.wait_until_succeeds(f"grep -q 'action default' {out}", timeout=30)
        user("systemctl --user stop notifyprobe || true")  # cleanup only: the probe may have reached its -hold already

    with subtest("D8: single instance"):
        check_single_instance()

    with subtest("Quit through MPRIS"):
        quit_sona()

    with subtest("D11: X11 (XWayland) and another button layout"):
        user("gsettings set org.gnome.desktop.wm.preferences button-layout 'close,minimize,maximize:'")
        offset = start_sona("sona-x11", env={"GDK_BACKEND": "x11"})
        w, frame = check_running(offset)
        machine.log(json.dumps(w))
        kinds = shell_eval(
            "global.get_window_actors().map(a => a.meta_window).filter(w => w.get_title() === 'Sona')"
            ".map(w => w.get_client_type())"
        )
        assert kinds == [1], f"not one X11 client (Meta.WindowClientType.X11 = 1): {kinds}"
        check_x11_window()
        buttons = sorted(w["buttons"], key=lambda b: b["x"])
        assert [b["name"] for b in buttons] == ["Close", "Minimize", "Maximize"], w["buttons"]
        assert all(b["x"] < frame["w"] / 2 for b in w["buttons"]), (w["buttons"], frame)
        check_mpris_bus()
        check_tray()
        check_single_instance()
        machine.sleep(3)
        machine.screenshot("gnome-x11")
        quit_sona()
        user("gsettings reset org.gnome.desktop.wm.preferences button-layout")

    with subtest("D10: scale 200 %"):
        monitor = re.search(r"Monitor (\S+)", user("gdctl show"))
        assert monitor is not None
        user(f"gdctl set --logical-monitor --primary --monitor {monitor.group(1)} --scale 2")
        machine.log(user("gdctl show"))
        offset = start_sona("sona-hidpi")
        w, frame = check_running(offset)
        machine.log(json.dumps(w))
        # Logical pixels: the window fits the 1280×800 logical screen.
        assert 0 < frame["w"] <= 1280 and 0 < frame["h"] <= 800, frame
        machine.sleep(3)
        machine.wait_for_text("Recommendations need Apple Music", timeout=120)
        machine.screenshot("gnome-scale-2")
        quit_sona()
  '';
}
