# Go 1.27.4, on the pinned nixpkgs' Go 1.27 build. go.mod names 1.27.4 (the fix
# for GO-2026-6859, net/http), and nixos-unstable still ships 1.27.3: only
# staging-next has 1.27.4 (NixOS/nixpkgs#586269), and moving the whole pin to
# it would rebuild GTK, GStreamer and the rest. This takes the source tarball
# of 1.27.4 (SHA-256 from https://go.dev/dl/?mode=json) and builds it with
# nixpkgs' recipe. Delete this file, and its three users (devenv.nix,
# package.nix, tests/vm via package.nix), when the pinned nixpkgs' go_1_27 is
# 1.27.4 or newer ("Pinned inputs" in docs/maintainers/repository.md).
{ go_1_27, fetchurl }:
go_1_27.overrideAttrs (_: rec {
  version = "1.27.4";
  src = fetchurl {
    url = "https://go.dev/dl/go${version}.src.tar.gz";
    hash = "sha256-c8+4VxrsDAEXdofbhxY9Rqog6XeWcncokaoXZU2aYJk=";
  };
})
