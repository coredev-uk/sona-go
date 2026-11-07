//go:build linux

package main

import (
	"log/slog"

	"github.com/ebitengine/purego"
)

// setApplicationID gives GLib the program name GTK reports as the Wayland
// app_id (and X11 WM_CLASS): the desktop file is dev.coredev.Sona.desktop,
// which the shell matches the window to by it. MyGo v0.8.3 sets neither.
// It loads GLib by the soname MyGo loads, so one copy serves the process.
func setApplicationID(log *slog.Logger) {
	glib, err := purego.Dlopen("libglib-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		log.Warn("application id", "err", err)
		return
	}
	var setPrgname, setApplicationName func(string)
	purego.RegisterLibFunc(&setPrgname, glib, "g_set_prgname")
	purego.RegisterLibFunc(&setApplicationName, glib, "g_set_application_name")
	setPrgname(AppID)
	setApplicationName("Sona")
}
