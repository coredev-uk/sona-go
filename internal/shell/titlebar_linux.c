#include <gtk/gtk.h>

#include "titlebar_linux.h"

#define SONA_LAST_PRESS "sona-last-secondary-press"

// Keeps the window's latest secondary-button press (before the web view
// sees it, which still gets it): opening the window menu needs that event,
// and the page asks for the menu only after it has handled the press.
static gboolean on_event(GtkEventControllerLegacy *controller, GdkEvent *event, gpointer window) {
  (void)controller;
  if (gdk_event_get_event_type(event) == GDK_BUTTON_PRESS &&
      gdk_button_event_get_button(event) == GDK_BUTTON_SECONDARY) {
    g_object_set_data_full(G_OBJECT(window), SONA_LAST_PRESS, gdk_event_ref(event),
                           (GDestroyNotify)gdk_event_unref);
  }
  return FALSE;
}

void sona_track_secondary_presses(GtkWidget *window) {
  GtkEventController *c = gtk_event_controller_legacy_new();
  gtk_event_controller_set_propagation_phase(c, GTK_PHASE_CAPTURE);
  g_signal_connect(c, "event", G_CALLBACK(on_event), window);
  gtk_widget_add_controller(window, c);
}

// Opens the desktop's window menu for the latest secondary press, once.
gboolean sona_show_window_menu(GtkWidget *window) {
  GdkEvent *press = g_object_get_data(G_OBJECT(window), SONA_LAST_PRESS);
  if (press == NULL) {
    return FALSE;
  }
  GdkSurface *surface = gtk_native_get_surface(GTK_NATIVE(window));
  gboolean shown = FALSE;
  if (surface != NULL && GDK_IS_TOPLEVEL(surface)) {
    shown = gdk_toplevel_show_window_menu(GDK_TOPLEVEL(surface), press);
  }
  g_object_set_data(G_OBJECT(window), SONA_LAST_PRESS, NULL);
  return shown;
}

// GTK's gtk-decoration-layout: the desktop's title bar button layout, as
// GTK reads it (GNOME's button-layout through the settings portal or
// GSettings, XSettings, settings.ini). The caller frees it.
char *sona_decoration_layout(void) {
  GtkSettings *settings = gtk_settings_get_default();
  if (settings == NULL) {
    return NULL;
  }
  char *layout = NULL;
  g_object_get(settings, "gtk-decoration-layout", &layout, NULL);
  return layout;
}
