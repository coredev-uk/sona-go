//go:build !darwin

package main

import "github.com/coredev-uk/sona-go/internal/config"

// loginStartsHidden: only macOS needs the settings to say so. The XDG
// autostart entry passes --hidden itself (app.Activation.Hidden), and
// Windows has no autostart yet.
func loginStartsHidden(config.Desktop) bool { return false }
