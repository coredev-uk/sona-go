#include <gtk/gtk.h>

#include "mousenav_linux.h"

// X11's and GDK's numbering of the back and forward buttons (BTN_SIDE and
// BTN_EXTRA in evdev).
#define BUTTON_BACK 8
#define BUTTON_FORWARD 9

// Runs before the web view sees the press (capture phase on the window).
static void on_pressed(GtkGestureClick *gesture, gint n_press, gdouble x, gdouble y, gpointer data) {
  (void)n_press;
  (void)x;
  (void)y;
  guint button = gtk_gesture_single_get_current_button(GTK_GESTURE_SINGLE(gesture));
  if (button != BUTTON_BACK && button != BUTTON_FORWARD) {
    return;
  }
  // Claimed, so the page never sees the press as the button-0 mouse event
  // WebKitGTK makes of it.
  gtk_gesture_set_state(GTK_GESTURE(gesture), GTK_EVENT_SEQUENCE_CLAIMED);
  sonaMouseNav((uintptr_t)data, button == BUTTON_FORWARD);
}

void sona_guard_mouse_nav(GtkWidget *window, uintptr_t id) {
  GtkGesture *g = gtk_gesture_click_new();
  gtk_gesture_single_set_button(GTK_GESTURE_SINGLE(g), 0);
  gtk_event_controller_set_propagation_phase(GTK_EVENT_CONTROLLER(g), GTK_PHASE_CAPTURE);
  g_signal_connect(g, "pressed", G_CALLBACK(on_pressed), (gpointer)id);
  gtk_widget_add_controller(window, GTK_EVENT_CONTROLLER(g));
}
