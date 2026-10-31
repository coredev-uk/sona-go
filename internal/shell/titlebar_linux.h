#ifndef SONA_TITLEBAR_LINUX_H
#define SONA_TITLEBAR_LINUX_H

#include <gtk/gtk.h>

void sona_track_secondary_presses(GtkWidget *window);
gboolean sona_show_window_menu(GtkWidget *window);
char *sona_decoration_layout(void);

#endif
