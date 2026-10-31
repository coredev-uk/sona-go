{
  pkgs,
  lib,
  ...
}:

let
  gst = pkgs.gst_all_1;
  # GStreamer runtime for the playback host: appsrc (base), AAC decoding
  # (libav), PipeWire output (pipewire), pulsesink/autoaudiosink (good).
  gstPlugins = [
    gst.gstreamer
    gst.gst-plugins-base
    gst.gst-plugins-good
    gst.gst-libav
    pkgs.pipewire
  ];
  chrome = pkgs.google-chrome;
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
      gcc
      gtk4
      webkitgtk_6_0
      libsoup_3
      glib-networking
      playerctl
      chrome # provides libwidevinecdm.so (never redistributed)
    ]
    ++ gstPlugins
    ++ map lib.getDev [
      gst.gstreamer
      gst.gst-plugins-base
    ];

  dotenv.enable = true;

  env = {
    CGO_ENABLED = "1";
    GOFLAGS = "-tags=sqlite_fts5";
    # Only this environment's plugins; never the host's (glibc mismatch).
    GST_PLUGIN_SYSTEM_PATH_1_0 = lib.makeSearchPathOutput "lib" "lib/gstreamer-1.0" gstPlugins;
    GST_PLUGIN_PATH_1_0 = "";
    GIO_EXTRA_MODULES = "${pkgs.glib-networking}/lib/gio/modules";
    # Development CDM location; the app also probes the usual install paths.
    SONA_WIDEVINE_CDM = "${chrome}/share/google/chrome/WidevineCdm/_platform_specific/linux_x64/libwidevinecdm.so";
  };

  scripts = {
    dev.exec = "go tool wails3 dev \"$@\"";
    build.exec = "go tool wails3 build \"$@\"";
    # Release packages (.deb/.rpm/Arch/AppImage) built and smoke-tested on
    # Ubuntu 24.04 in podman/docker; devenv builds only run on NixOS.
    package.exec = "bash build/linux/release/release.sh \"$@\"";
    gen.exec = ''
      set -e
      sqlc generate -f internal/db/sqlc.yaml
      go tool wails3 generate bindings -clean=true -ts
    '';
    fmt.exec = ''
      set -e
      gofumpt -w $(git ls-files '*.go' | grep -v '^internal/db/dbq/')
      (cd frontend && pnpm exec prettier --write src)
    '';
    check.exec = ''
      set -e
      go vet ./...
      golangci-lint run ./...
      (cd frontend && node scripts/check-path-case.mjs && pnpm exec tsc --noEmit && pnpm exec eslint src)
    '';
    test.exec = ''
      set -e
      go test ./... "$@"
      (cd frontend && pnpm exec vitest run)
    '';
    auth-spike.exec = "go run ./cmd/authspike \"$@\"";
    engine-spike.exec = "go run ./cmd/enginespike \"$@\"";
    play-spike.exec = "go run ./cmd/playspike \"$@\"";
  };
}
