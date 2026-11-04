package radio

import (
	"context"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The board's geometry (v0.2.0's OnAir.tsx), in DIPs: rows at least 420 wide
// in a grid that reaches 8 beyond its parent so the rows' own padding lines
// the text up, 16 across and 4 down; each row 96 high with the programme's
// 80 DIP art, its texts and a 32 DIP control.
const (
	boardMin   = 420
	boardGapX  = 16
	boardGapY  = 4
	boardBleed = 8
	rowHeight  = 96
	rowArt     = 80
	rowGap     = 16
	rowControl = 32
)

// board is the state of the on-air board: the width its columns follow, and
// the moment its schedules are read against.
type board struct {
	width widthOf
	clock refresh
}

// stationURL is a station's page.
func stationURL(id string) string { return "/station/" + url.PathEscape(id) }

// artOf is a row's art: the programme's own (Apple's schedule art), else the
// station's.
func artOf(s services.Station, programme services.StationEvent, onAir bool) string {
	if onAir && programme.Art.Medium != "" {
		return programme.Art.Medium
	}
	return s.Art.Medium
}

// onAirBoard is Apple Music's live stations as a now-and-next board (v0.2.0's
// OnAirBoard): per station, the programme on air with its art, its slot and
// how far through it is, and the next programme with a name of its own. The
// schedules are the stations' own pages (shared with them), read every 30
// seconds, never a ticking counter. ctx says what plays; menus is the page's,
// whose popup the page draws.
func (r *radio) onAirBoard(c *ui.Context, b *board, stations []services.Station, ctx kit.ItemsCtx, menus *kit.ActionMenus) {
	now := b.clock.read(c, r.now)
	root := ui.Grid(c.Key("board")).GapX(boardGapX).GapY(boardGapY).MarginX(-boardBleed).
		Role(ui.RoleList).Label(i18n.T("Live now"))
	width := b.width.track(c, root)
	if width > 0 {
		root.Columns(max(1, int((width+boardGapX)/(boardMin+boardGapX))))
	}
	root.Children(func() {
		if width <= 0 {
			ui.Box(c).Height(1) // the first frame measures; the next builds the rows
			return
		}
		for _, s := range stations {
			var detail *services.StationDetail
			if res := r.station(s.ID); res.HasData {
				d := res.Data
				detail = &d
			}
			r.onAirRow(c, s, detail, now, ctx, menus)
		}
	})
}

// onAirRow is one station of the board. The whole row is the station's link
// (or, for a station with no page, its play control): that button lies under
// the row's texts, which let the pointer through, and the row's own control
// (the play button, the equaliser, the menu) lies over it.
func (r *radio) onAirRow(c *ui.Context, s services.Station, detail *services.StationDetail, now int64, ctx kit.ItemsCtx, menus *kit.ActionMenus) {
	tk := theme.Current()
	tuned := ctx.CurrentStation != nil && ctx.CurrentStation(s)
	// A live station with no schedule has nothing to open: it plays. The
	// station's own page decides (its schedule), over the list's guess.
	page := s.HasPage
	var schedule []services.StationEvent
	if detail != nil {
		page, schedule = detail.Station.HasPage, detail.Schedule
	}
	programme, hasProgramme := onAir(schedule, now)
	what := s.Tagline
	artSrc := artOf(s, programme, hasProgramme)
	if hasProgramme {
		if n := programmeName(s.Name, s.Tagline, programme); n != "" {
			what = n
		}
	}
	// The programme's stretch (the station's own hours as one run), counted
	// down to the next programme with a name.
	run, hasRun := onAirSpan(s.Name, schedule, now)
	next, hasNext := nextProgramme(s.Name, schedule, now)
	reason := kit.StationUnavailableText(s.Unavailable)

	row := ui.Row(c.Key(s.ID)).Height(rowHeight).Padding(8).Gap(rowGap).Radius(tk.Radius["md"]).Role(ui.RoleListItem)
	hot := row.Hovered() || row.FocusWithin()
	if hot {
		row.Background(theme.C("hover"))
	}
	if !theme.ReducedMotion() {
		m := tk.Motion["feedback"]
		row.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	row.Children(func() {
		hit := ui.ButtonBase(c.Key("hit")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(tk.Radius["md"])
		if page {
			hit.Role(ui.RoleLink).Label(s.Name)
		} else {
			action := i18n.T("Play")
			if tuned && ctx.Playing {
				action = i18n.T("Pause")
			}
			// Named by its text, with the action after it.
			hit.Role(ui.RoleButton).Label(s.Name + " " + action)
		}
		if !s.Playable && !page {
			hit.Disabled(true)
		}

		kit.Artwork(c.Key("art"), r.env.Art, artSrc, rowArt, kit.ArtOpts{Edge: true}).IgnoreInput(true)
		ui.Column(c.Key("texts")).Grow(1).MinWidth(0).IgnoreInput(true).Children(func() {
			ink := theme.C("ink")
			nameInk := ink
			if tuned {
				nameInk = theme.C("play-ink")
			}
			text(c, s.Name, "label").TextColor(nameInk).SingleLine().TextAlign(ui.Start)
			text(c, what, "sub").TextColor(ink).SingleLine().TextAlign(ui.Start)
			if !s.Playable {
				ui.Row(c.Key("off")).Margin(8, 0, 0, 0).Gap(6).AlignItems(ui.Center).MinWidth(0).TextColor(theme.C("ink-muted")).Children(func() {
					ui.Icon(c, theme.Icon("circle-slash")).FontSize(14).Shrink(0)
					msg := reason
					if msg == "" {
						msg = i18n.T("Sona can't play this station")
					}
					text(c, msg, "meta").SingleLine().Shrink(1).MinWidth(0)
				})
				return
			}
			if hasRun {
				left := runningTime(max(60_000, run.endMs-now))
				fill := theme.C("ink-muted")
				if tuned && ctx.Playing {
					fill = theme.C("play")
				}
				ui.Row(c.Key("progress")).Margin(8, 0, 0, 0).Gap(12).AlignItems(ui.Center).TextColor(theme.C("ink-muted")).Children(func() {
					text(c, slotLabel(run, now), "meta").Shrink(0).SingleLine()
					progressTrack(c, "bar", i18n.T("Time through this programme"), i18n.T("{left} left", "left", left),
						progressOf(run, now), fill).Grow(1).MinWidth(32)
					text(c, i18n.T("{left} left", "left", left), "meta").Shrink(0).SingleLine()
				})
			}
			if hasNext {
				at, title := startLabel(next, now), next.Title
				line := i18n.T("Next at <0>{nextAt}</0>: {nextTitle}", "nextAt", at, "nextTitle", title)
				if hasRun && next.StartMs > run.endMs {
					// The station's own hours come between: the show isn't next.
					line = i18n.T("Later at <0>{nextAt}</0>: {nextTitle}", "nextAt", at, "nextTitle", title)
				}
				gap := float32(8)
				if hasRun {
					gap = 2
				}
				text(c, stripTags(line), "meta").TextColor(theme.C("ink-muted")).SingleLine().Margin(gap, 0, 0, 0)
			}
		})

		// The control sits over the row's button (a positioned box paints
		// over the box under it); the spacer keeps the texts clear of it.
		ui.Box(c.Key("control-room")).Width(rowControl).Height(rowControl).Shrink(0)
		ui.Row(c.Key("control")).Absolute().Top(8).Right(8).Width(rowControl).Height(rowControl).Center().Children(func() {
			switch {
			case tuned:
				kit.EqualiserBars(c, ctx.Playing, ctx.Motion)
			case !page:
				if menus != nil {
					menus.Trigger(c.Key("more"), kit.ActionTarget{Item: kit.StationItem(s), Name: s.Name}, kit.TriggerOpts{Visible: hot})
				}
			case s.Playable:
				play := kit.Button(c.Key("play"), "", kit.ButtonOpts{
					Variant: kit.Ghost, Size: kit.SizeSM, Icon: "play", Filled: true,
					A11yLabel: i18n.T("Play {name}", "name", s.Name),
				}).Radius(tk.Radius["full"])
				if play.Clicked() {
					// The equaliser replaces this button once the station
					// plays: focus stays in the row, on the station's link.
					hit.Focus()
					r.playStation(s.ID, nil)
				}
			}
		})

		if hit.Clicked() {
			switch {
			case page:
				r.env.Nav.Push(stationURL(s.ID))
			case tuned:
				r.toggle()
			default:
				r.playStation(s.ID, nil)
			}
		}
	})
}

// toggle pauses or resumes playback, off the main thread.
func (r *radio) toggle() {
	go func() {
		if err := r.pb.Toggle(context.Background()); err != nil && r.env.Log != nil {
			r.env.Log.Warn("radio: toggle playback", "err", err)
		}
	}()
}

// stripTags drops the <0> markers of a message's rich text: a span that
// only carries a font feature here.
func stripTags(s string) string { return tagMarkers.Replace(s) }

var tagMarkers = strings.NewReplacer("<0>", "", "</0>", "")

// onAirSkeleton is the board's frame while the live stations load: rows at
// their real height and art size.
func (r *radio) onAirSkeleton(c *ui.Context, w *widthOf) {
	hover := theme.C("hover")
	sm := theme.Current().Radius["sm"]
	root := ui.Grid(c.Key("board-skeleton")).GapX(boardGapX).GapY(boardGapY).MarginX(-boardBleed).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	width := w.track(c, root)
	if width > 0 {
		root.Columns(max(1, int((width+boardGapX)/(boardMin+boardGapX))))
	}
	root.Children(func() {
		if width <= 0 {
			ui.Box(c).Height(1)
			return
		}
		for i := range 6 {
			ui.Row(c.Key(i)).Height(rowHeight).Padding(8).Gap(rowGap).Children(func() {
				ui.Box(c).Size(rowArt, rowArt).Radius(sm).Background(hover).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Box(c).Width(128).Height(14).Margin(3, 0, 0, 0).Radius(sm).Background(hover)
					ui.Box(c).Width(192).Height(12).Margin(7, 0, 0, 0).Radius(sm).Background(hover)
					ui.Box(c).Grow(1)
					ui.Box(c).FillWidth().MaxWidth(288).Height(12).Margin(0, 0, 2, 0).Radius(sm).Background(hover)
				})
			})
		}
	})
}
