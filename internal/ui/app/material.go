package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// materialSamples are what the window's material shows behind the sidebar
// with no tint, by the app's appearance (light, then dark): Liquid Glass and
// the sidebar material, active and inactive, over black, white, grey, red,
// orange, yellow, green, cyan, blue, purple and magenta desktops, measured
// on Apple silicon with macOS 27.2 (#317). Only the worst are kept: a sample
// no worse in every channel than another (lighter for dark text, darker for
// light text) is left out.
var materialSamples = [2][]ui.Color{
	{
		ui.RGB(124, 124, 255), ui.RGB(136, 255, 98), ui.RGB(215, 123, 255), ui.RGB(255, 255, 85),
		ui.RGB(247, 247, 89), ui.RGB(133, 133, 133), ui.RGB(255, 136, 121), ui.RGB(255, 183, 104),
	},
	{
		ui.RGB(62, 158, 33), ui.RGB(132, 132, 27), ui.RGB(144, 34, 255), ui.RGB(202, 108, 33),
		ui.RGB(252, 57, 39), ui.RGB(56, 147, 148), ui.RGB(239, 51, 252), ui.RGB(122, 122, 122),
	},
}

// tintSteps is how finely sidebarTint searches: in steps of 5%.
const tintSteps = 20

// sidebarTint is the opacity of the surface tone the sidebar lays over the
// window's material: the least, in steps of 5%, at which the sidebar's text
// (ink, ink-muted, ink-faint) keeps theme.TextFloor over every sample of the
// material; 1 where only a solid sidebar does.
func sidebarTint(tk *theme.Tokens) float32 {
	samples := materialSamples[0]
	if tk.Dark {
		samples = materialSamples[1]
	}
	surface := tk.Color["surface"]
	for i := range tintSteps {
		tint := float32(i) / tintSteps
		if tintHolds(tk, surface.Alpha(tint), samples) {
			return tint
		}
	}
	return 1
}

func tintHolds(tk *theme.Tokens, tinted ui.Color, samples []ui.Color) bool {
	for _, m := range samples {
		bg := tinted.Over(m)
		for _, ink := range [...]string{"ink", "ink-muted", "ink-faint"} {
			if theme.Contrast(tk.Color[ink].Over(bg), bg) < theme.TextFloor {
				return false
			}
		}
	}
	return true
}

// tintFor is the sidebar's tint for the tokens on screen, worked out again
// only when they change (a theme, a mode).
func (a *App) tintFor(tk *theme.Tokens) float32 {
	if tk != a.tintTokens {
		a.tintTokens, a.tint = tk, sidebarTint(tk)
	}
	return a.tint
}

// followMaterial keeps the main window's material off while Reduce effects
// or the desktop's Reduce Transparency asks for solid surfaces, or the
// theme's text would need a solid sidebar over it, and on otherwise. It
// returns the sidebar's tint over the material this frame, or 1 for a solid
// sidebar: only where the window has a material and shows it (c.Vibrancy).
func (a *App) followMaterial(c *ui.Context) float32 {
	want := a.material
	tint := a.tintFor(theme.Current())
	if theme.ReduceEffects() || c.Preferences().ReduceTransparency || tint >= 1 {
		want = mygo.VibrancyNone
	}
	if want != a.materialSet {
		a.materialSet = want
		a.mu.Lock()
		win := a.win
		a.mu.Unlock()
		if win != nil {
			win.SetVibrancy(want)
		}
	}
	if want == mygo.VibrancyNone || !c.Vibrancy() {
		return 1
	}
	return tint
}

// followAppearance has the system draw what it draws of the windows (the
// material, the window buttons) light or dark as Sona draws: the app's
// appearance is the polarity a theme or the mode setting fixes, else the
// desktop's. Only where the window has a material (macOS).
func (a *App) followAppearance() {
	if a.material == mygo.VibrancyNone {
		return
	}
	src := mygo.ThemeSystem
	if dark, fixed := theme.Fixed(); fixed {
		src = mygo.ThemeLight
		if dark {
			src = mygo.ThemeDark
		}
	}
	if src != a.appearance {
		a.appearance = src
		mygo.Theme.SetSource(src)
	}
}
