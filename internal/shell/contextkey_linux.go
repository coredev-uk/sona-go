//go:build linux

package shell

/*
#cgo pkg-config: gtk4
#include "contextkey_linux.h"
*/
import "C"

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Each key re-sent to the page as the keydown the web view would have
// delivered, on the focused element, so the page's own context-menu code
// (rows, tiles) handles it as before.
const (
	shiftF10JS = `(document.activeElement||document.body).dispatchEvent(new KeyboardEvent("keydown",{key:"F10",code:"F10",shiftKey:true,bubbles:true,cancelable:true}))`
	menuKeyJS  = `(document.activeElement||document.body).dispatchEvent(new KeyboardEvent("keydown",{key:"ContextMenu",code:"ContextMenu",bubbles:true,cancelable:true}))`
)

var contextKeyWindows sync.Map // window ID → *application.WebviewWindow

// guardKeyboard works around two WebKitGTK keyboard traps, both on the
// hidden text widget the GTK 4 web view keeps beside the page to translate
// key bindings; once it has GTK focus the page gets no more keys until the
// window is clicked (docs/accessibility.md):
//   - Shift+F10 and the Menu key open that widget's own text context menu,
//     so they are stopped before WebKit sees them and handed to the page,
//     where they open Sona's row and tile menus;
//   - Tab off either end of the page moves GTK focus onto that widget, so
//     focus is sent straight back to the web view.
func guardKeyboard(w *application.WebviewWindow) {
	application.InvokeSync(func() {
		p := w.NativeWindow()
		if p == nil {
			return
		}
		id := w.ID()
		if _, loaded := contextKeyWindows.LoadOrStore(id, w); loaded {
			return
		}
		C.sona_guard_keyboard((*C.GtkWidget)(p), C.uintptr_t(id))
	})
}

//export sonaContextKey
func sonaContextKey(id C.uintptr_t, menuKey C.int) {
	v, ok := contextKeyWindows.Load(uint(id))
	if !ok {
		return
	}
	js := shiftF10JS
	if menuKey != 0 {
		js = menuKeyJS
	}
	// Off the GTK callback: ExecJS may wait for the main thread.
	go v.(*application.WebviewWindow).ExecJS(js)
}
