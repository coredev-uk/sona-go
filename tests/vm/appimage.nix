# The release AppImage (built by build/linux/release/release.sh) in the
# GNOME session. NixOS runs AppImages through appimage-run's FHS
# environment; the .deb, .rpm and Arch packages have no NixOS install path
# (release.sh smoke-tests them on their own distributions).
{ pkgs, appimage, ... }:

let
  file = pkgs.runCommand "sona.AppImage" { } "install -m755 ${appimage} $out";
in
{
  name = "sona-appimage";
  enableOCR = true;

  nodes.machine = {
    imports = [ ./gnome-session.nix ];
    programs.appimage = {
      enable = true;
      binfmt = true;
      # Libraries the AppImage expects from the host that appimage-run's
      # environment lacks (the AppImage bundles the rest): libgpg-error
      # (libgcrypt), libcom_err (Kerberos), libgmp (GnuTLS),
      # FriBidi (Pango), and SVT-AV1 (Ubuntu's libavcodec).
      package = pkgs.appimage-run.override {
        extraPkgs = p: [
          p.libgpg-error
          p.e2fsprogs
          p.gmp
          p.fribidi
          p.svt-av1
        ];
      };
    };
    virtualisation.additionalPaths = [ file ];
  };

  testScript = ''
    ${builtins.readFile ./common.py}
    ${builtins.readFile ./gnome.py}

    APPIMAGE = "appimage-run ${file}"
    # The AppImage's binary runs under its own name.
    SONA_PIDS = "pgrep -u alice -x sona"

    start_all()

    with subtest("GNOME session"):
        wait_for_gnome()

    with subtest("D1: the AppImage runs"):
        out = user(f"{APPIMAGE} --version 2>&1")
        assert re.search(r"^sona \d+\.\d+\.\d+", out, re.M), out
        machine.log(out)

    seed_queue()

    with subtest("Start, title bar, offline and private"):
        offset = start_sona("sona-appimage", cmd=APPIMAGE)
        w, frame = check_running(offset, cli=APPIMAGE)
        machine.log(json.dumps(w))
        assert [b["name"] for b in w["buttons"]] == ["Close"], w["buttons"]
        check_offline(offset)
        machine.sleep(3)
        machine.screenshot("appimage-gnome")

    with subtest("D5, D7, D8: MPRIS, tray, single instance"):
        check_mpris_bus()
        check_gnome_tray()
        check_single_instance(cli=APPIMAGE)

    with subtest("Quit through MPRIS"):
        quit_sona()
  '';
}
