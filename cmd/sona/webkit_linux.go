//go:build linux

package main

/*
#cgo pkg-config: webkitgtk-6.0
#include <webkit/webkit.h>
*/
import "C"

import "fmt"

// webkitVersion is the WebKitGTK version loaded at run time (which can
// differ from the headers built against).
func webkitVersion() string {
	return fmt.Sprintf("%d.%d.%d", uint(C.webkit_get_major_version()), uint(C.webkit_get_minor_version()), uint(C.webkit_get_micro_version()))
}
