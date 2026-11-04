//go:build !darwin

package main

import "github.com/coredev-uk/sona-go/internal/app"

// openURLs is macOS's URL handler; elsewhere links arrive as launch
// arguments.
func openURLs(func(app.Activation, bool)) {}
