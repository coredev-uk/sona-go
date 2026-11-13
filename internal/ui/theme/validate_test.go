package theme

import (
	"maps"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// sona returns a copy of Sona's own tokens for one appearance, for a test to
// break.
func sona(t *testing.T, dark bool) *Tokens {
	t.Helper()
	tk, err := state.base.Tokens(dark)
	if err != nil {
		t.Fatal(err)
	}
	c := *tk
	c.Color = maps.Clone(tk.Color)
	c.Scalar = maps.Clone(tk.Scalar)
	return &c
}

// has reports whether a problem of that kind, about that token, is listed.
func has(ps []Problem, kind ProblemKind, token string) bool {
	for _, p := range ps {
		if p.Kind == kind && p.Token == token {
			return true
		}
	}
	return false
}

func TestValidateRefusesEachKindOfFailure(t *testing.T) {
	for _, dark := range []bool{false, true} {
		mode := modeName(dark)
		for name, c := range map[string]struct {
			edit  func(tk *Tokens)
			kind  ProblemKind
			token string
		}{
			"muted text too faint": {func(tk *Tokens) { tk.Color["ink-muted"] = tk.Color["ground"] }, ProblemContrast, "ink-muted"},
			"ink on a pressed row": {func(tk *Tokens) { tk.Color["ink"] = tk.Color["active"] }, ProblemContrast, "ink"},
			"danger text":          {func(tk *Tokens) { tk.Color["danger"] = tk.Color["ground"] }, ProblemContrast, "danger"},
			"spot as text": {func(tk *Tokens) {
				tk.Color["amber-ink"] = tk.Color["ground"]
				tk.Color["play-ink"] = tk.Color["ground"]
			}, ProblemContrast, "amber-ink"},
			"control line":  {func(tk *Tokens) { tk.Color["control-line"] = tk.Color["ground"] }, ProblemContrast, "control-line"},
			"focus ring":    {func(tk *Tokens) { tk.Color["ring"] = tk.Color["ground"].Alpha(0.2) }, ProblemContrast, "ring"},
			"avatar text":   {func(tk *Tokens) { tk.Color["avatar-3-fg"] = tk.Color["avatar-3-bg"] }, ProblemContrast, "avatar-3-fg"},
			"scrim":         {func(tk *Tokens) { tk.Scalar["backdrop-scrim"] = 0.3 }, ProblemRange, "backdrop-scrim"},
			"spot fill lost": {func(tk *Tokens) {
				tk.Color["amber"] = tk.Color["ground"]
				tk.Color["play"] = tk.Color["ground"]
			}, ProblemContrast, "amber"},
			"glyph on the spot": {func(tk *Tokens) {
				tk.Color["on-amber"] = tk.Color["amber"]
				tk.Color["on-play"] = tk.Color["amber"]
			}, ProblemContrast, "on-amber"},
			"play role apart from the spot": {func(tk *Tokens) { tk.Color["play"] = tk.Color["ink"] }, ProblemPlayRole, "play"},
			"primary is the spot colour":    {func(tk *Tokens) { tk.Color["primary"] = tk.Color["amber"] }, ProblemSpotRole, "primary"},
			"ring is the spot colour":       {func(tk *Tokens) { tk.Color["ring"] = tk.Color["amber"] }, ProblemSpotRole, "ring"},
			"selected nav row is the spot":  {func(tk *Tokens) { tk.Color["active"] = tk.Color["amber-ink"] }, ProblemSpotRole, "active"},
			"a token missing":               {func(tk *Tokens) { delete(tk.Color, "surface") }, ProblemBadValue, "surface"},
		} {
			tk := sona(t, dark)
			c.edit(tk)
			if ps := Validate(tk); !has(ps, c.kind, c.token) {
				t.Errorf("%s, %s: no problem of kind %d about %s: %v", mode, name, c.kind, c.token, ps)
			}
		}
	}
}

// The selection (the spot colour at 32%) has to keep the text readable: a
// spot colour as loud as ink does not.
func TestValidateChecksTheTextOnTheSelection(t *testing.T) {
	tk := sona(t, false)
	tk.Color["selection"] = tk.Color["ink"].Alpha(0.9)
	if ps := Validate(tk); !has(ps, ProblemContrast, "ink") {
		t.Errorf("ink on a dark selection passed: %v", ps)
	}
}

func TestSpotFloorIsLowerInLightThanInDark(t *testing.T) {
	// A fill 2.3:1 from the ground passes in a light theme, not in a dark one.
	light, dark := sona(t, false), sona(t, true)
	fill := ui.Hex("#d9a13a")
	if c := Contrast(fill, light.Color["ground"]); c < 2 {
		t.Fatalf("test fill is %.2f:1 on the light ground", c)
	}
	light.Color["amber"], light.Color["play"] = fill, fill
	if ps := Validate(light); has(ps, ProblemContrast, "amber") {
		t.Errorf("light: %v", ps)
	}
	dark.Color["amber"], dark.Color["play"] = fill.Mix(dark.Color["ground"], 0.7), fill.Mix(dark.Color["ground"], 0.7)
	if ps := Validate(dark); !has(ps, ProblemContrast, "amber") {
		t.Errorf("dark: a dim spot colour passed: %v", ps)
	}
}

func TestProblemWordsEveryKind(t *testing.T) {
	for k := ProblemSyntax; k <= ProblemUnreadable; k++ {
		if s := (Problem{Kind: k, Token: "x", Detail: "d", Mode: "dark"}).String(); s == "" || strings.HasSuffix(s, "problem") {
			t.Errorf("kind %d has no wording: %q", k, s)
		}
	}
}
