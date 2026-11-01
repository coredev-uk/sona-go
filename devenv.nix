{
  pkgs,
  lib,
  ...
}:

let
  inherit (pkgs.stdenv.hostPlatform) isLinux isDarwin darwinMinVersion;
  gst = pkgs.gst_all_1;
  # GStreamer, for the playback host and WebKit's media players (motion
  # artwork, music videos): appsrc and the converters (base), autoaudiosink
  # and pulsesink, osxaudiosink on macOS (good), WebKit's WebVTT encoder and
  # parsers (bad), AAC and video decoding (libav), pipewiresink (Linux).
  gstPlugins = [
    gst.gstreamer
    gst.gst-plugins-base
    gst.gst-plugins-good
    gst.gst-plugins-bad
    gst.gst-libav
  ]
  ++ lib.optional isLinux pkgs.pipewire;
in
{
  languages.go = {
    enable = true;
    package = pkgs.go_1_26;
  };

  languages.javascript = {
    enable = true;
    package = pkgs.nodejs_24;
    pnpm.enable = true;
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
    # Wails' Linux toolkit (GTK4 + WebKitGTK 6; macOS uses the system's
    # WebKit), TLS for libsoup, MPRIS checks, and a private D-Bus for `test`.
    ++ lib.optionals isLinux [
      gcc
      gtk4
      webkitgtk_6_0
      libsoup_3
      glib-networking
      playerctl
      dbus
    ];

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
    GIO_EXTRA_MODULES = "${pkgs.glib-networking}/lib/gio/modules";
  }
  # One macOS deployment target for everything a build links: nixpkgs'
  # (its libraries and the cgo objects its clang compiles target it) and the
  # link itself. Wails' cgo directives pass -mmacosx-version-min=10.13 to the
  # linker (11.0 on arm64), so without the -extldflags above, which Go puts
  # last on the link line, ld warns "object file … was built for newer macOS
  # version (14.0) than being linked (11.0)" for every cgo object. A
  # `go build -ldflags=…` replaces GOFLAGS' -ldflags, so
  # build/darwin/Taskfile.yml adds the same flag itself.
  // lib.optionalAttrs isDarwin {
    MACOSX_DEPLOYMENT_TARGET = darwinMinVersion;
  };

  # Each script is described in CONTRIBUTING.md ("devenv scripts").
  scripts =
    let
      # The Go packages embed frontend/dist (cmd/sona, via the frontend package), so vet, lint, test and
      # build need it. Task skips both steps while their sources are unchanged.
      frontend = ''
        go tool wails3 task common:install:frontend:deps
        go tool wails3 task common:build:frontend DEV=true
      '';
    in
    {
      dev.exec = ''go tool wails3 dev -config ./build/config.yml -port "''${WAILS_VITE_PORT:-9245}" "$@"'';
      build.exec = ''go tool wails3 build "$@"'';
      # Release packages (.deb/.rpm/Arch/AppImage), built on Ubuntu 24.04 in
      # podman/docker and smoke-tested; Taskfile's `release` runs the same.
      release.exec = ''bash build/linux/release/release.sh "$@"'';
      gen.exec = ''
        set -e
        sqlc generate -f internal/db/sqlc.yaml
        go tool wails3 generate bindings -clean=true -ts ./cmd/sona
      '';
      fmt.exec = ''
        set -e
        go tool wails3 task common:install:frontend:deps
        gofumpt -w $(git ls-files '*.go' | grep -v '^internal/db/dbq/')
        (cd frontend && pnpm exec prettier --write src)
      '';
      check.exec = ''
        set -e
        ${frontend}
        go vet ./...
        # Every package must still type-check on the other platforms
        # (CONTRIBUTING.md, "Platforms").
        GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
        go run ./cmd/typecheck -os darwin -arch arm64 ./...
        golangci-lint run --new-from-rev=origin/main ./...
        cd frontend
        node scripts/check-path-case.mjs
        pnpm exec tsc --noEmit
        pnpm exec eslint src
        pnpm exec prettier --check src
        pnpm i18n:check
      '';
      # Not `test`: the shell's `test` builtin would shadow it.
      tests.exec = ''
        set -e
        ${frontend}
        ${lib.optionalString isLinux "dbus-run-session -- "}go test -race ./... "$@"
        (cd frontend && pnpm exec vitest run)
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
