//go:build linux

package shell

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>

// GTK's gtk-xft-dpi is -1 when no settings source (the settings portal,
// GSettings, XSettings) gave one; GTK then renders at 96 dpi. WebKitGTK
// instead scales every web view created after the first web process
// started by the monitor's physical DPI, which is infinite when the
// compositor reports no physical size (headless and nested outputs): that
// view's page gets a 0x0 viewport. Setting the DPI GTK already uses keeps
// GTK unchanged and gives WebKit a finite one.
static void sona_pin_font_dpi(void) {
  GtkSettings *settings = gtk_settings_get_default();
  if (settings == NULL) {
    return;
  }
  int dpi = -1;
  g_object_get(settings, "gtk-xft-dpi", &dpi, NULL);
  if (dpi <= 0) {
    g_object_set(settings, "gtk-xft-dpi", 96 * 1024, NULL);
  }
}
*/
import "C"

// pinFontDPI gives GTK an explicit 96 dpi when no desktop setting provides
// one, before a second web view is created (see
// docs/release-checklist.md, "Blank mini player"). Call it on the GTK
// main thread.
func pinFontDPI() { C.sona_pin_font_dpi() }
