package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Tooltip shows text beside anchor once the pointer rests on it or the
// keyboard focus comes to it, and describes anchor to assistive technology
// (shared/ui/tooltip.tsx): an ink bubble with 12px text and an arrow,
// above the anchor or below it where there is no room above. Escape hides
// it first, before it closes anything around (MyGo's TooltipBase does that,
// and shows the next tip at once within 0.4 s of the last). It returns
// anchor; build anchor first, and give it children before this call.
//
//	kit.Tooltip(c, kit.Button(c, "", opts), i18n.T("Play"))
//
// MyGo decides the delay (0.6 s, where v0.2.0's provider said 400 ms).
func Tooltip(c *ui.Context, anchor ui.Element, text string) ui.Element {
	return controlsTooltip(c, anchor, text, true)
}

// controlsTooltip is Tooltip; describe says whether the tip is also what
// assistive technology reads as the anchor's description (not when it only
// repeats the anchor's name).
func controlsTooltip(c *ui.Context, anchor ui.Element, text string, describe bool) ui.Element {
	return controlsTooltipKeys(c, anchor, text, describe, nil)
}

// TooltipKeys is Tooltip with the keys the control answers to after the text,
// as keycaps (v0.2.0's ChordKeys in a tooltip): only the first of chords is
// drawn, and assistive technology reads it as part of the description. No
// chords (a shortcut the user unbound) is a plain Tooltip.
func TooltipKeys(c *ui.Context, anchor ui.Element, text string, chords []string) ui.Element {
	return controlsTooltipKeys(c, anchor, text, true, chords)
}

func controlsTooltipKeys(c *ui.Context, anchor ui.Element, text string, describe bool, chords []string) ui.Element {
	if text == "" {
		return anchor
	}
	var chord string
	if len(chords) > 0 {
		chord = chords[0]
	}
	var spoken []string
	if describe {
		spoken = append(spoken, text)
	}
	if chord != "" {
		spoken = append(spoken, ChordText(chord))
	}
	if len(spoken) > 0 {
		anchor.Description(strings.Join(spoken, ", "))
	}
	// Where the anchor was in the last frame places the arrow.
	at := anchor.Bounds()
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		tk := theme.Current()
		fill := theme.C("foreground")
		tip.MaxWidth(320).Padding(6, 12).Radius(tk.Radius["md"]).Background(fill).TextColor(theme.C("background"))
		tip.AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(6, 0, 6, 0)
		tip.Draw(func(p *ui.Painter, r ui.Rect) {
			const half, tall = 5, 5
			x := min(max(at.X+at.W/2, r.X+tk.Radius["md"]+half), r.X+r.W-tk.Radius["md"]-half)
			var path ui.Path
			if r.Y+r.H <= at.Y+at.H/2 { // above its anchor: the arrow points down
				y := r.Y + r.H
				path.MoveTo(x-half, y-0.5).LineTo(x+half, y-0.5).LineTo(x, y+tall).Close()
			} else {
				y := r.Y
				path.MoveTo(x-half, y+0.5).LineTo(x+half, y+0.5).LineTo(x, y-tall).Close()
			}
			p.FillPath(&path, fill)
		})
		tip.Children(func() {
			if chord == "" {
				controlsText(c, text, "meta").TextAlign(ui.Center)
				return
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				controlsText(c, text, "meta").TextAlign(ui.Center)
				ui.Row(c.Key("keys")).Gap(2).AlignItems(ui.Center).Role(ui.RoleNone).Children(func() {
					for j, part := range ChordParts(chord) {
						ui.Row(c.Key("cap:"+string(rune('0'+j)))).Height(20).MinWidth(20).PaddingX(4).Center().Role(ui.RoleNone).
							Radius(tk.Radius["sm"]).Border(1, theme.C("ink-faint")).Children(func() {
							controlsText(c, part, "meta").FontWeight(500).FontFeatures("tnum").Role(ui.RoleNone)
						})
					}
				})
			})
		})
	})
	return anchor
}
