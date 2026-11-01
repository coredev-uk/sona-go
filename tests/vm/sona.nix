# Sona as a Nix package, built from this checkout the way
# `go tool wails3 build` builds bin/sona (build/linux/Taskfile.yml): the
# frontend's production build embedded into a production Go build
# (`-tags production,sqlite_fts5`, `-trimpath`, VERSION as main.version),
# plus the desktop entry, icons and licence notices nfpm installs
# (build/linux/nfpm/nfpm.yaml). No Last.fm or Discord keys are compiled in.
#
# The TypeScript bindings are the committed ones in frontend/bindings.
# After a change to go.sum or frontend/pnpm-lock.yaml, update the hash it
# names (`nix-build tests/vm -A sona` prints the new one).
{
  lib,
  buildGoModule,
  stdenvNoCC,
  fetchPnpmDeps,
  pnpmConfigHook,
  pnpm,
  nodejs_24,
  pkg-config,
  wrapGAppsHook4,
  gtk4,
  webkitgtk_6_0,
  libsoup_3,
  glib-networking,
  gst_all_1,
  pipewire,
}:

let
  root = ../..;
  inherit (lib) fileset;
  version = lib.trim (builtins.readFile (root + "/VERSION"));

  # Tracked files only (no node_modules, dist, bin or local data), without
  # the VM tests' own files, so editing a test does not rebuild Sona. The
  # notification probe stays: it builds from the same module.
  src = fileset.toSource {
    inherit root;
    fileset = fileset.difference (fileset.gitTracked root) (
      fileset.difference ./. (fileset.maybeMissing ./notifyprobe)
    );
  };

  # The frontend alone, so a Go change does not rebuild it.
  frontendSrc = fileset.toSource {
    inherit root;
    fileset = fileset.intersection (fileset.gitTracked root) (root + "/frontend");
  };

  # Only the lockfile decides the dependency hash.
  pnpmSrc = fileset.toSource {
    root = root + "/frontend";
    fileset = fileset.unions [
      (root + "/frontend/package.json")
      (root + "/frontend/pnpm-lock.yaml")
      (root + "/frontend/.npmrc")
    ];
  };

  frontend = stdenvNoCC.mkDerivation (finalAttrs: {
    pname = "sona-frontend";
    inherit version;
    src = frontendSrc;

    pnpmRoot = "frontend";
    pnpmDeps = fetchPnpmDeps {
      inherit (finalAttrs) pname version;
      inherit pnpm;
      src = pnpmSrc;
      fetcherVersion = 5;
      hash = "sha256-5lkDzkl7wQ20YURKgSvRdHWjmQ7UFvVKlK1lrcfEsFs=";
    };

    nativeBuildInputs = [
      nodejs_24
      pnpm
      pnpmConfigHook
    ];

    buildPhase = ''
      runHook preBuild
      pnpm -C frontend run build
      runHook postBuild
    '';

    installPhase = ''
      runHook preInstall
      cp -r frontend/dist $out
      runHook postInstall
    '';
  });

  gst = gst_all_1;
in
buildGoModule {
  pname = "sona";
  inherit version src;

  vendorHash = "sha256-BxyGnJ97X8CMFU4FokTY++D+5IHIGIAPuCYiEG17iWU=";

  subPackages = [ "." ];
  # The binary's name everywhere else (bin/sona, /usr/bin/sona), not the module's.
  postBuild = ''
    mv $GOPATH/bin/sona-go $GOPATH/bin/sona
  '';
  tags = [
    "production"
    "sqlite_fts5"
  ];
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
  env.CGO_ENABLED = 1;

  nativeBuildInputs = [
    pkg-config
    wrapGAppsHook4
  ];

  # GTK4 + WebKitGTK 6 for the window, libsoup and glib-networking for
  # WebKit's TLS, and the GStreamer set devenv.nix uses (the wrapper puts
  # the plugins on GST_PLUGIN_SYSTEM_PATH_1_0).
  buildInputs = [
    gtk4
    webkitgtk_6_0
    libsoup_3
    glib-networking
    gst.gstreamer
    gst.gst-plugins-base
    gst.gst-plugins-good
    gst.gst-plugins-bad
    gst.gst-libav
    pipewire
  ];

  # main.go embeds frontend/dist.
  preBuild = ''
    cp -r ${frontend} frontend/dist
    chmod -R u+w frontend/dist
  '';
  overrideModAttrs = _: _: { preBuild = ""; };

  # The Go tests run in CI (`go test -race` under dbus-run-session).
  doCheck = false;

  postInstall = ''
    install -Dm644 build/linux/dev.coredev.Sona.desktop -t $out/share/applications
    for icon in build/linux/icons/hicolor/*/apps/*; do
      install -Dm644 "$icon" "$out/share/icons/''${icon#build/linux/icons/}"
    done
    install -Dm644 LICENSE internal/playback/widevine/LICENSE.chromium -t $out/share/doc/sona
  '';

  passthru = { inherit frontend; };

  meta = {
    description = "Apple Music for the Linux desktop";
    homepage = "https://github.com/coredev-uk/sona-go";
    license = lib.licenses.agpl3Only;
    mainProgram = "sona";
    platforms = lib.platforms.linux;
  };
}
