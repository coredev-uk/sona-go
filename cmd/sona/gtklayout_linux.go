//go:build linux

package main

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// gtkDecorationLayout reads GTK's gtk-decoration-layout setting, which MyGo's
// window buttons follow ("appmenu:close", "close,minimize,maximize:"). It
// calls GTK, so it runs on the main thread once the toolkit started; it loads
// the libraries by the sonames MyGo loads, so one copy serves the process.
func gtkDecorationLayout() (string, error) {
	gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return "", fmt.Errorf("load GTK: %w", err)
	}
	gobject, err := purego.Dlopen("libgobject-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return "", fmt.Errorf("load GObject: %w", err)
	}
	glib, err := purego.Dlopen("libglib-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return "", fmt.Errorf("load GLib: %w", err)
	}
	var (
		settingsDefault func() uintptr
		objectGet       func(obj uintptr, name *byte, out unsafe.Pointer, end uintptr)
		free            func(p unsafe.Pointer)
	)
	purego.RegisterLibFunc(&settingsDefault, gtk, "gtk_settings_get_default")
	purego.RegisterLibFunc(&objectGet, gobject, "g_object_get")
	purego.RegisterLibFunc(&free, glib, "g_free")

	settings := settingsDefault()
	if settings == 0 {
		return "", fmt.Errorf("GTK has no default settings")
	}
	var p unsafe.Pointer
	objectGet(settings, unsafe.StringData("gtk-decoration-layout\x00"), unsafe.Pointer(&p), 0)
	if p == nil {
		return "", nil
	}
	defer free(p)
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n)), nil
}
