//go:build darwin

package kit

import (
	"os"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/unix"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The macOS glass spike (#210 §6.1). Nothing here is meant to be merged: it
// puts MyGo's glass on every surface the issue names, so the result can be
// judged on screen, and SONA_GLASS turns it off or picks surfaces so the
// same build can be captured with and without.
//
//	SONA_GLASS=0                     no glass anywhere (today's look)
//	SONA_GLASS=1 (or unset)          every surface
//	SONA_GLASS=window,bars,menus     only those surfaces
//
// Surfaces: window (sidebar vibrancy, transparent root), mini (the mini
// player's window), menus (menus and popovers), bars (player bar and top
// bar over the scrolling page), nowplaying (the panes of Now Playing).
//
// SONA_GLASS_TOPBAR=strip draws the top bar as a full-width pane of glass
// instead of a scroll edge.
const glassSurfaceList = "window,mini,menus,bars,nowplaying"

var (
	glassSurfaces = parseGlassSurfaces(os.Getenv("SONA_GLASS"))
	glassNative   = macOSMajor() >= 26
	glassStrip    = os.Getenv("SONA_GLASS_TOPBAR") == "strip"
	glassClarity  = glassClarityEnv()
)

func parseGlassSurfaces(v string) map[string]bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "off", "false", "none":
		return nil
	case "", "1", "on", "true", "all":
		v = glassSurfaceList
	}
	out := map[string]bool{}
	for _, s := range strings.Split(v, ",") {
		out[strings.TrimSpace(s)] = true
	}
	return out
}

func glassClarityEnv() float32 {
	f, err := strconv.ParseFloat(os.Getenv("SONA_GLASS_CLARITY"), 32)
	if err != nil {
		return 0
	}
	return float32(f)
}

// macOSMajor is the macOS major version (26 for Tahoe), 0 if unknown.
func macOSMajor() int {
	v, _ := unix.Sysctl("kern.osproductversion")
	major, _, _ := strings.Cut(v, ".")
	n, _ := strconv.Atoi(major)
	return n
}

// GlassOn reports whether the surface is drawn on glass in this run.
func GlassOn(surface string) bool { return glassSurfaces[surface] }

// GlassNative reports macOS 26 or later, which has Liquid Glass as a window
// material (VibrancyGlass); before it the window uses the sidebar material.
func GlassNative() bool { return glassNative }

// GlassTopStrip reports whether the top bar is a full-width glass strip
// rather than a scroll edge.
func GlassTopStrip() bool { return glassStrip }

// GlassPane makes e a pane of Liquid Glass of the given radius when the
// surface is on glass, and returns e either way. The caller draws no
// background of its own then. With Reduce Transparency the plugin draws the
// theme's Surface instead.
func GlassPane(e ui.Element, surface string, radius float32) ui.Element {
	if !GlassOn(surface) {
		return e
	}
	return e.Radius(radius).Material(glass.Glass{Clarity: glassClarity})
}

// glassBarInset is the room a floating bar takes: its height and the margin
// it floats at.
const glassBarMargin = 12

// GlassBarMargin is how far the player bar floats from the window's edges.
func GlassBarMargin() float32 { return glassBarMargin }

// GlassInsets are the room the floating bars take at the top and bottom of
// the page: 0 when the bars are not glass.
func GlassInsets() (top, bottom float32) {
	if !GlassOn("bars") {
		return 0, 0
	}
	m := theme.Current().Metric
	return m["top-bar"], m["player-bar"] + 2*glassBarMargin
}

// GlassScroll puts a page's scroll container under the floating bars: its
// content starts below the top bar and ends above the player bar, and
// scrolls under both.
func GlassScroll(sc ui.Element) ui.Element {
	top, bottom := GlassInsets()
	if top == 0 && bottom == 0 {
		return sc
	}
	return sc.Padding(top, 0, bottom).ScrollbarInsets(top, 0, bottom)
}

// GlassTopEdge paints the scroll edge under the top bar: the page's content
// fades into the ground as it scrolls up under the bar.
func GlassTopEdge(c *ui.Context, barHeight float32) {
	if !GlassOn("bars") || glassStrip {
		return
	}
	ui.Box(c.Key("glass-top-edge")).Absolute().Top(0).Left(0).Right(0).Height(barHeight + 10).PassThrough().
		Material(glass.ScrollEdge{Hard: os.Getenv("SONA_GLASS_EDGE") == "hard"})
}
