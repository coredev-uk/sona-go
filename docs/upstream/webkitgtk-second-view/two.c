// Two WebKitWebViews in one process: the second one's page gets a 0x0
// viewport when GTK has no gtk-xft-dpi and the monitor reports no physical
// size. Each page writes innerWidth x innerHeight and devicePixelRatio into
// document.title every 500 ms; notify::title prints it next to the GTK
// allocation of the web view.
//
// Build: cc two.c -o two $(pkg-config --cflags --libs webkitgtk-6.0 gtk4)
// Run:   ./two            second window created 3 s after activate
//        ./two same       both views in one window (created at activate)
//        ./two mapped     second window created when the first maps
// Trigger: GDK_DEBUG=no-portals (or any session without a settings portal,
// GSettings or XSettings DPI) on a compositor whose wl_output reports 0 mm.
#include <gtk/gtk.h>
#include <webkit/webkit.h>

static const char *html =
    "<html><body><div id=o style='width:320px;padding:16px;background:#c33;color:#fff;"
    "font:24px sans-serif'></div><script>"
    "setInterval(() => { document.title = o.textContent = innerWidth + 'x' + innerHeight"
    " + ' dpr ' + devicePixelRatio; }, 500);"
    "</script></body></html>";
static const char *mode = "delay";

static void on_title(WebKitWebView *view, GParamSpec *pspec, gpointer name) {
  g_print("%s: page %s, widget %dx%d, mapped %d\n", (const char *)name,
          webkit_web_view_get_title(view), gtk_widget_get_width(GTK_WIDGET(view)),
          gtk_widget_get_height(GTK_WIDGET(view)), gtk_widget_get_mapped(GTK_WIDGET(view)));
}

static GtkWidget *new_view(const char *name) {
  GtkWidget *view = webkit_web_view_new();
  gtk_widget_set_hexpand(view, TRUE);
  gtk_widget_set_vexpand(view, TRUE);
  g_signal_connect(view, "notify::title", G_CALLBACK(on_title), (gpointer)name);
  webkit_web_view_load_html(WEBKIT_WEB_VIEW(view), html, NULL);
  return view;
}

static GtkWidget *new_window(GtkApplication *app, const char *title) {
  GtkWidget *win = gtk_application_window_new(app);
  gtk_window_set_title(GTK_WINDOW(win), title);
  gtk_window_set_default_size(GTK_WINDOW(win), 640, 480);
  GtkWidget *box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
  gtk_window_set_child(GTK_WINDOW(win), box);
  return win;
}

static void add_view(GtkWidget *win, const char *name) {
  gtk_box_append(GTK_BOX(gtk_window_get_child(GTK_WINDOW(win))), new_view(name));
}

static gboolean second_window(gpointer app) {
  GtkWidget *win = new_window(app, "second");
  add_view(win, "second");
  gtk_window_present(GTK_WINDOW(win));
  return G_SOURCE_REMOVE;
}

static void on_map(GtkWidget *win, gpointer app) { second_window(app); }

static void activate(GtkApplication *app) {
  GtkWidget *win = new_window(app, "first");
  add_view(win, "first");
  if (g_str_equal(mode, "same"))
    add_view(win, "second");
  else if (g_str_equal(mode, "mapped"))
    g_signal_connect_after(win, "map", G_CALLBACK(on_map), app);
  else
    g_timeout_add_seconds(3, second_window, app);
  gtk_window_present(GTK_WINDOW(win));
}

int main(int argc, char **argv) {
  if (argc > 1) mode = argv[1];
  GtkApplication *app = gtk_application_new("org.example.TwoViews", G_APPLICATION_NON_UNIQUE);
  g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
  int status = g_application_run(G_APPLICATION(app), 1, argv);
  g_object_unref(app);
  return status;
}
