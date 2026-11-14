package theme

import (
	"fmt"
	"maps"
	"slices"

	"github.com/egoist/mygo/ui"
)

// The accents are the spot colour ("amber", "amber-ink", "on-amber" and, with
// them, the play roles and the selection) a person picks for one of Sona's
// own themes in Settings → Appearance. Each is designed per theme and per
// variant, and written in that theme's file ("accents"); a combination that
// fails Validate is not offered. The accent changes nothing else: the spot
// colour still means "playing" and nothing else (docs/design.md).

// accentOrder is every accent a theme file may give, in the order Settings
// shows them (after "Theme default", which is the theme's own colour).
var accentOrder = [...]string{"amber", "gold", "lime", "sky", "teal", "rose"}

// AccentIDs is every accent ID a theme file may give, in display order.
func AccentIDs() []string { return slices.Clone(accentOrder[:]) }

// accentTokens are the colour tokens an accent sets, and the only ones.
var accentTokens = [...]string{"amber", "amber-ink", "on-amber"}

// accentFile is one accent of a theme file: the three tokens for each
// variant the theme gives.
type accentFile struct {
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

// accentValues are an accent's colour tokens for each variant; nil for a
// variant the theme does not give.
type accentValues struct{ light, dark map[string]string }

// Accents are the accents the theme offers, in display order; none for a
// community palette or a user's theme (their spot colour is their own).
func (t *Theme) Accents() []string {
	var out []string
	for _, id := range accentOrder {
		if t.accents[id] != nil {
			out = append(out, id)
		}
	}
	return out
}

// HasAccent reports that the theme offers the accent.
func (t *Theme) HasAccent(id string) bool { return t.accents[id] != nil }

// WithAccent is the theme with the accent laid over it: the same theme in
// every way, with the spot colour the accent gives. An ID the theme does not
// offer ("" for the theme's own colour among them) gives the theme itself.
// The result is shared, like the theme; do not edit it.
func (t *Theme) WithAccent(id string) *Theme {
	state.mu.Lock()
	defer state.mu.Unlock()
	return t.withAccentLocked(id)
}

func (t *Theme) withAccentLocked(id string) *Theme {
	a := t.accents[id]
	if a == nil {
		return t
	}
	if d := t.derived[id]; d != nil {
		return d
	}
	d := t.overlay(a)
	if t.derived == nil {
		t.derived = map[string]*Theme{}
	}
	t.derived[id] = d
	return d
}

// overlay is the theme with a's colours over its variants.
func (t *Theme) overlay(a *accentValues) *Theme {
	d := &Theme{ID: t.ID, Name: t.Name, Author: t.Author, Builtin: t.Builtin, File: t.File}
	for _, v := range []struct {
		from  *variant
		to    **variant
		extra map[string]string
	}{{t.light, &d.light, a.light}, {t.dark, &d.dark, a.dark}} {
		if v.from == nil {
			continue
		}
		n := *v.from
		n.color = maps.Clone(v.from.color)
		if n.color == nil {
			n.color = map[string]string{}
		}
		maps.Copy(n.color, v.extra)
		*v.to = &n
	}
	return d
}

// AccentColors are the accent's fill and the glyph colour on it in one
// variant: what a swatch shows. An ID the theme does not offer ("" among
// them) gives the theme's own.
func (t *Theme) AccentColors(id string, dark bool) (fill, glyph ui.Color) {
	tk := t.WithAccent(id).Tokens(dark)
	return tk.Color["amber"], tk.Color["on-amber"]
}

// readAccents checks a theme file's accents and keeps the ones that pass
// Validate in every variant the theme gives. A malformed accent (an unknown ID
// or token, a value that does not resolve, a variant the theme lacks, or one
// it has that the accent does not give) refuses the file; one that only fails
// the floors is left out, so it is never offered.
func readAccents(base *Document, t *Theme, in map[string]accentFile) (map[string]*accentValues, []Problem) {
	var out []Problem
	got := map[string]*accentValues{}
	for _, id := range slices.Sorted(maps.Keys(in)) {
		if !slices.Contains(accentOrder[:], id) {
			out = append(out, Problem{Kind: ProblemBadValue, Token: "accents", Detail: fmt.Sprintf("%q is not an accent Sona has", id)})
			continue
		}
		f := in[id]
		a := &accentValues{}
		for _, v := range []struct {
			dark bool
			have *variant
			in   map[string]string
			set  *map[string]string
		}{{false, t.light, f.Light, &a.light}, {true, t.dark, f.Dark, &a.dark}} {
			mode := modeName(v.dark)
			switch {
			case v.have == nil && v.in != nil:
				out = append(out, Problem{Kind: ProblemBadValue, Mode: mode, Token: "accents", Detail: id + " gives a variant the theme does not"})
			case v.have != nil && v.in == nil:
				out = append(out, Problem{Kind: ProblemBadValue, Mode: mode, Token: "accents", Detail: id + " has no " + mode + " variant"})
			case v.in != nil:
				for _, name := range accentTokens {
					if _, ok := v.in[name]; !ok {
						out = append(out, Problem{Kind: ProblemBadValue, Mode: mode, Token: name, Detail: id + " is missing it"})
					}
				}
				for name := range v.in {
					if !slices.Contains(accentTokens[:], name) {
						out = append(out, Problem{Kind: ProblemUnknownToken, Mode: mode, Token: name, Detail: "accent"})
					}
				}
				out = append(out, checkVariant(base, v.dark, &variant{color: v.in})...)
				*v.set = v.in
			}
		}
		got[id] = a
	}
	if len(out) > 0 {
		return nil, out
	}
	for id, a := range got {
		d := t.overlay(a)
		doc, err := d.document(base)
		if err != nil {
			return nil, []Problem{{Kind: ProblemBadValue, Token: "accents", Detail: err.Error()}}
		}
		for _, dark := range []bool{false, true} {
			if (dark && d.dark == nil) || (!dark && d.light == nil) {
				continue
			}
			tk, err := doc.Tokens(dark)
			if err != nil {
				return nil, []Problem{{Kind: ProblemBadValue, Mode: modeName(dark), Token: "accents", Detail: err.Error()}}
			}
			if len(Validate(tk)) > 0 {
				delete(got, id)
				break
			}
		}
	}
	return got, nil
}
