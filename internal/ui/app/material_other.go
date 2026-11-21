//go:build !darwin

package app

import "github.com/egoist/mygo"

// miniCornerRadius is 0 off macOS: the mini player keeps the desktop's
// corners.
const miniCornerRadius = 0

// windowMaterial is none off macOS: every surface is solid.
func windowMaterial() mygo.Vibrancy { return mygo.VibrancyNone }
