package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of the station list (v0.2.0's STATION_GRID), in DIPs: columns of
// 260 or more, 16 across and 4 down, 64 high rows, in a list that reaches 8
// beyond its parent on each side so the rows' own padding lines the text up.
const (
	stationMin    = 260
	stationGapX   = 16
	stationGapY   = 4
	stationRowH   = 64
	stationBleed  = 8
	stationArtDIP = 48
)

// StationListState is the state of a StationList: the width its columns
// follow.
type StationListState struct{ width itemsWidth }

// StationListOpts are the options of StationList.
type StationListOpts struct {
	// Label names the list when no heading labels it.
	Label string
	// OnPlay plays a station (the feature's player, or ItemPlayers).
	OnPlay func(services.Station)
}

// StationList is live stations as a dense list (v0.2.0's StationList): cover,
// name, what is on air (while the station plays and its stream names it) or
// its tagline or provider, and a play control; the one playing carries the
// equaliser. Radio's "Live now" and Apple's broadcaster sections share it.
//
// A station with a page (HasPage) links to it and has a play button. A station
// without one (a local broadcaster has no schedule) is itself the play control:
// its name is a button that plays it (and pauses it while it plays), and its
// menu is beside it. What plays is ctx.CurrentStation, ctx.Playing and
// ctx.OnAir; ctx.Toggle pauses or resumes.
func StationList(c *ui.Context, st *StationListState, stations []services.Station, ctx ItemsCtx, o StationListOpts) {
	root := ui.Grid(c.Key("kit.stations." + o.Label)).GapX(stationGapX).GapY(stationGapY).MarginX(-stationBleed).
		Role(ui.RoleList).Label(o.Label)
	width := st.width.track(c, root)
	if width > 0 {
		root.Columns(max(1, int((width+stationGapX)/(stationMin+stationGapX))))
	}
	root.Children(func() {
		if width <= 0 {
			ui.Box(c).Height(1) // the first frame measures; the next builds the rows
			return
		}
		for i := range stations {
			stationRow(c, stations[i], ctx, o)
		}
		if ctx.Menus != nil && !ctx.NoPopup {
			ctx.Menus.Popup(c)
		}
	})
}

// stationRow is one station of the list.
func stationRow(c *ui.Context, s services.Station, ctx ItemsCtx, o StationListOpts) {
	tk := theme.Current()
	on := ctx.CurrentStation != nil && ctx.CurrentStation(s)
	row := ui.Row(c.Key(s.ID)).Height(stationRowH).Gap(12).PaddingX(8).Radius(tk.Radius["md"]).Role(ui.RoleListItem)
	hot := row.Hovered() || row.FocusWithin()
	if hot {
		row.Background(theme.C("hover"))
	}
	if !theme.ReducedMotion() {
		m := tk.Motion["feedback"]
		row.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	unavailable := ""
	if !s.Playable {
		unavailable = StationUnavailableText(s.Unavailable)
	}
	line := s.Tagline
	if line == "" {
		line = s.Provider
	}
	if on && ctx.OnAir != nil {
		line = OnAirLine(*ctx.OnAir)
	}

	row.Children(func() {
		// The hit area: the cover and the texts. A station with a page links
		// to it; one without is the play control itself.
		hit := ui.ButtonBase(c.Key("hit")).Grow(1).MinWidth(0).FillHeight().Gap(12).AlignItems(ui.Center).Justify(ui.Start).
			Radius(tk.Radius["sm"])
		if s.HasPage {
			hit.Role(ui.RoleLink).Label(s.Name)
		} else {
			action := i18n.T("Play")
			if on && ctx.Playing {
				action = i18n.T("Pause")
			}
			// Named by its text, with the action after it.
			hit.Role(ui.RoleButton).Label(s.Name + " " + action)
			if !s.Playable {
				// A disabled element takes no pointer, so the row says why.
				hit.Disabled(true)
				row.Tooltip(unavailable)
			}
		}
		hit.Children(func() {
			itemsArtwork(c, ctx.Art, s.Art.Small, stationArtDIP, ArtOpts{Edge: true})
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				label, sub := tk.Type["label"], tk.Type["sub"]
				ink := theme.C("ink")
				if on {
					ink = theme.C("play-ink")
				}
				ui.Text(c, s.Name).FontSize(label.Size).FixedLineHeight(label.LineHeight).FontWeight(int(label.Weight)).
					TextColor(ink).SingleLine().TextAlign(ui.Start)
				ui.Text(c, line).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).TextColor(theme.C("ink-muted")).
					SingleLine().TextAlign(ui.Start)
			})
		})

		if s.HasPage {
			if on {
				ui.Row(c.Key("eq")).Margin(0, 10, 0, 0).Children(func() { EqualiserBars(c, ctx.Playing, ctx.Motion) })
			} else {
				// Revealed on hover or focus; the equaliser replaces it once
				// the station plays, so focus stays in the row, on the link.
				// A disabled button takes no pointer, so the reason is the tip
				// of the box around it.
				box := ui.Row(c.Key("play")).Shrink(0)
				if !s.Playable {
					box.Tooltip(unavailable)
				}
				box.Children(func() {
					play := Button(c, "", ButtonOpts{
						Variant: Ghost, Size: SizeSM, Icon: "play", Disabled: !s.Playable,
						A11yLabel: i18n.T("Play {name}", "name", s.Name),
					}).Radius(tk.Radius["full"])
					if !hot && !play.Focused() {
						play.Opacity(0)
					}
					if play.Clicked() && o.OnPlay != nil {
						hit.Focus()
						o.OnPlay(s)
					}
				})
			}
		} else {
			switch {
			case on:
				ui.Row(c.Key("eq")).Margin(0, 4, 0, 0).Children(func() { EqualiserBars(c, ctx.Playing, ctx.Motion) })
			case s.Playable:
				mark := ui.Row(c.Key("mark")).Margin(0, 4, 0, 0).Shrink(0).TextColor(theme.C("ink-muted")).IgnoreInput(true)
				if !hot {
					mark.Opacity(0)
				}
				mark.Children(func() { theme.Glyph(c, "play", 16, theme.GlyphOpts{Color: theme.C("ink-muted"), Inherit: true}) })
			}
			if ctx.Menus != nil {
				ctx.Menus.Trigger(c.Key("more"), ActionTarget{Item: StationItem(s), Name: s.Name}, TriggerOpts{Visible: hot})
			}
		}

		if hit.Clicked() {
			switch {
			case s.HasPage:
				ctx.nav(itemsPage("/station/", s.ID))
			case on:
				if ctx.Toggle != nil {
					ctx.Toggle()
				}
			case o.OnPlay != nil:
				o.OnPlay(s)
			}
		}
	})
}

// itemsArtwork is Artwork that tolerates a missing loader: a plain --hover
// box of the same size.
func itemsArtwork(c *ui.Context, l *ArtLoader, src string, size float32, o ArtOpts) ui.Element {
	if l == nil {
		return ui.Box(c).Size(size, size).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover"))
	}
	return Artwork(c, l, src, size, o)
}
