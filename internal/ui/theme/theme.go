// Package theme is Sona's look in code: the design tokens of docs/design.md
// (colours for light and dark, the type scale, radii, spacing, motion),
// their application to MyGo's theme, the Lucide icons and the interface
// font (docs/ui-architecture.md, "Theme tokens"). Pages never hard-code a
// colour or a size: theme.C("ink"), theme.Current().Type["title"].
package theme

import (
	"fmt"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Mode is the appearance the user chose (config.Settings.Theme).
type Mode int

// The modes. (Dark is taken by the func Dark, so they carry the type name.)
const (
	// ModeSystem follows the desktop's light or dark appearance.
	ModeSystem Mode = iota
	ModeLight
	ModeDark
)

// FontFamily is the interface font's family name, as RegisterFonts
// registers it.
const FontFamily = "Schibsted Grotesk"

// state is everything the package keeps; main thread, but guarded so that a
// settings goroutine can set the modes.
var state struct {
	mu   sync.Mutex
	doc  *Document
	mode Mode

	// What the last frame saw: the desktop's appearance and preferences.
	systemDark bool
	prefs      ui.Preferences

	reduceEffects bool // the setting
	reduceMotion  bool // the setting

	tokens [2]*Tokens // light, dark; nil until used after a (re)load
	themes map[themeKey]*ui.Theme
}

type themeKey struct {
	dark         bool
	textScale    float32
	highContrast bool
}

func init() {
	d, err := Parse(embeddedTokens)
	if err != nil {
		panic(err) // tokens.json is part of the binary: a test keeps it valid
	}
	state.doc = d
}

// strict says an unknown token or icon is a bug: it panics in tests and in
// development builds, and shows a placeholder in production.
func strict() bool { return devBuild || testing.Testing() }

// SetDocument replaces the tokens (dev builds reload tokens.json; tests
// load variants). Frames after it use the new values.
func SetDocument(d *Document) {
	state.mu.Lock()
	state.doc = d
	state.tokens = [2]*Tokens{}
	state.themes = nil
	state.mu.Unlock()
}

// Reload reads a tokens.json from path and uses it; on an error the
// current tokens stay.
func Reload(path string) error {
	d, err := LoadFile(path)
	if err != nil {
		return err
	}
	SetDocument(d)
	return nil
}

// ReloadEmbedded goes back to the tokens built into the binary.
func ReloadEmbedded() {
	d, err := Parse(embeddedTokens)
	if err != nil {
		panic(err)
	}
	SetDocument(d)
}

// SourceFile is tokens.json's path relative to the repository root: dev
// builds watch it and call Reload.
const SourceFile = "internal/ui/theme/tokens.json"

// SetMode sets the appearance: ModeSystem follows the desktop.
func SetMode(m Mode) {
	state.mu.Lock()
	state.mode = m
	state.mu.Unlock()
}

// Dark reports whether frames draw dark now: the chosen mode, or the
// desktop's appearance (as the last Apply saw it) under ModeSystem.
func Dark() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return darkLocked()
}

func darkLocked() bool {
	switch state.mode {
	case ModeLight:
		return false
	case ModeDark:
		return true
	}
	return state.systemDark
}

// Current returns the tokens for the current mode.
func Current() *Tokens {
	state.mu.Lock()
	defer state.mu.Unlock()
	return currentLocked()
}

func currentLocked() *Tokens {
	i := 0
	if darkLocked() {
		i = 1
	}
	if t := state.tokens[i]; t != nil {
		return t
	}
	t, err := state.doc.Tokens(i == 1)
	if err != nil {
		panic(err) // Parse resolved both modes
	}
	state.tokens[i] = t
	return t
}

// C returns a colour token of the current mode. An unknown name is a bug:
// it panics in tests and development builds, and in production is magenta
// so it shows.
func C(name string) ui.Color {
	if c, ok := Current().Color[name]; ok {
		return c
	}
	if strict() {
		panic(fmt.Sprintf("theme: no colour token %q", name))
	}
	return ui.RGB(255, 0, 255)
}

// SetReduceEffects is the Reduce effects setting (Settings → Appearance):
// no backdrops, solid surfaces, no motion.
func SetReduceEffects(on bool) {
	state.mu.Lock()
	state.reduceEffects = on
	state.mu.Unlock()
}

// SetReducedMotion is the setting that keeps motion to a minimum besides
// the desktop's own preference; Reduce effects implies it.
func SetReducedMotion(on bool) {
	state.mu.Lock()
	state.reduceMotion = on
	state.mu.Unlock()
}

// ReduceEffects reports that surfaces are solid: no backdrops, no blur
// (v0.2.0's .reduce-effects, set by the setting).
func ReduceEffects() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.reduceEffects
}

// ReducedMotion reports that nothing should move: the settings or the
// desktop's preference (as the last Apply saw it).
func ReducedMotion() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.reduceEffects || state.reduceMotion || state.prefs.ReduceMotion
}

// Apply sets the window's ui.Theme from the tokens, as MyGo's widgets take
// it: Sona's colours and type, whatever the desktop's own look. Call it at
// the top of every view; it also reads the desktop's appearance and
// preferences (MyGo starts each frame from a theme following them), and
// makes a new theme only when the tokens, the mode or the preferences
// changed.
func Apply(c *ui.Context) {
	sys := c.Theme()
	prefs := c.Preferences()
	state.mu.Lock()
	state.systemDark = sys.Dark
	state.prefs = prefs
	dark := darkLocked()
	key := themeKey{dark, prefs.TextScale, prefs.HighContrast}
	t := state.themes[key]
	if t == nil {
		t = build(currentLocked(), prefs)
		if state.themes == nil {
			state.themes = map[themeKey]*ui.Theme{}
		}
		state.themes[key] = t
	}
	state.mu.Unlock()
	c.SetTheme(t)
}

// build maps the tokens onto a ui.Theme, which the widgets (inputs,
// selects, sliders, switches, scroll bars, tooltips, toasts) draw from.
func build(tk *Tokens, prefs ui.Preferences) *ui.Theme {
	t := ui.LightTheme()
	if tk.Dark {
		t = ui.DarkTheme()
	}
	col := func(name string) ui.Color { return tk.Color[name] }
	t.Dark = tk.Dark
	t.Background = col("ground")
	t.Surface, t.SurfaceHover, t.SurfacePressed = col("hover"), col("line"), col("active")
	t.Border = col("line")
	t.Text, t.TextMuted = col("ink"), col("ink-muted")
	// Sliders, switches and the choice in Sona's play colour; the focus
	// ring in ink (docs/design.md, "Colour").
	t.Accent, t.AccentHover, t.AccentPressed = col("amber"), col("amber"), col("amber")
	t.AccentText = col("on-amber")
	t.Focus = col("ring")
	t.Danger, t.Success, t.Warning = col("danger"), col("positive"), col("amber-ink")
	t.Selection = col("selection")
	t.Scrollbar = col("scrollbar")
	// Tooltips and toasts are the theme turned over.
	t.Inverse, t.InverseText = col("ink"), col("ground")
	t.Radius = tk.Radius["md"]
	t.Font = FontFamily
	body := tk.Type["body"]
	t.FontSize = body.Size
	if prefs.TextScale > 0 {
		t.FontSize *= prefs.TextScale
	}
	return t
}
