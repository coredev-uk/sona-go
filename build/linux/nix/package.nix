# Sona as a Nix package, built from this checkout the way devenv's `build`
# script builds bin/sona: `-tags mygo_noinspector,sqlite_fts5`, VERSION as
# main.version and MyGo's production mode
# (`-X github.com/egoist/mygo.production=1`), plus the desktop entry, icons,
# licence notices, shell completions and manual page nfpm installs
# (build/linux/nfpm/nfpm.yaml). The wrapper's libraries are runtime.nix's.
# No Last.fm or Discord keys are compiled in: the release has them, so the
# flake's `sona` on x86_64-linux is the release (release.nix) and this is its
# `sona-source`.
#
# Built by the flake (`nix build .#sona-source`, flake.nix) and by the
# desktop VM tests (`nix-build tests/vm -A sona`, tests/vm/default.nix).
# After a change to go.sum, update the hash it names (either build prints the
# new one).
{
  lib,
  callPackage,
  buildGoModule,
  go_1_27,
  pkg-config,
  makeWrapper,
  installShellFiles,
  gtk3,
}:

let
  root = ../../..;
  inherit (lib) fileset;
  version = lib.trim (builtins.readFile (root + "/VERSION"));

  # Tracked files only (no node_modules, dist, bin or local data): a flake's
  # source already is (and has no .git), a checkout is filtered here.
  # Without the VM tests' own files, the flake and the release package, so
  # editing them (or pinning the next release) does not rebuild Sona. The
  # notification probe stays: it builds from the same module.
  tracked = if builtins.pathExists (root + "/.git") then fileset.gitTracked root else root;
  vmTests = root + "/tests/vm";
  src = fileset.toSource {
    inherit root;
    fileset = fileset.difference tracked (
      fileset.unions [
        (fileset.difference vmTests (fileset.maybeMissing (vmTests + "/notifyprobe")))
        (fileset.maybeMissing (root + "/flake.nix"))
        (fileset.maybeMissing (root + "/flake.lock"))
        (fileset.maybeMissing ./release.nix)
        (fileset.maybeMissing ./release.json)
      ]
    );
  };

  runtime = callPackage ./runtime.nix { };
in
(buildGoModule.override { go = go_1_27; }) {
  pname = "sona";
  inherit version src;

  vendorHash = "sha256-jBqKZ3s3uWFuJ8NO3I7GBQCXNAPadHNm03+Q65eDs1c=";

  subPackages = [ "cmd/sona" ];
  tags = [
    "mygo_noinspector"
    "sqlite_fts5"
  ];
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X github.com/egoist/mygo.production=1"
  ];
  env.CGO_ENABLED = 1;

  nativeBuildInputs = [
    pkg-config
    makeWrapper
    installShellFiles
  ];

  # The GStreamer set devenv.nix uses, and the libraries MyGo loads by soname.
  buildInputs = [ gtk3 ] ++ runtime.gstreamer;

  # The Go tests run in CI (`go test -race` under dbus-run-session).
  doCheck = false;

  postInstall = ''
    install -Dm644 build/linux/dev.coredev.Sona.desktop -t $out/share/applications
    for icon in build/linux/icons/hicolor/*/apps/*; do
      install -Dm644 "$icon" "$out/share/icons/''${icon#build/linux/icons/}"
    done
    # The `sona` command's completions and manual page, from the built
    # binary before it is wrapped (build/linux/Taskfile.yml, package:cli).
    export HOME=$TMPDIR
    installShellCompletion --cmd sona \
      --bash <($out/bin/sona completion bash) \
      --zsh <($out/bin/sona completion zsh) \
      --fish <($out/bin/sona completion fish)
    $out/bin/sona help --man > sona.1
    installManPage sona.1
    wrapProgram $out/bin/sona ${runtime.wrapperArgs}
    install -Dm644 LICENSE internal/playback/widevine/LICENSE.chromium -t $out/share/doc/sona
  '';

  meta = {
    description = "Apple Music for the Linux desktop";
    homepage = "https://github.com/coredev-uk/sona-go";
    license = lib.licenses.agpl3Only;
    mainProgram = "sona";
    platforms = lib.platforms.linux;
  };
}
