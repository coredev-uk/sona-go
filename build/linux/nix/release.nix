# Sona's published release as a Nix package: the Arch package of the GitHub
# release release.json pins, its binary given Nix's loader and wrapped with
# the libraries it links plus runtime.nix's. The release build compiles in
# the Last.fm API key and secret and the Discord application ID (repository
# secrets, release.yml), which a build from source has no way to get, so this
# is the flake's `sona` on x86_64-linux; package.nix builds the checkout
# (`sona-source`, and the only package on aarch64-linux, which has no release
# build).
#
# release.json moves with each published release: nix-release-pin.yml opens
# the pull request.
{
  lib,
  stdenv,
  callPackage,
  fetchurl,
  makeWrapper,
  patchelf,
  zstd,
  glib,
  gst_all_1,
}:

let
  pin = lib.importJSON ./release.json;
  runtime = callPackage ./runtime.nix { };
  # What the binary links (`patchelf --print-needed`), beyond glibc.
  linked = [
    glib
    gst_all_1.gstreamer
    gst_all_1.gst-plugins-base
    stdenv.cc.cc.lib
  ];
in
stdenv.mkDerivation {
  pname = "sona";
  inherit (pin) version;

  src = fetchurl {
    url = "https://github.com/coredev-uk/sona-go/releases/download/v${pin.version}/sona-${pin.version}-1-x86_64.pkg.tar.zst";
    inherit (pin) hash;
  };

  nativeBuildInputs = [
    makeWrapper
    patchelf
    zstd
  ];

  # An Arch package: .PKGINFO and .MTREE beside usr/.
  sourceRoot = ".";
  unpackCmd = "tar --zstd -xf $curSrc usr";

  # The interpreter is the only change to the release's binary (already
  # stripped). Not autoPatchelfHook: patchelf 0.19 setting both the
  # interpreter and a RUNPATH on this Go binary leaves it segfaulting at
  # start, so the linked libraries come from the wrapper's LD_LIBRARY_PATH.
  dontStrip = true;
  dontPatchELF = true;

  installPhase = ''
    runHook preInstall
    install -Dm755 usr/bin/sona -t $out/bin
    patchelf --set-interpreter ${stdenv.cc.bintools.dynamicLinker} $out/bin/sona
    cp -r usr/share $out/share
    wrapProgram $out/bin/sona ${runtime.wrapperArgs} \
      --prefix LD_LIBRARY_PATH : ${lib.makeLibraryPath linked}
    runHook postInstall
  '';

  doInstallCheck = true;
  installCheckPhase = ''
    runHook preInstallCheck
    v=$($out/bin/sona --version)
    if [ "$v" != "sona ${pin.version}" ]; then
      echo "sona --version printed '$v', not 'sona ${pin.version}'"
      exit 1
    fi
    runHook postInstallCheck
  '';

  meta = {
    description = "Apple Music for the Linux desktop";
    homepage = "https://github.com/coredev-uk/sona-go";
    changelog = "https://github.com/coredev-uk/sona-go/releases/tag/v${pin.version}";
    license = lib.licenses.agpl3Only;
    sourceProvenance = [ lib.sourceTypes.binaryNativeCode ];
    mainProgram = "sona";
    platforms = [ "x86_64-linux" ];
  };
}
