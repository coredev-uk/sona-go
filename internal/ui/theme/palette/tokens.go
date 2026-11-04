package palette

import "github.com/egoist/mygo/ui"

// Overrides are the colour tokens the sleeve redeclares (v0.2.0's
// sleeveVars): its own tokens, plus the roles built on them, which the token
// document resolves where they are declared, so they are named again here to
// follow the sleeve's. Apply them with theme.Scope; build them once per cover.
func (s Sleeve) Overrides() map[string]ui.Color {
	m := hexes(map[string]string{
		"ink":                  s.Ink,
		"ink-muted":            s.InkMuted,
		"ink-faint":            s.InkMuted,
		"hover":                s.Hover,
		"active":               s.Active,
		"line":                 s.Line,
		"control-line":         s.ControlLine,
		"foreground":           s.Ink,
		"primary":              s.Ink,
		"primary-foreground":   s.Surface,
		"secondary":            s.Hover,
		"secondary-foreground": s.Ink,
		"muted":                s.Hover,
		"muted-foreground":     s.InkMuted,
		"accent":               s.Hover,
		"accent-foreground":    s.Ink,
		"border":               s.Line,
		"input":                s.ControlLine,
		"ring":                 s.Ink,
	})
	if s.AccentInk != "" {
		c := ui.Hex(s.AccentInk)
		m["amber-ink"], m["play-ink"] = c, c
	}
	return m
}

// Overrides are the colour tokens that put the accent in amber's place on the
// page (v0.2.0's accentVars): amber's three tokens and the play roles built
// on them.
func (a Accent) Overrides() map[string]ui.Color {
	return hexes(map[string]string{
		"amber": a.Fill, "play": a.Fill,
		"on-amber": a.OnFill, "on-play": a.OnFill,
		"amber-ink": a.Ink, "play-ink": a.Ink,
	})
}

func hexes(in map[string]string) map[string]ui.Color {
	out := make(map[string]ui.Color, len(in)+2)
	for k, v := range in {
		out[k] = ui.Hex(v)
	}
	return out
}
