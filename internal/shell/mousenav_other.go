//go:build !linux

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// trackMouseNav is GTK's: it works around WebKitGTK, which gives the page
// no usable back and forward buttons; no other platform's web view is known
// to need it.
func trackMouseNav(*application.WebviewWindow, func(forward bool)) {}
