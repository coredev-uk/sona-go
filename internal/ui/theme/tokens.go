package theme

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

//go:embed tokens.json
var embeddedTokens []byte

// Tokens are Sona's design tokens (docs/design.md) resolved for one mode.
// They are loaded from tokens.json: embedded in the binary, and in dev
// builds read from the file so a change shows on the next frame.
type Tokens struct {
	// Dark is the mode these tokens are for.
	Dark bool
	// Color holds every colour token: the custom properties of v0.2.0's
	// shared/styles/index.css by their names without the "--" ("ground",
	// "surface", "raised", "hover", "active", "line", "control-line", "ink",
	// "ink-muted", "ink-faint", "amber", "amber-ink", "on-amber", "danger",
	// "positive", "avatar-0-bg"…), the roles built on them ("play",
	// "play-ink", "on-play", "primary", "ring"…) and a few more the
	// components need ("edge": the artwork's 1px line, "selection",
	// "scrollbar"…).
	Color map[string]ui.Color
	// Type is the type scale: "display", "title", "heading", "body",
	// "label", "secondary", "meta", and the CSS names "page", "ui", "sub"
	// and "caption" as aliases.
	Type map[string]TypeStyle
	// Radius: "sm" (artwork, 4), "md" (controls, 6), "lg" (menus and
	// dialogs, 8), "xl", "full".
	Radius map[string]float32
	// Space is the 4px scale by multiple: "1" is 4, "2" is 8…
	Space map[string]float32
	// Metric holds the fixed sizes of the shell and the controls
	// ("sidebar", "top-bar", "player-bar", "control", "button-sm"…).
	Metric map[string]float32
	// Scalar holds unitless tokens ("backdrop-scrim": the scrim's opacity
	// over a backdrop, 0.92 in light).
	Scalar map[string]float32
	// Shadow holds the shadows: what floats ("float") and the raised active tab ("sm").
	Shadow map[string]Shadow
	// Motion holds durations and easings ("feedback": 120 ms, "float",
	// "route", "backdrop"…).
	Motion map[string]Motion
}

// TypeStyle is a row of the type scale, in DIPs.
type TypeStyle struct {
	Size, LineHeight, Weight, Tracking float32
}

// Motion is a duration and its easing.
type Motion struct {
	Duration time.Duration
	Ease     ui.Easing
}

// Shadow is a box shadow (ui.Element.Shadow's arguments).
type Shadow struct {
	X, Y, Blur, Spread float32
	Color              ui.Color
}

// Document is a parsed tokens.json: both modes, not yet resolved.
type Document struct {
	raw document
}

type shadowSpec struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Blur   float32 `json:"blur"`
	Spread float32 `json:"spread"`
	Color  string  `json:"color"`
}

type document struct {
	Color  map[string]map[string]string     `json:"color"`
	Scalar map[string]map[string]float32    `json:"scalar"`
	Shadow map[string]map[string]shadowSpec `json:"shadow"`
	Type   map[string]json.RawMessage       `json:"type"`
	Radius map[string]float32               `json:"radius"`
	Space  map[string]float32               `json:"space"`
	Metric map[string]float32               `json:"metric"`
	Ease   map[string][4]float32            `json:"ease"`
	Motion map[string]struct {
		Duration string `json:"duration"`
		Ease     string `json:"ease"`
	} `json:"motion"`
}

// Parse reads a tokens.json and checks that it resolves in both modes.
func Parse(data []byte) (*Document, error) {
	var raw document
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("theme: tokens: %w", err)
	}
	d := &Document{raw: raw}
	for _, dark := range []bool{false, true} {
		if _, err := d.Tokens(dark); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// LoadFile reads a tokens.json from path (dev builds watch it).
func LoadFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func modeName(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}

// Tokens resolves the document for a mode.
func (d *Document) Tokens(dark bool) (*Tokens, error) {
	mode := modeName(dark)
	t := &Tokens{
		Dark:   dark,
		Color:  map[string]ui.Color{},
		Type:   map[string]TypeStyle{},
		Radius: d.raw.Radius,
		Space:  d.raw.Space,
		Metric: d.raw.Metric,
		Scalar: map[string]float32{},
		Shadow: map[string]Shadow{},
		Motion: map[string]Motion{},
	}
	r := &resolver{src: d.raw.Color[mode], done: t.Color, visiting: map[string]bool{}}
	names := make([]string, 0, len(r.src))
	for name := range r.src {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := r.color(name); err != nil {
			return nil, fmt.Errorf("theme: tokens: color %q (%s): %w", name, mode, err)
		}
	}
	for name, byMode := range d.raw.Scalar {
		v, ok := byMode[mode]
		if !ok {
			return nil, fmt.Errorf("theme: tokens: scalar %q has no %s value", name, mode)
		}
		t.Scalar[name] = v
	}
	for name, byMode := range d.raw.Shadow {
		s, ok := byMode[mode]
		if !ok {
			return nil, fmt.Errorf("theme: tokens: shadow %q has no %s value", name, mode)
		}
		c, err := parseLiteral(s.Color)
		if err != nil {
			return nil, fmt.Errorf("theme: tokens: shadow %q: %w", name, err)
		}
		t.Shadow[name] = Shadow{s.X, s.Y, s.Blur, s.Spread, c}
	}
	for name := range d.raw.Type {
		ts, err := d.typeStyle(name, 0)
		if err != nil {
			return nil, err
		}
		t.Type[name] = ts
	}
	for name, m := range d.raw.Motion {
		dur, err := time.ParseDuration(m.Duration)
		if err != nil {
			return nil, fmt.Errorf("theme: tokens: motion %q: %w", name, err)
		}
		b, ok := d.raw.Ease[m.Ease]
		if !ok {
			return nil, fmt.Errorf("theme: tokens: motion %q: no easing %q", name, m.Ease)
		}
		t.Motion[name] = Motion{Duration: dur, Ease: easing(b)}
	}
	return t, nil
}

// easing is the cubic-bezier of a [x1 y1 x2 y2] token; the straight line is
// ui.Linear.
func easing(b [4]float32) ui.Easing {
	if b == [4]float32{0, 0, 1, 1} {
		return ui.Linear
	}
	return ui.CubicBezier(b[0], b[1], b[2], b[3])
}

// typeStyle reads a row of the scale: an object, or the name of another row.
func (d *Document) typeStyle(name string, depth int) (TypeStyle, error) {
	raw := d.raw.Type[name]
	var alias string
	if json.Unmarshal(raw, &alias) == nil {
		if depth > 8 || d.raw.Type[alias] == nil {
			return TypeStyle{}, fmt.Errorf("theme: tokens: type %q is an alias of %q, which is not a row", name, alias)
		}
		return d.typeStyle(alias, depth+1)
	}
	var row struct {
		Size       float32 `json:"size"`
		LineHeight float32 `json:"lineHeight"`
		Weight     float32 `json:"weight"`
		Tracking   float32 `json:"tracking"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return TypeStyle{}, fmt.Errorf("theme: tokens: type %q: %w", name, err)
	}
	if row.Size <= 0 || row.LineHeight <= 0 {
		return TypeStyle{}, fmt.Errorf("theme: tokens: type %q needs a size and a lineHeight", name)
	}
	return TypeStyle{row.Size, row.LineHeight, row.Weight, row.Tracking}, nil
}

// resolver turns colour sources into colours: "#rrggbb[aa]", the name of
// another token, or "name/N" (that token at N percent of its opacity).
type resolver struct {
	src      map[string]string
	done     map[string]ui.Color
	visiting map[string]bool
}

func (r *resolver) color(name string) (ui.Color, error) {
	if c, ok := r.done[name]; ok {
		return c, nil
	}
	s, ok := r.src[name]
	if !ok {
		return ui.Color{}, fmt.Errorf("no such token")
	}
	if r.visiting[name] {
		return ui.Color{}, fmt.Errorf("refers to itself")
	}
	r.visiting[name] = true
	defer delete(r.visiting, name)
	c, err := r.value(s)
	if err != nil {
		return ui.Color{}, err
	}
	r.done[name] = c
	return c, nil
}

func (r *resolver) value(s string) (ui.Color, error) {
	if strings.HasPrefix(s, "#") {
		return parseLiteral(s)
	}
	ref, pct, hasPct := strings.Cut(s, "/")
	c, err := r.color(ref)
	if err != nil {
		return ui.Color{}, fmt.Errorf("%q: %w", ref, err)
	}
	if !hasPct {
		return c, nil
	}
	p, err := strconv.ParseFloat(pct, 32)
	if err != nil || p < 0 || p > 100 {
		return ui.Color{}, fmt.Errorf("%q: opacity is a percentage", s)
	}
	return ui.RGBA(c.R, c.G, c.B, float32(c.A)/255*float32(p)/100), nil
}

// parseLiteral reads "#rgb", "#rrggbb" or "#rrggbbaa", any of them
// followed by "/N" percent opacity.
func parseLiteral(s string) (ui.Color, error) {
	hex, pct, hasPct := strings.Cut(s, "/")
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 && len(h) != 8 {
		return ui.Color{}, fmt.Errorf("%q is not a colour", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return ui.Color{}, fmt.Errorf("%q is not a colour", s)
	}
	alpha := float32(1)
	if len(h) == 8 {
		alpha = float32(v&0xff) / 255
		v >>= 8
	}
	if hasPct {
		p, err := strconv.ParseFloat(pct, 32)
		if err != nil || p < 0 || p > 100 {
			return ui.Color{}, fmt.Errorf("%q: opacity is a percentage", s)
		}
		alpha *= float32(p) / 100
	}
	return ui.RGBA(uint8(v>>16), uint8(v>>8), uint8(v), alpha), nil
}
