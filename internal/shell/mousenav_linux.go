//go:build linux

package shell

/*
#cgo pkg-config: gtk4
#include "mousenav_linux.h"
*/
import "C"

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var mouseNavWatchers sync.Map // window ID → func(forward bool)

// trackMouseNav calls fn(false) for a press of the mouse's back button and
// fn(true) for its forward button on w. WebKitGTK gives the page neither
// (it reports both as a button-0 press, which cannot tell them apart), so
// they are taken at the window, before the web view, and not passed on.
func trackMouseNav(w *application.WebviewWindow, fn func(forward bool)) {
	application.InvokeSync(func() {
		p := w.NativeWindow()
		if p == nil {
			return
		}
		id := w.ID()
		if _, loaded := mouseNavWatchers.LoadOrStore(id, fn); loaded {
			return
		}
		C.sona_guard_mouse_nav((*C.GtkWidget)(p), C.uintptr_t(id))
	})
}

//export sonaMouseNav
func sonaMouseNav(id C.uintptr_t, forward C.int) {
	v, ok := mouseNavWatchers.Load(uint(id))
	if !ok {
		return
	}
	// Off the GTK callback: the watcher may wait for the main thread.
	go v.(func(bool))(forward != 0)
}
