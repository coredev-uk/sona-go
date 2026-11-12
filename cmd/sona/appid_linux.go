//go:build linux

package main

import (
	"log/slog"
	"strings"

	"github.com/ebitengine/purego"
)

// setApplicationID gives GLib and GDK the names GTK reports as the Wayland
// app_id, the X11 WM_CLASS (instance and class) and the application's name
// on the accessibility bus: the desktop file is dev.coredev.Sona.desktop,
// which the shell matches the window to by them, and Orca keys its
// per-application scripts on the accessibility name. MyGo names the program
// after the executable (sona; sona-e2e in the end-to-end build) in App.Run,
// after anything main does first, so this runs after that, from
// App.WhenReady and before the first window: a GtkWindow takes its class
// when it is created.
// It loads GLib and GDK by the sonames MyGo loads, so one copy serves the
// process.
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

	// GDK derives the class from the program name once, as GTK starts:
	// the name with its first letter in upper case.
	gdk, err := purego.Dlopen("libgdk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		log.Warn("application id", "err", err)
		return
	}
	var setProgramClass func(string)
	purego.RegisterLibFunc(&setProgramClass, gdk, "gdk_set_program_class")
	setProgramClass(strings.ToUpper(AppID[:1]) + AppID[1:])
}
