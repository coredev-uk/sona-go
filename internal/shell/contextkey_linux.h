#ifndef SONA_CONTEXTKEY_LINUX_H
#define SONA_CONTEXTKEY_LINUX_H

#include <gtk/gtk.h>
#include <stdint.h>

void sona_guard_keyboard(GtkWidget *window, uintptr_t id);
extern void sonaContextKey(uintptr_t id, int menuKey);

#endif
