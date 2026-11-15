//go:build !darwin && !windows

package main

import "github.com/coredev-uk/sona-go/internal/config"

// loginStartsHidden: only macOS and Windows need the settings to say so
// (login_desktop.go). The XDG autostart entry passes --hidden itself
// (app.Activation.Hidden).
func loginStartsHidden(config.Desktop) bool { return false }
