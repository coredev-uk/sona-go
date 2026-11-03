#ifndef SONA_MOUSENAV_LINUX_H
#define SONA_MOUSENAV_LINUX_H

#include <gtk/gtk.h>
#include <stdint.h>

void sona_guard_mouse_nav(GtkWidget *window, uintptr_t id);
extern void sonaMouseNav(uintptr_t id, int forward);

#endif
