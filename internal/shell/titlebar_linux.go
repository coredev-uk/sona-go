//go:build linux

package shell

/*
#cgo pkg-config: gtk4
#include <stdlib.h>
#include "titlebar_linux.h"
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// errNoPress: the window menu was asked for without a right click on the
// window to open it at.
var errNoPress = errors.New("no right click to open the window menu at")

// trackSecondaryPresses keeps the window's latest right click, which
// showWindowMenu needs (GTK opens the menu for an input event).
func trackSecondaryPresses(w *application.WebviewWindow) {
	application.InvokeSync(func() {
		if p := w.NativeWindow(); p != nil {
			C.sona_track_secondary_presses((*C.GtkWidget)(p))
		}
	})
}

// showWindowMenu opens the desktop's window menu (GNOME's, KWin's, …) at
// the latest right click on w.
func showWindowMenu(w *application.WebviewWindow) error {
	var shown bool
	application.InvokeSync(func() {
		if p := w.NativeWindow(); p != nil {
			shown = C.sona_show_window_menu((*C.GtkWidget)(p)) != 0
		}
	})
	if !shown {
		return errNoPress
	}
	return nil
}

// DecorationLayout is the desktop's title bar button layout as GTK reads
// it (gtk-decoration-layout: "appmenu:close"); "" when GTK has none. Call
// it once the app runs (GTK is initialised then).
func DecorationLayout() string {
	var layout string
	application.InvokeSync(func() {
		if c := C.sona_decoration_layout(); c != nil {
			layout = C.GoString(c)
			C.free(unsafe.Pointer(c))
		}
	})
	return layout
}
