# The GNOME (Wayland) session of gnome.nix and appimage.nix.
{ lib, pkgs, ... }:

let
  appindicator = pkgs.gnomeExtensions.appindicator;
in
{
  imports = [ ./common.nix ];

  services.displayManager.gdm.enable = true;
  services.desktopManager.gnome.enable = true;
  # No welcome tour or core apps: only the shell and Sona.
  services.gnome.core-apps.enable = false;
  environment.gnome.excludePackages = [ pkgs.gnome-tour ];

  # GNOME has no tray; the AppIndicator extension (enabled, as a user
  # wanting Sona's tray would) adds a StatusNotifierWatcher to the shell.
  environment.systemPackages = [
    appindicator
    pkgs.mutter # gdctl
  ];
  programs.dconf.profiles.user.databases = [
    {
      settings = {
        "org/gnome/shell".enabled-extensions = [ appindicator.extensionUuid ];
        # Never blank or lock the screen during the test.
        "org/gnome/desktop/session".idle-delay = lib.gvariant.mkUint32 0;
        "org/gnome/desktop/screensaver".lock-enabled = false;
      };
    }
  ];

  # The test reads the shell's state through org.gnome.Shell.Eval, which
  # only unsafe mode allows (as nixpkgs' own GNOME tests do). Without a
  # unit PATH of its own the shell keeps the session's, which it needs to
  # find the programs its desktop entries start.
  systemd.user.services."org.gnome.Shell@" = {
    enableDefaultPath = false;
    serviceConfig.ExecStart = [
      ""
      "${pkgs.gnome-shell}/bin/gnome-shell --unsafe-mode"
    ];
  };
}
