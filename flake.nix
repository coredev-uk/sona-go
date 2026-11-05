# Sona as a flake: `nix run github:coredev-uk/sona-go`, `nix profile add
# github:coredev-uk/sona-go`, or `inputs.sona.url` with `overlays.default`
# (pkgs.sona) or `packages.<system>.default` in a NixOS or Home Manager
# configuration. The package is build/linux/nix/package.nix, the one the
# desktop VM tests install (tests/vm). Development stays in devenv.
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
    in
    {
      packages = forAllSystems (pkgs: rec {
        sona = pkgs.callPackage ./build/linux/nix/package.nix { };
        default = sona;
      });

      overlays.default = final: _: { sona = final.callPackage ./build/linux/nix/package.nix { }; };

      checks = forAllSystems (pkgs: {
        inherit (self.packages.${pkgs.stdenv.hostPlatform.system}) sona;
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
