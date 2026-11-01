# The VM every test shares: user alice logged in automatically, the test
# helpers installed, a 2560×1600 screen, no network.
{
  lib,
  pkgs,
  modulesPath,
  notifyprobe,
  ...
}:

let
  # vmcheck.py with PyGObject and the AT-SPI typelib.
  vmcheck = pkgs.writeShellApplication {
    name = "vmcheck";
    runtimeEnv.GI_TYPELIB_PATH = lib.makeSearchPath "lib/girepository-1.0" [
      pkgs.at-spi2-core
      pkgs.glib.out
      pkgs.gobject-introspection
    ];
    text = ''
      exec ${pkgs.python3.withPackages (p: [ p.pygobject3 ])}/bin/python3 ${./vmcheck.py} "$@"
    '';
  };

  # A queue Sona restores paused at start (coordinator.Saved), so MPRIS
  # has a track to show without playback. No artwork: nothing to fetch.
  queue = pkgs.writeText "queue.json" (
    builtins.toJSON {
      items = [
        {
          id = "1440857781";
          title = "VM Queue Song";
          artist = "Sona VM Artist";
          album = "Offline Album";
          duration = 215 * 1000 * 1000 * 1000;
          artwork = "";
        }
      ];
      index = 0;
      position = 0;
      playedFor = 0;
      shuffle = false;
      repeat = "off";
      context = "";
      volume = 0.8;
    }
  );
in
{
  imports = [ "${modulesPath}/../tests/common/user-account.nix" ];

  virtualisation = {
    memorySize = 4096;
    cores = 4;
    # A HiDPI-sized screen (16:10, 2560×1600): scale 1 and scale 2 both
    # leave room for Sona's window.
    qemu.options = [
      "-vga none"
      "-device virtio-vga,xres=2560,yres=1600"
    ];
    resolution = {
      x = 2560;
      y = 1600;
    };
    # Offline, also when the driver runs outside Nix's sandbox (the
    # interactive driver): no Apple, no CDM download, no sign-in.
    restrictNetwork = true;
  };

  services.displayManager.autoLogin = {
    enable = true;
    user = "alice";
  };

  # Private mode for every Sona started in the session (the VMs never
  # play, but the rule holds); `sona private status` reports it.
  environment.sessionVariables.SONA_PRIVATE = "1";

  environment.systemPackages = [
    notifyprobe
    vmcheck
    pkgs.playerctl
    pkgs.dbus
    pkgs.xwininfo
  ];

  environment.etc."sona-vm/queue.json".source = queue;
}
