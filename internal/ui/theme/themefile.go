package theme

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// FormatVersion is the version of the theme file format this Sona reads.
const FormatVersion = 1

// Limits of a theme file's text fields.
const (
	maxIDLength     = 40
	maxNameLength   = 40
	maxAuthorLength = 80
	// maxShadowExtent bounds a shadow's offsets, spread and blur (DIPs): a
	// shadow is drawn on every raised surface.
	maxShadowExtent = 128
)

//go:embed themes/*.json
var builtinFiles embed.FS

// builtinOrder is the built-in themes in the order the gallery shows them:
// Sona's own five, then the community palettes; each is themes/<id>.json.
var builtinOrder = [...]string{
	"sona", "graphite", "paper", "contrast", "oled",
	"catppuccin", "nord", "gruvbox", "solarized", "dracula", "rose-pine", "tokyo-night",
}

// Theme is a named set of overrides of Sona's own tokens (docs/themes.md): a
// light variant, a dark variant or both, each overriding colour, scalar and
// shadow tokens. A token it leaves out comes from Sona's own document
// (tokens.json) of the same polarity, so a theme never shows a missing
// colour. A theme that gives one variant only pins the appearance to it.
// Themes are immutable once parsed.
type Theme struct {
	ID, Name, Author string
	// Builtin is true for the themes embedded in Sona.
	Builtin bool
	// File is the user theme's file name in the themes folder.
	File string

	light, dark *variant // nil: the variant is not given

	// cache is the resolved tokens of this theme over the base of that
	// revision; guarded by state.mu.
	cache struct {
		rev uint64
		tk  [2]*Tokens
	}
}

// variant is one polarity's overrides.
type variant struct {
	color  map[string]string
	scalar map[string]float32
	shadow map[string]shadowSpec
}

// HasLight reports that the theme gives a light variant.
func (t *Theme) HasLight() bool { return t.light != nil }

// HasDark reports that the theme gives a dark variant.
func (t *Theme) HasDark() bool { return t.dark != nil }

// pinned is the polarity a one-variant theme forces: dark, and whether it
// forces any.
func (t *Theme) pinned() (dark, ok bool) {
	switch {
	case t.light != nil && t.dark == nil:
		return false, true
	case t.dark != nil && t.light == nil:
		return true, true
	}
	return false, false
}

// Pinned reports whether the theme has one variant only, and which: such a
// theme pins the appearance to it, whatever the mode setting says.
func (t *Theme) Pinned() (dark, pinned bool) { return t.pinned() }

// file is the theme file as it is written.
type file struct {
	Comment string       `json:"$comment"`
	Format  int          `json:"format"`
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Author  string       `json:"author"`
	Light   *variantFile `json:"light"`
	Dark    *variantFile `json:"dark"`
}

type variantFile struct {
	Comment string                `json:"$comment"`
	Color   map[string]string     `json:"color"`
	Scalar  map[string]float32    `json:"scalar"`
	Shadow  map[string]shadowSpec `json:"shadow"`
}

// ParseTheme reads a theme file and checks it: the format, the names of the
// tokens, that every value resolves, and Validate's floors for each variant
// the file gives. It returns the theme, or no theme and the problems that
// refuse it.
func ParseTheme(data []byte) (*Theme, []Problem) {
	state.mu.Lock()
	base := state.base
	state.mu.Unlock()
	return parseTheme(data, base)
}

func parseTheme(data []byte, base *Document) (*Theme, []Problem) {
	var f file
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	err := dec.Decode(&f)
	if err == nil {
		if _, e := dec.Token(); e != io.EOF {
			err = fmt.Errorf("unexpected data after the theme")
		}
	}
	if err != nil {
		return nil, []Problem{{Kind: ProblemSyntax, Detail: err.Error()}}
	}
	var out []Problem
	if f.Format != FormatVersion {
		out = append(out, Problem{Kind: ProblemFormat, Detail: fmt.Sprint(f.Format)})
	}
	if !validID(f.ID) {
		out = append(out, Problem{Kind: ProblemID})
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(f.Name)); n < 1 || n > maxNameLength {
		out = append(out, Problem{Kind: ProblemName})
	}
	if f.Light == nil && f.Dark == nil {
		out = append(out, Problem{Kind: ProblemNoVariant})
	}
	t := &Theme{ID: f.ID, Name: strings.TrimSpace(f.Name), Author: strings.TrimSpace(f.Author)}
	if utf8.RuneCountInString(t.Author) > maxAuthorLength {
		t.Author = string([]rune(t.Author)[:maxAuthorLength])
	}
	for _, v := range []struct {
		dark bool
		in   *variantFile
		set  **variant
	}{{false, f.Light, &t.light}, {true, f.Dark, &t.dark}} {
		if v.in == nil {
			continue
		}
		*v.set = &variant{color: v.in.Color, scalar: v.in.Scalar, shadow: v.in.Shadow}
		out = append(out, checkVariant(base, v.dark, *v.set)...)
	}
	if len(out) > 0 {
		return nil, out
	}
	d, err := t.document(base)
	if err != nil {
		return nil, []Problem{{Kind: ProblemBadValue, Detail: err.Error()}}
	}
	for _, v := range []struct {
		dark bool
		set  *variant
	}{{false, t.light}, {true, t.dark}} {
		if v.set == nil {
			continue
		}
		tk, err := d.Tokens(v.dark)
		if err != nil {
			return nil, []Problem{{Kind: ProblemBadValue, Mode: modeName(v.dark), Detail: err.Error()}}
		}
		out = append(out, Validate(tk)...)
	}
	if len(out) > 0 {
		return nil, out
	}
	return t, nil
}

func validID(id string) bool {
	if id == "" || len(id) > maxIDLength {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// checkVariant reports the unknown token names of a variant and the values
// that don't resolve, each token once, in name order.
func checkVariant(base *Document, dark bool, v *variant) []Problem {
	mode := modeName(dark)
	var out []Problem
	known := base.raw.Color[mode]
	merged := maps.Clone(known)
	maps.Copy(merged, v.color)
	r := &resolver{src: merged, done: map[string]ui.Color{}, visiting: map[string]bool{}}
	for _, name := range slices.Sorted(maps.Keys(v.color)) {
		if _, ok := known[name]; !ok {
			out = append(out, Problem{Kind: ProblemUnknownToken, Mode: mode, Token: name, Detail: "color"})
			continue
		}
		if _, err := r.color(name); err != nil {
			out = append(out, Problem{Kind: ProblemBadValue, Mode: mode, Token: name, Detail: err.Error()})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(v.scalar)) {
		if _, ok := base.raw.Scalar[name]; !ok {
			out = append(out, Problem{Kind: ProblemUnknownToken, Mode: mode, Token: name, Detail: "scalar"})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(v.shadow)) {
		if _, ok := base.raw.Shadow[name]; !ok {
			out = append(out, Problem{Kind: ProblemUnknownToken, Mode: mode, Token: name, Detail: "shadow"})
			continue
		}
		s := v.shadow[name]
		if _, err := parseLiteral(s.Color); err != nil {
			out = append(out, Problem{Kind: ProblemBadValue, Mode: mode, Token: name, Detail: err.Error()})
		}
		for _, n := range []float32{s.X, s.Y, s.Spread} {
			if n < -maxShadowExtent || n > maxShadowExtent {
				out = append(out, Problem{Kind: ProblemBadValue, Mode: mode, Token: name, Detail: "offset and spread stay within ±128"})
				break
			}
		}
		if s.Blur < 0 || s.Blur > maxShadowExtent {
			out = append(out, Problem{Kind: ProblemBadValue, Mode: mode, Token: name, Detail: "blur is 0 to 128"})
		}
	}
	return out
}

// document is the theme laid over base: its own tokens where it gives them,
// Sona's where it doesn't, and Sona's whole variant for a polarity it does
// not give. Sona's own theme has nothing to override and is base itself.
func (t *Theme) document(base *Document) (*Document, error) {
	if t.empty() {
		return base, nil
	}
	raw := base.raw
	raw.Color = map[string]map[string]string{"light": maps.Clone(base.raw.Color["light"]), "dark": maps.Clone(base.raw.Color["dark"])}
	raw.Scalar = make(map[string]map[string]float32, len(base.raw.Scalar))
	for name, byMode := range base.raw.Scalar {
		raw.Scalar[name] = maps.Clone(byMode)
	}
	raw.Shadow = make(map[string]map[string]shadowSpec, len(base.raw.Shadow))
	for name, byMode := range base.raw.Shadow {
		raw.Shadow[name] = maps.Clone(byMode)
	}
	for _, v := range []struct {
		mode string
		set  *variant
	}{{"light", t.light}, {"dark", t.dark}} {
		if v.set == nil {
			continue
		}
		maps.Copy(raw.Color[v.mode], v.set.color)
		for name, val := range v.set.scalar {
			if raw.Scalar[name] != nil {
				raw.Scalar[name][v.mode] = val
			}
		}
		for name, val := range v.set.shadow {
			if raw.Shadow[name] != nil {
				raw.Shadow[name][v.mode] = val
			}
		}
	}
	d := &Document{raw: raw}
	for _, dark := range []bool{false, true} {
		if _, err := d.Tokens(dark); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// empty reports that the theme overrides nothing.
func (t *Theme) empty() bool {
	for _, v := range []*variant{t.light, t.dark} {
		if v != nil && (len(v.color) > 0 || len(v.scalar) > 0 || len(v.shadow) > 0) {
			return false
		}
	}
	return true
}

// Tokens resolves the theme's tokens for one appearance, whatever the
// current theme is: what draws a picture of the theme (the gallery in
// Settings → Appearance). A variant the theme does not give is Sona's own.
// The result is cached until the tokens change, so do not edit it; main
// thread, like Current.
func (t *Theme) Tokens(dark bool) *Tokens {
	state.mu.Lock()
	defer state.mu.Unlock()
	if t.cache.rev != state.rev {
		t.cache.rev, t.cache.tk = state.rev, [2]*Tokens{}
	}
	i := 0
	if dark {
		i = 1
	}
	if tk := t.cache.tk[i]; tk != nil {
		return tk
	}
	d, err := t.document(state.base)
	if err != nil {
		d = state.base // a base reloaded in development that this theme no longer fits
	}
	tk, err := d.Tokens(dark)
	if err != nil {
		panic(err) // Parse resolved both modes
	}
	t.cache.tk[i] = tk
	return tk
}

// builtins parses the embedded themes; a built-in that is not valid is a
// bug in the binary (a test keeps them valid).
func builtins(base *Document) []*Theme {
	var out []*Theme
	for _, id := range builtinOrder {
		data, err := builtinFiles.ReadFile("themes/" + id + ".json")
		if err != nil {
			panic(err)
		}
		t, problems := parseTheme(data, base)
		if t == nil {
			panic(fmt.Sprintf("theme: built-in %q: %v", id, problems))
		}
		if t.ID != id {
			panic(fmt.Sprintf("theme: themes/%s.json has the id %q", id, t.ID))
		}
		t.Builtin = true
		out = append(out, t)
	}
	return out
}
