//go:build !linux

package shell

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// trackSecondaryPresses is GTK's: elsewhere Sona opens no window menu.
func trackSecondaryPresses(*application.WebviewWindow) {}

// showWindowMenu: Sona opens the window menu only on Linux (GTK).
func showWindowMenu(*application.WebviewWindow) error { return errors.ErrUnsupported }
