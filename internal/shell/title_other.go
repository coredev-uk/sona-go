//go:build !linux

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// setFramelessTitle titles a frameless window's toplevel on Linux, the
// platform Sona ships for; elsewhere the window keeps what Wails sets.
func setFramelessTitle(*application.WebviewWindow, string) {}
