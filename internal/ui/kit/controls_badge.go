package kit

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// BadgeVariant is a Badge's look (shared/ui/badge.tsx).
type BadgeVariant int

// The badge variants.
const (
	BadgeDefault BadgeVariant = iota // ink fill
	BadgeSecondary
	BadgeDestructive
	BadgeOutline
	BadgeGhost
	BadgeLink
)

// BadgeOpts are the options of Badge.
type BadgeOpts struct {
	Variant BadgeVariant
	// Icon is a Lucide name shown before the label (12px).
	Icon string
	// Interactive makes the badge the anchor of badge.tsx's asChild: it
	// takes the focus, shows a hover state and reports Clicked.
	Interactive bool
}

// Badge is a pill label: 20px high, 12px medium text. With Interactive it is
// a focusable, clickable element (a tag that links somewhere).
func Badge(c *ui.Context, label string, o BadgeOpts) ui.Element {
	var e ui.Element
	if o.Interactive {
		e = ui.ButtonBase(c)
	} else {
		e = ui.Row(c).Shrink(0).AlignItems(ui.Center)
	}
	hovered := o.Interactive && e.Hovered()
	bg, border, fg := ui.Transparent, ui.Transparent, theme.C("foreground")
	switch o.Variant {
	case BadgeSecondary:
		bg, fg = theme.C("secondary"), theme.C("secondary-foreground")
		if hovered {
			bg = bg.Alpha(0.9)
		}
	case BadgeDestructive:
		bg, fg = theme.C("destructive"), ui.RGB(255, 255, 255)
		if theme.Dark() {
			bg = bg.Alpha(0.6)
		}
		if hovered {
			bg = bg.Alpha(0.9)
		}
	case BadgeOutline:
		border = theme.C("border")
		if hovered {
			bg, fg = theme.C("accent"), theme.C("accent-foreground")
		}
	case BadgeGhost:
		if hovered {
			bg, fg = theme.C("accent"), theme.C("accent-foreground")
		}
	case BadgeLink:
		fg = theme.C("primary")
	default:
		bg, fg = theme.C("primary"), theme.C("primary-foreground")
		if hovered {
			bg = bg.Alpha(0.9)
		}
	}
	e.Height(20).PaddingX(8).Gap(4).Radius(controlsRadius("full")).Background(bg).TextColor(fg).Border(1, border).Justify(ui.Center)
	if !theme.ReducedMotion() && o.Interactive {
		m := theme.Current().Motion["feedback"]
		e.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	e.Children(func() {
		if o.Icon != "" {
			ui.Icon(c, theme.Icon(o.Icon)).FontSize(12)
		}
		t := controlsText(c, label, "meta").FontWeight(500).SingleLine()
		if o.Variant == BadgeLink && hovered {
			t.Underline()
		}
	})
	return e
}

// ExplicitMark is the small "E" beside explicit titles in rows and cards: a
// 14px square (Badges.tsx), an image named "Explicit" with that tooltip.
func ExplicitMark(c *ui.Context) ui.Element {
	e := ui.Row(c).Size(14, 14).Shrink(0).Center().Radius(3).Background(theme.C("ink-faint").Alpha(0.25)).
		Role(ui.RoleImage).Label(i18n.T("Explicit"))
	e.Children(func() {
		ui.Text(c, i18n.TC("explicit content mark", "E")).FontSize(9).FixedLineHeight(9).FontWeight(600).
			TextColor(theme.C("ink-muted")).NoWrap()
	})
	return controlsTooltip(c, e, i18n.T("Explicit"), false)
}

// Tag is a small bordered label, 20px high (Badges.tsx). title is its
// tooltip, and what assistive technology reads as its description.
func Tag(c *ui.Context, text, title string) ui.Element {
	e := ui.Row(c).Height(20).PaddingX(6).Shrink(0).AlignItems(ui.Center).Radius(4).Border(1, theme.C("control-line"))
	e.Children(func() {
		ui.Text(c, text).FontSize(11).FixedLineHeight(11).FontWeight(500).TextColor(theme.C("ink-muted")).SingleLine()
	})
	return Tooltip(c, e, title)
}

// PreviewMark reads "Preview" beside a song that plays as Apple Music's
// 30-second preview rather than the full track.
func PreviewMark(c *ui.Context) ui.Element {
	return Tag(c, i18n.TC("a song playing as its 30-second preview", "Preview"), i18n.T("Apple Music's 30-second preview, not the full song"))
}

// SDMark reads "SD" beside a music video playing in standard definition, the
// only quality Apple licenses a software-protected player.
func SDMark(c *ui.Context) ui.Element {
	return Tag(c, i18n.TC("standard-definition video, as a short badge", "SD"), i18n.T("Standard definition: Apple allows HD only on players with hardware protection"))
}

// TraitLabels are Apple's audio labels for a release, most specific first.
// Labels only: Sona streams AAC.
func TraitLabels(t services.Traits) []string {
	var out []string
	switch {
	case t.Atmos:
		out = append(out, i18n.T("Dolby Atmos"))
	case t.Spatial:
		out = append(out, i18n.T("Spatial Audio"))
	}
	switch {
	case t.HiResLossless:
		out = append(out, i18n.T("Hi-Res Lossless"))
	case t.Lossless:
		out = append(out, i18n.T("Lossless"))
	}
	if t.AppleDigitalMaster {
		out = append(out, i18n.T("Apple Digital Master"))
	}
	return out
}

// BadgesOpts are the options of Badges.
type BadgesOpts struct {
	Explicit bool
	Traits   services.Traits
	// Extra are further labels after Apple's audio ones (a video's "4K", "HDR").
	Extra []string
}

// Badges is the badge line of a collection or song header: Explicit, then
// Apple's audio labels (each with a tooltip saying they describe the
// release as Apple offers it, not what Sona plays), then Extra. It builds
// nothing and returns the zero Element when there is nothing to show.
func Badges(c *ui.Context, o BadgesOpts) ui.Element {
	audio := TraitLabels(o.Traits)
	if !o.Explicit && len(audio) == 0 && len(o.Extra) == 0 {
		return ui.Element{}
	}
	row := ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center)
	row.Children(func() {
		if o.Explicit {
			Tag(c.Key("explicit"), i18n.T("Explicit"), "")
		}
		for _, label := range audio {
			Tag(c.Key("trait-"+label), label, i18n.T("{label}: Apple's label for this release. Sona plays it as AAC.", "label", label))
		}
		for _, l := range o.Extra {
			Tag(c.Key("extra-"+l), l, "")
		}
	})
	return row
}

// Separator is a 1px --border line: horizontal across its parent, or
// vertical along it. It is decorative: no role, hidden from assistive
// technology (shared/ui/separator.tsx).
func Separator(c *ui.Context, vertical bool) ui.Element {
	e := ui.Box(c).Shrink(0).Role(ui.RoleNone).Background(theme.C("border"))
	if vertical {
		return e.Width(1).FillHeight()
	}
	return e.Height(1).FillWidth()
}

// Skeleton is a rounded --accent block that pulses between full and half
// opacity every 2 s (animate-pulse), still when motion is reduced. A zero
// width fills the parent.
func Skeleton(c *ui.Context, w, h float32) ui.Element {
	e := ui.Box(c).Height(h).Shrink(0).Role(ui.RoleNone)
	if w > 0 {
		e.Width(w)
	} else {
		e.FillWidth()
	}
	fill, still := theme.C("accent"), theme.ReducedMotion()
	radius := controlsRadius("md")
	return e.Draw(func(p *ui.Painter, r ui.Rect) {
		a := float32(1)
		if !still {
			p.AnimationFrame()
			phase := float64(p.Now().UnixMilli()%2000) / 2000
			a = float32(0.75 + 0.25*math.Cos(2*math.Pi*phase))
		}
		p.Fill(r, fill.Alpha(a), radius)
	})
}
