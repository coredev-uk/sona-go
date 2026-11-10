# Sona as a flake: `nix run github:coredev-uk/sona-go`, `nix profile add
# github:coredev-uk/sona-go`, or `inputs.sona.url` with `overlays.default`
# (pkgs.sona) or `packages.<system>.default` in a NixOS or Home Manager
# configuration. Development stays in devenv.
#
#   sona          the published release (build/linux/nix/release.nix) on
#                 x86_64-linux, with the Last.fm and Discord credentials only
#                 the release build has; on aarch64-linux, which has no release
#                 build, sona-source
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
      # Linux only: macOS builds are .app bundles (build/darwin/bundle.sh).
      # Google ships no Widevine CDM for aarch64-linux, so there Sona plays
      # previews only (`unsupported_platform`).
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      sonaPackages = pkgs: rec {
        sona-source = pkgs.callPackage ./build/linux/nix/package.nix { };
        sona =
          if pkgs.stdenv.hostPlatform.system == "x86_64-linux" then
            pkgs.callPackage ./build/linux/nix/release.nix { }
          else
            sona-source;
      };
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

      checks = forAllSystems (pkgs: {
        inherit (self.packages.${pkgs.stdenv.hostPlatform.system}) sona sona-source;
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
