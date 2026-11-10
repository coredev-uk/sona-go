# What Sona needs at run time, shared by both Nix packages (package.nix,
# built from the checkout, and release.nix, the published release): MyGo
# loads GTK3, Pango, EGL, epoxy and the tray's appindicator by soname, and
# GStreamer finds its plugins on GST_PLUGIN_SYSTEM_PATH_1_0, so the wrapper
# sets both (`wrapProgram $out/bin/sona ${wrapperArgs}`).
{
  lib,
  gtk3,
  glib,
  pango,
  cairo,
  gdk-pixbuf,
  harfbuzz,
  fontconfig,
  freetype,
  libepoxy,
  at-spi2-core,
  libglvnd,
  libayatana-appindicator,
  libx11,
  libxtst,
  libsecret,
  gst_all_1,
  pipewire,
}:

let
  gst = gst_all_1;
  # The GStreamer set devenv.nix uses; the binary links the first two.
  gstreamer = [
    gst.gstreamer
    gst.gst-plugins-base
    gst.gst-plugins-good
    gst.gst-plugins-bad
    gst.gst-libav
    pipewire
  ];
  uiLibs = [
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
  inherit gstreamer;
  wrapperArgs = lib.escapeShellArgs [
    "--prefix"
    "LD_LIBRARY_PATH"
    ":"
    "${lib.makeLibraryPath uiLibs}:/run/opengl-driver/lib"
    "--prefix"
    "GST_PLUGIN_SYSTEM_PATH_1_0"
    ":"
    (lib.makeSearchPathOutput "lib" "lib/gstreamer-1.0" gstreamer)
  ];
}
