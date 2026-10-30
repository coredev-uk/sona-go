//go:build linux

package shell

/*
#cgo pkg-config: gtk4
#include <stdlib.h>
#include <gtk/gtk.h>
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// setFramelessTitle titles a frameless window's toplevel. Wails sets no
// title on frameless Linux windows (WebviewWindowOptions.Title and
// SetTitle are skipped), so compositors, task switchers and window rules
// would see the application's name instead.
func setFramelessTitle(w *application.WebviewWindow, title string) {
	application.InvokeSync(func() {
		p := w.NativeWindow()
		if p == nil {
			return
		}
		t := C.CString(title)
		defer C.free(unsafe.Pointer(t))
		C.gtk_window_set_title((*C.GtkWindow)(p), t)
	})
}
