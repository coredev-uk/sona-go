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
    package = pkgs.callPackage ./build/linux/nix/go.nix { };
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
      # Only the documentation site (site/, the `docs` script); the app's
      # build and `check` never touch it.
      nodejs
      # Only build/darwin/dmg/render.sh, which draws the .dmg window's
      # background from its SVG.
      resvg
      # Runs the developer commands (justfile); devenv's scripts call it.
      just
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

  # Each script is the justfile recipe of its name, so `dev` here and
  # `just dev` (Windows, or anywhere just is installed) run the same commands
  # (docs/development.md, "Developer commands").
  scripts =
    lib.genAttrs
      [
        "auth-spike"
        "build"
        "check"
        "dev"
        "docs"
        "e2e"
        "fmt"
        "gallery"
        "gen"
        "headless"
        "orca-walk"
        "play-spike"
        "probe"
        "release"
        # Not `test`: the shell's `test` builtin would shadow it.
        "tests"
      ]
      (name: {
        exec = ''exec just --justfile "$DEVENV_ROOT/justfile" ${name} "$@"'';
      });

  # .githooks refuse commits and pushes on main (AGENTS.md, "Workflow"). The
  # setting lives in the shared git config, so it covers every worktree.
  enterShell = ''
    git config core.hooksPath .githooks
  '';
}
