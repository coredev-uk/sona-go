//go:build darwin

package app

import (
	"log/slog"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The macOS glass spike (#210 §6.1), the window side: see kit/glass_darwin.go.

// glassWindow gives the main window its material, behind a transparent root
// and sidebar (view.go): Liquid Glass on macOS 26 and later, the sidebar
// material before it.
func glassWindow(o *mygo.WindowOptions) {
	if !kit.GlassOn("window") {
		return
	}
	o.Vibrancy = mygo.VibrancySidebar
	if kit.GlassNative() {
		o.Vibrancy = mygo.VibrancyGlass
	}
}

// glassMini gives the mini player window glass and rounded corners.
func glassMini(o *mygo.WindowOptions) {
	if !kit.GlassOn("mini") {
		return
	}
	o.Vibrancy = mygo.VibrancyPopover
	if kit.GlassNative() {
		o.Vibrancy = mygo.VibrancyGlass
	}
	o.CornerRadius = 20
}

var glassSeen struct {
	set                                              bool
	vibrancy, reduce, contrast, dark, active, motion bool
}

// glassReport logs what the window and the desktop say, whenever it changes,
// so a capture run can tell whether the window had its material, whether
// Reduce Transparency reached the app and whether the window was active.
func glassReport(c *ui.Context, log *slog.Logger) {
	p := c.Preferences()
	now := glassSeen
	now.set = true
	now.vibrancy, now.reduce, now.contrast, now.motion = c.Vibrancy(), p.ReduceTransparency, p.HighContrast, p.ReduceMotion
	now.dark, now.active = c.Theme().Dark, c.WindowActive()
	if now == glassSeen {
		return
	}
	glassSeen = now
	log.Info("glass spike", "vibrancy", now.vibrancy, "reduceTransparency", now.reduce, "increaseContrast", now.contrast,
		"reduceMotion", now.motion, "dark", now.dark, "windowActive", now.active, "native", kit.GlassNative(), "SONA_GLASS", os.Getenv("SONA_GLASS"))
}
