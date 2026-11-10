//go:build !darwin

package kit

import "github.com/egoist/mygo/ui"

// The macOS glass spike (glass_darwin.go) has no counterpart elsewhere:
// Linux and Windows keep their solid surfaces.

func GlassOn(string) bool                                    { return false }
func GlassNative() bool                                      { return false }
func GlassTopStrip() bool                                    { return false }
func GlassPane(e ui.Element, _ string, _ float32) ui.Element { return e }
func GlassBarMargin() float32                                { return 0 }
func GlassInsets() (top, bottom float32)                     { return 0, 0 }
func GlassScroll(sc ui.Element) ui.Element                   { return sc }
func GlassTopEdge(*ui.Context, float32)                      {}
