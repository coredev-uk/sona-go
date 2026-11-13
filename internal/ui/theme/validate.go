package theme

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

// Luminance is a colour's WCAG relative luminance, 0 (black) to 1 (white).
func Luminance(c ui.Color) float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// Contrast is the WCAG contrast ratio of two opaque colours, 1 to 21.
func Contrast(a, b ui.Color) float64 {
	la, lb := Luminance(a), Luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// The floors Validate holds a theme to (docs/design.md "Colour", docs/themes.md).
const (
	// TextFloor is text against any background it sits on (WCAG AA).
	TextFloor = 4.5
	// UIFloor is a control's line and the focus ring (WCAG AA, non-text).
	UIFloor = 3.0
	// SpotGroundFloorLight is the spot colour's fill against the ground in
	// a light theme; Sona's own amber has 2.0:1 there.
	SpotGroundFloorLight = 2.0
	// SpotRowFloorLight is the fill against the surface and the hover tone in
	// a light theme (amber has 1.88:1 and 1.76:1).
	SpotRowFloorLight = 1.75
	// SpotFloorDark is the fill against the ground, surface and hover tone in
	// a dark theme.
	SpotFloorDark = 3.0
	// MinBackdropScrim is the least opacity the scrim over a backdrop may have.
	MinBackdropScrim = 0.8
)

// ProblemKind says what is wrong with a theme; the UI words each kind in
// the user's language, with the Problem's fields as the parameters.
type ProblemKind int

// The kinds of problem.
const (
	// ProblemSyntax: the file is not JSON, or has a field that is not part of
	// the format. Detail is the parser's message.
	ProblemSyntax ProblemKind = iota
	// ProblemFormat: "format" is not a version Sona reads. Detail is the value.
	ProblemFormat
	// ProblemID: "id" is missing or not 1 to 40 of a-z, 0-9 and "-".
	ProblemID
	// ProblemIDTaken: another theme (a built-in, or a file earlier in the
	// folder) has the same id.
	ProblemIDTaken
	// ProblemName: "name" is missing or longer than 40 characters.
	ProblemName
	// ProblemNoVariant: neither "light" nor "dark" is given.
	ProblemNoVariant
	// ProblemTooLarge: the file is bigger than the size cap.
	ProblemTooLarge
	// ProblemUnknownToken: Token is not a token Sona has. Detail is its group
	// ("color", "scalar" or "shadow").
	ProblemUnknownToken
	// ProblemBadValue: the value of Token can't be used. Detail says why.
	ProblemBadValue
	// ProblemContrast: Token (the foreground) has Got:1 on Other (the
	// background), and needs Want:1.
	ProblemContrast
	// ProblemRange: Token is Got and has to be at least Want.
	ProblemRange
	// ProblemPlayRole: the play role Token does not follow the spot colour.
	ProblemPlayRole
	// ProblemSpotRole: the role Token resolves to the spot colour, which is
	// only for what plays.
	ProblemSpotRole
	// ProblemUnreadable: the file could not be read.
	ProblemUnreadable
)

// Problem is one reason a theme was refused.
type Problem struct {
	Kind ProblemKind
	// Mode is "light" or "dark" for a problem of one variant, "" otherwise.
	Mode string
	// Token is the token concerned; for ProblemContrast the foreground.
	Token string
	// Other is the background of a ProblemContrast.
	Other string
	// Detail is Go's own wording of a parse failure, for logs and for the
	// UI to show after its own sentence.
	Detail string
	// Got and Want are the measured and the required value.
	Got, Want float64
}

func (p Problem) String() string {
	where := ""
	if p.Mode != "" {
		where = p.Mode + ": "
	}
	switch p.Kind {
	case ProblemSyntax:
		return where + "not a valid theme file: " + p.Detail
	case ProblemFormat:
		return where + "format " + p.Detail + " is not supported"
	case ProblemID:
		return where + "id must be 1 to 40 of a-z, 0-9 and -"
	case ProblemIDTaken:
		return where + "id is already used by another theme"
	case ProblemName:
		return where + "name must be 1 to 40 characters"
	case ProblemNoVariant:
		return where + "give a light variant, a dark variant or both"
	case ProblemTooLarge:
		return where + "file is too large"
	case ProblemUnreadable:
		return where + "file can't be read"
	case ProblemUnknownToken:
		return fmt.Sprintf("%s%s token %q does not exist", where, p.Detail, p.Token)
	case ProblemBadValue:
		return fmt.Sprintf("%s%s: %s", where, p.Token, p.Detail)
	case ProblemContrast:
		return fmt.Sprintf("%s%s on %s is %.2f:1, needs %.1f:1", where, p.Token, p.Other, p.Got, p.Want)
	case ProblemRange:
		return fmt.Sprintf("%s%s is %.2f, needs at least %.2f", where, p.Token, p.Got, p.Want)
	case ProblemPlayRole:
		return fmt.Sprintf("%s%s must follow the spot colour", where, p.Token)
	case ProblemSpotRole:
		return fmt.Sprintf("%s%s must not be the spot colour: it marks what plays", where, p.Token)
	}
	return where + "problem"
}

// rowTones are the backgrounds text sits on, up to the pressed tone.
var rowTones = [...]string{"ground", "surface", "raised", "hover", "active"}

// textTokens are the colours that carry text on those tones: the three ink
// tiers, the spot colour as text, and the two status colours.
var textTokens = [...]string{"ink", "ink-muted", "ink-faint", "amber-ink", "positive", "danger"}

// spotRoles are the roles that must not resolve to the spot colour: the
// spot colour means playing and nothing else (docs/design.md, principle 3).
// ring is the focus ring too; active and hover are the navigation's
// selected and hovered rows.
var spotRoles = [...]string{
	"primary", "primary-foreground", "ring", "active", "hover", "accent", "secondary", "muted",
}

// Validate checks one resolved appearance against the floors and returns
// what fails, nothing when the tokens are fit to ship. It is the single rule
// set: the runtime refuses a theme that fails, and the tests hold every
// built-in to it.
//
//   - text (ink, ink-muted, ink-faint, positive, danger and the spot colour as
//     text, amber-ink) keeps 4.5:1 on every tone up to active and on a
//     selected row (ink at 12% over the ground);
//   - control-line and the ring keep 3:1 on the ground, surface and raised;
//   - every avatar tone's text keeps 4.5:1 on its fill;
//   - the backdrop scrim stays at 0.8 or more;
//   - the spot colour (amber) keeps its distance from ground, surface and
//     hover (2:1, 1.75:1 and 1.75:1 in light, 3:1 in dark), its glyph colour
//     (on-amber) 4.5:1 on it, and ink keeps 4.5:1 on the selection (the spot
//     colour at 32%);
//   - the play roles follow the spot colour, and no role listed in spotRoles
//     resolves to it.
func Validate(tk *Tokens) []Problem {
	mode := modeName(tk.Dark)
	var out []Problem
	col := func(name string) ui.Color { return tk.Color[name] }
	opaque := func(c, under ui.Color) ui.Color {
		if c.A == 255 {
			return c
		}
		return c.Over(under)
	}
	add := func(p Problem) {
		p.Mode = mode
		out = append(out, p)
	}
	missing := false
	for _, name := range []string{
		"ground", "surface", "raised", "hover", "active", "ink", "ink-muted", "ink-faint", "amber", "amber-ink",
		"on-amber", "positive", "danger", "control-line", "ring", "selection", "play", "play-ink", "on-play",
	} {
		if _, ok := tk.Color[name]; !ok {
			add(Problem{Kind: ProblemBadValue, Token: name, Detail: "is not defined"})
			missing = true
		}
	}
	if missing {
		return out
	}

	ground := col("ground")
	selectedRow := opaque(col("ink").Alpha(0.12), ground)
	tones := make([]struct {
		name string
		c    ui.Color
	}, 0, len(rowTones)+1)
	for _, name := range rowTones {
		tones = append(tones, struct {
			name string
			c    ui.Color
		}{name, opaque(col(name), ground)})
	}
	tones = append(tones, struct {
		name string
		c    ui.Color
	}{"selected row", selectedRow})
	for _, bg := range tones {
		for _, fg := range textTokens {
			if c := Contrast(opaque(col(fg), bg.c), bg.c); c < TextFloor {
				add(Problem{Kind: ProblemContrast, Token: fg, Other: bg.name, Got: c, Want: TextFloor})
			}
		}
	}
	for _, name := range [...]string{"ground", "surface", "raised"} {
		bg := opaque(col(name), ground)
		for _, fg := range [...]string{"control-line", "ring"} {
			if c := Contrast(opaque(col(fg), bg), bg); c < UIFloor {
				add(Problem{Kind: ProblemContrast, Token: fg, Other: name, Got: c, Want: UIFloor})
			}
		}
	}
	for i := range 8 {
		bgName, fgName := fmt.Sprintf("avatar-%d-bg", i), fmt.Sprintf("avatar-%d-fg", i)
		bg, fg := tk.Color[bgName], tk.Color[fgName]
		if c := Contrast(opaque(fg, opaque(bg, ground)), opaque(bg, ground)); c < TextFloor {
			add(Problem{Kind: ProblemContrast, Token: fgName, Other: bgName, Got: c, Want: TextFloor})
		}
	}
	if v, ok := tk.Scalar["backdrop-scrim"]; ok && float64(v) < MinBackdropScrim {
		add(Problem{Kind: ProblemRange, Token: "backdrop-scrim", Got: float64(v), Want: MinBackdropScrim})
	}

	// The spot colour.
	fill := opaque(col("amber"), ground)
	for i, name := range [...]string{"ground", "surface", "hover"} {
		floor := SpotFloorDark
		if !tk.Dark {
			floor = SpotRowFloorLight
			if i == 0 {
				floor = SpotGroundFloorLight
			}
		}
		if c := Contrast(fill, opaque(col(name), ground)); c < floor {
			add(Problem{Kind: ProblemContrast, Token: "amber", Other: name, Got: c, Want: floor})
		}
	}
	if c := Contrast(opaque(col("on-amber"), fill), fill); c < TextFloor {
		add(Problem{Kind: ProblemContrast, Token: "on-amber", Other: "amber", Got: c, Want: TextFloor})
	}
	for _, name := range [...]string{"ground", "surface", "raised"} {
		bg := opaque(col(name), ground)
		sel := opaque(col("selection"), bg)
		if c := Contrast(opaque(col("ink"), sel), sel); c < TextFloor {
			add(Problem{Kind: ProblemContrast, Token: "ink", Other: "selection on " + name, Got: c, Want: TextFloor})
		}
	}
	for _, r := range [...]struct{ role, spot string }{{"play", "amber"}, {"play-ink", "amber-ink"}, {"on-play", "on-amber"}} {
		if col(r.role) != col(r.spot) {
			add(Problem{Kind: ProblemPlayRole, Token: r.role})
		}
	}
	rgbOf := func(c ui.Color) [3]uint8 { return [3]uint8{c.R, c.G, c.B} }
	for _, role := range spotRoles {
		c, ok := tk.Color[role]
		if !ok || c.A == 0 {
			continue
		}
		if rgbOf(c) == rgbOf(col("amber")) || rgbOf(c) == rgbOf(col("amber-ink")) {
			add(Problem{Kind: ProblemSpotRole, Token: role})
		}
	}
	return out
}
