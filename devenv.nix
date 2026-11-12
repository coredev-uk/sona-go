{
  pkgs,
  lib,
  ...
}:

let
  inherit (pkgs.stdenv.hostPlatform) isLinux isDarwin darwinMinVersion;
  gst = pkgs.gst_all_1;
  # GStreamer, for the playback host and the motion artwork decoder
  # (phase 1 kit media): appsrc and the converters (base), autoaudiosink and
  # pulsesink, osxaudiosink on macOS (good), parsers (bad), AAC and H.264/HEVC
  # decoding (libav), pipewiresink (Linux).
  gstPlugins = [
    gst.gstreamer
    gst.gst-plugins-base
    gst.gst-plugins-good
    gst.gst-plugins-bad
    gst.gst-libav
  ]
  ++ lib.optional isLinux pkgs.pipewire;
  # The libraries MyGo loads by soname with purego on Linux (GTK3, Pango,
  # EGL, epoxy, AT-SPI, the tray, X11 placement, Secret Service). They come
  # from this nixpkgs pin, as GStreamer does, so the process has one GLib: a
  # second one breaks it.
  uiLibs = with pkgs; [
    gtk3
    glib
    pango
    cairo
    gdk-pixbuf
    harfbuzz
    fontconfig.lib
    freetype
    libepoxy
    at-spi2-core
    libglvnd
    libayatana-appindicator
    libx11
    libxtst
    libsecret
  ];
in
{
  languages.go = {
    enable = true;
    package = pkgs.go_1_27;
    # devenv builds Delve with go_1_27, so from source rather than the cache. Its tests attach
    # with ptrace, which hosts with kernel.yama.ptrace_scope 2 or 3 refuse (Talos, so the
    # Kandev agent pods), and the shell would fail to build.
    delve.package = pkgs.delve.overrideAttrs { doCheck = false; };
  };

  packages =
    with pkgs;
    [
      gopls
      golangci-lint
      gofumpt
      sqlc
      goose
      sqlite
      pkg-config
    ]
    ++ gstPlugins
    ++ map lib.getDev [
      gst.gstreamer
      gst.gst-plugins-base
    ]
    # MyGo's Linux toolkit (GTK3, loaded with purego: uiLibs), MPRIS checks,
    # a private D-Bus for `tests`, and the headless session for `headless`
    # and `e2e` (sway, wtype).
    ++ lib.optionals isLinux (
      [
        gcc
        playerctl
        dbus
        sway
        wtype
      ]
      ++ uiLibs
    );

  dotenv.enable = true;

  env = {
    CGO_ENABLED = "1";
    GOFLAGS =
      "-tags=sqlite_fts5"
      + lib.optionalString isDarwin " -ldflags=-extldflags=-mmacosx-version-min=${darwinMinVersion}";
    # Only this environment's plugins; never the host's (glibc mismatch).
    GST_PLUGIN_SYSTEM_PATH_1_0 = lib.makeSearchPathOutput "lib" "lib/gstreamer-1.0" gstPlugins;
    GST_PLUGIN_PATH_1_0 = "";
  }
  // lib.optionalAttrs isLinux {
    # /run/opengl-driver/lib holds NixOS's GPU drivers (EGL); llvmpipe from
    # libglvnd's Mesa otherwise.
    LD_LIBRARY_PATH = lib.makeLibraryPath uiLibs + ":/run/opengl-driver/lib";
  }
  # One macOS deployment target for everything a build links: nixpkgs'
  # (its libraries and the cgo objects its clang compiles target it) and the
  # link itself, so ld doesn't warn "object file … was built for newer macOS
  # version than being linked" for every cgo object. A `go build -ldflags=…`
  # replaces GOFLAGS' -ldflags, so build/darwin/Taskfile.yml adds the same
  # flag itself.
  // lib.optionalAttrs isDarwin {
    MACOSX_DEPLOYMENT_TARGET = darwinMinVersion;
  };

  # Each script is described in CONTRIBUTING.md ("devenv scripts").
  scripts = {
    # The development loop (cmd/devloop): rebuilds the app with the dev tag
    # on every save, restarts it where it was, reloads tokens and catalogues
    # without a rebuild. `r` rebuilds, `q` quits. A first argument
    # `-background` is devloop's: restarts don't take the focus.
    dev.exec = ''
      set -e
      go build -o bin/devloop ./cmd/devloop
      flags=()
      if [ "''${1:-}" = "-background" ]; then flags=(-background); shift; fi
      exec bin/devloop "''${flags[@]}" -- "$@"
    '';
    # Every kit component in light and dark, under the same loop.
    gallery.exec = ''
      set -e
      go build -o bin/devloop ./cmd/devloop
      flags=()
      if [ "''${1:-}" = "-background" ]; then flags=(-background); shift; fi
      exec bin/devloop -target ./cmd/uigallery "''${flags[@]}" -- "$@"
    '';
    # Production: no inspector (mygo_noinspector) and MyGo's production mode
    # (build/ldflags.sh; without the Last.fm and Discord credentials: no
    # -trimpath here, which would leave them in the build info).
    build.exec = ''go build -tags mygo_noinspector,sqlite_fts5 -ldflags "$(bash build/ldflags.sh --no-credentials --goos "$(go env GOOS)")" -o bin/sona ./cmd/sona "$@"'';
    # Release packages (.deb/.rpm/Arch/AppImage) built and smoke-tested in
    # podman/docker on Ubuntu 24.04 (build/linux/release/release.sh).
    release.exec = ''bash build/linux/release/release.sh "$@"'';
    gen.exec = ''sqlc generate -f internal/db/sqlc.yaml'';
    fmt.exec = ''gofumpt -w $(git ls-files '*.go' | grep -v '^internal/db/dbq/')'';
    check.exec = ''
      set -e
      go vet ./...
      # Development builds (devloop's restore, the instance names, probe
      # builds) type-check too.
      go vet -tags dev,sqlite_fts5 ./...
      go vet -tags probe,sqlite_fts5 ./...
      # Every package must still type-check on the other platforms
      # (CONTRIBUTING.md, "Platforms").
      GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
      go run ./cmd/typecheck -os darwin -arch arm64 ./...
      golangci-lint run --new-from-rev=origin/main ./...
      # Build values kept past their frame (MyGo's lifetime check).
      go tool mygo vet internal/ui
      # Both message catalogues match the source (CONTRIBUTING.md,
      # "Strings and translations").
      go run ./cmd/i18ntool extract -check
      go run ./cmd/pathcase
    '';
    # Not `test`: the shell's `test` builtin would shadow it.
    tests.exec = ''
      set -e
      ${lib.optionalString isLinux "dbus-run-session -- "}go test -race ./... "$@"
      # The packages with dev-only code, in a development build.
      ${lib.optionalString isLinux "dbus-run-session -- "}go test -race -tags dev,sqlite_fts5 ./internal/devinstance ./internal/ui/devstate ./internal/ui/app ./internal/app ./internal/mpris
    '';
    auth-spike.exec = ''go run ./cmd/authspike "$@"'';
    play-spike.exec = ''go run ./cmd/playspike "$@"'';
    # A headless sway session with private XDG dirs, D-Bus, AT-SPI and stub
    # portals around a command (CONTRIBUTING.md, "UI tests and the
    # end-to-end smoke"); `e2e` runs the end-to-end smoke in it.
    headless.exec = ''bash e2e/headless.sh "$@"'';
    e2e.exec = ''headless -- go test -tags e2e -count=1 ./e2e/... "$@"'';
    # A real Orca walked through the app in `headless` (docs/accessibility.md,
    # "Listening with Orca"). Orca comes from the pinned nixpkgs and is built
    # on first use, so it is not part of everyone's dev shell; what it said
    # is kept in bin/orca-out (E2E_OUT).
    orca-walk.exec = ''
      set -e
      rev=$(sed -n 's|.*nixpkgs/\([0-9a-f]\{40\}\)$|\1|p' devenv.yaml)
      orca=$(nix build --no-link --print-out-paths "github:NixOS/nixpkgs/$rev#orca")/bin/orca
      E2E_ORCA=1 E2E_ORCA_BIN=$orca E2E_OUT=''${E2E_OUT:-$PWD/bin/orca-out} headless -- go test -tags e2e -count=1 -v -run '^TestOrca' ./e2e/ "$@"
    '';
    # The Albums scroll probe in the app's own process (CONTRIBUTING.md,
    # "Measuring UI performance"), in `headless` on a profile whose covers
    # are cached or reachable (the e2e fixture's are not, outside e2e).
    probe.exec = ''
      set -e
      go build -tags probe,sqlite_fts5 -o bin/sona-probe ./cmd/sona
      SONA_PRIVATE=1 exec bin/sona-probe -probe "$@"
    '';
  };

  # .githooks refuse commits and pushes on main (AGENTS.md, "Worktrees and Branches"). The
  # setting lives in the shared git config, so it covers every worktree.
  enterShell = ''
    git config core.hooksPath .githooks
  '';
}
