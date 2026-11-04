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
    # and a private D-Bus for `tests`.
    ++ lib.optionals isLinux (
      [
        gcc
        playerctl
        dbus
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
    # The app with the dev build tag (inspector, dev-restore). PR 0.3's
    # devloop replaces this with a watcher.
    dev.exec = ''
      set -e
      go build -tags dev,sqlite_fts5 -o bin/sona-dev ./cmd/sona
      exec bin/sona-dev "$@"
    '';
    # Production: no inspector (mygo_noinspector) and MyGo's production mode.
    build.exec = ''go build -tags mygo_noinspector,sqlite_fts5 -ldflags "-X main.version=$(cat VERSION) -X github.com/egoist/mygo.production=1" -o bin/sona ./cmd/sona "$@"'';
    # Release packages (.deb/.rpm/Arch/AppImage) in podman/docker on Ubuntu
    # 24.04. Unavailable until the phase 4 packaging PR: release.sh says so.
    release.exec = ''bash build/linux/release/release.sh "$@"'';
    gen.exec = ''sqlc generate -f internal/db/sqlc.yaml'';
    fmt.exec = ''gofumpt -w $(git ls-files '*.go' | grep -v '^internal/db/dbq/')'';
    check.exec = ''
      set -e
      go vet ./...
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
    '';
    auth-spike.exec = ''go run ./cmd/authspike "$@"'';
    play-spike.exec = ''go run ./cmd/playspike "$@"'';
  };

  # .githooks refuse commits and pushes on main (AGENTS.md, "Worktrees and Branches"). The
  # setting lives in the shared git config, so it covers every worktree.
  enterShell = ''
    git config core.hooksPath .githooks
  '';
}
