#include <gtk/gtk.h>

#include "contextkey_linux.h"

// Runs before the web view sees the key (capture phase on the window).
static gboolean on_context_key(GtkEventControllerKey *controller, guint keyval, guint keycode,
                               GdkModifierType state, gpointer data) {
  (void)controller;
  (void)keycode;
  GdkModifierType mods = state & (GDK_SHIFT_MASK | GDK_CONTROL_MASK | GDK_ALT_MASK | GDK_SUPER_MASK);
  if (keyval == GDK_KEY_Menu && mods == 0) {
    sonaContextKey((uintptr_t)data, 1);
    return TRUE;
  }
  if (keyval == GDK_KEY_F10 && mods == GDK_SHIFT_MASK) {
    sonaContextKey((uintptr_t)data, 0);
    return TRUE;
  }
  return FALSE;
}

// True when w is a web view: the page's WebKitWebView, or the docked
// inspector's view (a plain WebKitWebViewBase, which WebKitGTK parents
// inside the page's view).
static gboolean is_web_view(GtkWidget *w) {
  GType base = g_type_from_name("WebKitWebViewBase");
  return base != 0 && G_TYPE_CHECK_INSTANCE_TYPE(w, base);
}

// The nearest web view above w, or NULL when w is not inside one.
static GtkWidget *web_view_ancestor(GtkWidget *w) {
  for (GtkWidget *p = gtk_widget_get_parent(w); p != NULL; p = gtk_widget_get_parent(p)) {
    if (is_web_view(p)) {
      return p;
    }
  }
  return NULL;
}

static gboolean refocus_web_view(gpointer view) {
  gtk_widget_grab_focus(GTK_WIDGET(view));
  g_object_unref(view);
  return G_SOURCE_REMOVE;
}

// GTK focus moved inside a web view but off it: onto the hidden text
// widget WebKit keeps as its child. Nothing there passes keys to the page,
// so focus goes back to the view (after this notification, not inside it).
// A web view holding the focus itself is right as it is: the docked
// inspector's view sits inside the page's, and sending focus to the page
// would leave the inspector's console unable to take a key.
static void on_focus_widget(GObject *window, GParamSpec *pspec, gpointer data) {
  (void)pspec;
  (void)data;
  GtkWidget *focus = gtk_window_get_focus(GTK_WINDOW(window));
  if (focus == NULL || is_web_view(focus)) {
    return;
  }
  GtkWidget *view = web_view_ancestor(focus);
  if (view != NULL) {
    g_idle_add(refocus_web_view, g_object_ref(view));
  }
}

void sona_guard_keyboard(GtkWidget *window, uintptr_t id) {
  GtkEventController *c = gtk_event_controller_key_new();
  gtk_event_controller_set_propagation_phase(c, GTK_PHASE_CAPTURE);
  g_signal_connect(c, "key-pressed", G_CALLBACK(on_context_key), (gpointer)id);
  gtk_widget_add_controller(window, c);
  g_signal_connect(window, "notify::focus-widget", G_CALLBACK(on_focus_widget), NULL);
}
