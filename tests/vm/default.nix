# Sona's desktop VM tests (tests/vm/README.md): NixOS VMs running GNOME and
# KDE Plasma sessions, with Sona built from this checkout.
#
#   nix-build tests/vm -A gnome      # one test: gnome or plasma
#   nix-build tests/vm -A all        # both
#   nix-build tests/vm -A sona       # the package alone
#   nix-build tests/vm -A appimage --arg appimage ./bin/release/sona-<version>-x86_64.AppImage
#   $(nix-build tests/vm -A gnome.driverInteractive)/bin/nixos-test-driver   # interactive
#
# nixpkgs is the revision devenv pins (devenv.lock), so the VMs run the
# GTK3 and GStreamer Sona is developed against.
{
  pkgs ?
    let
      locked = (builtins.fromJSON (builtins.readFile ../../devenv.lock)).nodes.nixpkgs.locked;
    in
    import (builtins.fetchTarball {
      url = "https://github.com/${locked.owner}/${locked.repo}/archive/${locked.rev}.tar.gz";
      sha256 = locked.narHash;
    }) { },
  # A release AppImage (build/linux/release/release.sh) for the appimage test.
  appimage ? null,
}:

let
  sona = pkgs.callPackage ./sona.nix { };

  # Sona's notification client on its own (notifyprobe/main.go), from the
  # same source and modules as Sona.
  notifyprobe = sona.overrideAttrs (old: {
    pname = "sona-notifyprobe";
    subPackages = [ "tests/vm/notifyprobe" ];
    preBuild = "";
    postBuild = "";
    postInstall = "";
    meta = old.meta // {
      mainProgram = "notifyprobe";
    };
  });

  test =
    file:
    let
      args = { inherit sona notifyprobe appimage; };
    in
    pkgs.testers.runNixOSTest {
      imports = [ file ];
      # For the test file and for the VMs' modules.
      _module.args = args;
      node.specialArgs = args;
    };

  tests = {
    gnome = test ./gnome.nix;
    plasma = test ./plasma.nix;
  };
in
tests
// {
  inherit sona notifyprobe;
  all = pkgs.linkFarm "sona-vm-tests" (
    pkgs.lib.mapAttrsToList (name: path: { inherit name path; }) tests
  );
  appimage =
    if appimage == null then
      throw "the appimage test needs --arg appimage <path to sona-<version>-x86_64.AppImage>"
    else
      test ./appimage.nix;
}
