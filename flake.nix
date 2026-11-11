# Sona as a flake: `nix run github:coredev-uk/sona-go/nix`, `nix profile add
# github:coredev-uk/sona-go/nix`, or `inputs.sona.url` with `overlays.default`
# (pkgs.sona) or `packages.<system>.default` in a NixOS or Home Manager
# configuration. Development stays in devenv.
#
# Two refs, the same flake:
#   nix    main plus build/linux/nix/pins.json, the published builds the
#          packages below install. nix-branch.yml keeps it there with no pull
#          request: on every published release and nightly rc it checks the
#          new Arch package, merges main and moves the pins. Updating the flake
#          input is all an update takes.
#   main   no pins: `sona` is `sona-source`, main's code built from source.
#
#   sona          the latest release (build/linux/nix/release.nix) on
#                 x86_64-linux, with the Last.fm and Discord credentials only
#                 the release build has; sona-source on aarch64-linux, which
#                 has no release build, and on a ref without pins
#   sona-nightly  the newest published build, the nightly rc of main (or the
#                 release, when that is newer), on the nix ref; sona-source on
#                 aarch64-linux
#   sona-source   built from this checkout (build/linux/nix/package.nix, the
#                 one the desktop VM tests install, tests/vm); no Last.fm
#                 scrobbling or Discord presence
{
  description = "Sona: Apple Music for the Linux desktop";

  # devenv.yaml's nixpkgs revision, so the package gets the Go, GTK3 and
  # GStreamer Sona is developed against. Bumped with it (CONTRIBUTING.md,
  # "Pinned inputs").
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/31ecbaca1bdefa45dffc409f0250bd8039ea0010";

  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;
      # Linux only: macOS builds are .app bundles (build/darwin/bundle.sh).
      # Google ships no Widevine CDM for aarch64-linux, so there Sona plays
      # previews only (`unsupported_platform`).
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      pinsFile = ./build/linux/nix/pins.json;
      pins = if builtins.pathExists pinsFile then lib.importJSON pinsFile else null;
      # The names depend on the pins only, never on pkgs: an overlay whose
      # names depend on `final` recurses forever.
      sonaPackages =
        pkgs:
        let
          sona-source = pkgs.callPackage ./build/linux/nix/package.nix { };
          release =
            pin:
            if pkgs.stdenv.hostPlatform.system == "x86_64-linux" then
              pkgs.callPackage ./build/linux/nix/release.nix { inherit pin; }
            else
              sona-source;
        in
        {
          inherit sona-source;
          sona = if pins != null then release pins.release else sona-source;
        }
        // lib.optionalAttrs (pins != null) { sona-nightly = release pins.nightly; };
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          p = sonaPackages pkgs;
        in
        p // { default = p.sona; }
      );

      overlays.default = final: _: sonaPackages final;

      checks = forAllSystems (
        pkgs: removeAttrs self.packages.${pkgs.stdenv.hostPlatform.system} [ "default" ]
      );

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
