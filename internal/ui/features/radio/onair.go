package radio

import (
	"context"
	"math"
	"net/url"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The board's geometry, in DIPs. A row is the station column (art, name,
// control) and a track that follows it; the board reaches boardBleed past its
// parent on the left so the art lines up with the page's edge, and so does
// the rail the bands below hang their headings in (railW).
const (
	whoW       = 232 // the station column, with its left padding
	whoGap     = 12  // between it and the track
	boardBleed = 8
	rowH       = 72 // a lane: the art 56 with 8 above and below
	rowArt     = 56
	laneH      = 56 // a programme's box, as tall as the art
	rowControl = 32
	rulerH     = 44 // the time ruler: the now flag, the hours' labels and ticks
	flagH      = 18
	// railW is where the content of a band starts: the track's left edge.
	railW = whoW - boardBleed + whoGap
)

// board is the state of the on-air board: the width its track follows, and the
// moment its schedules are read against.
type board struct {
	width widthOf
	clock refresh
}

// stationURL is a station's page.
func stationURL(id string) string { return "/station/" + url.PathEscape(id) }

// onAirBoard is Apple Music's live stations as a programme guide: a time
// ruler across the top, one lane per station with its programmes placed by
// their real start and end, and one line down all of them at now. The
// schedules are the stations' own pages (shared with them), read every 30
// seconds, never a ticking counter. ctx says what plays; menus is the page's,
// whose popup the page draws.
func (r *radio) onAirBoard(c *ui.Context, b *board, stations []services.Station, ctx kit.ItemsCtx, menus *kit.ActionMenus) {
	now := b.clock.read(c, r.now)
	root := ui.Column(c.Key("board")).FillWidth().Margin(0, 0, 0, -boardBleed)
	width := b.width.track(c, root)
	root.Children(func() {
		trackW := width - whoW - whoGap
		if trackW < 120 {
			// The first frame measures; the next builds the lanes.
			r.ruler(c, guide{}, i18n.T("Live now"))
			return
		}
		g := newGuide(now, trackW)
		r.ruler(c, g, i18n.T("Live now"))
		ui.Column(c.Key("lanes")).FillWidth().Role(ui.RoleList).Label(i18n.T("Live now")).Children(func() {
			for _, s := range stations {
				var detail *services.StationDetail
				if res := r.station(s.ID); res.HasData {
					d := res.Data
					detail = &d
				}
				r.lane(c, s, detail, g, ctx, menus)
			}
		})
		// The one line down every lane, over the programmes. It has no input.
		ui.Box(c.Key("now")).Absolute().Left(whoW + whoGap + g.nowX - 1).Top(flagH).Bottom(0).Width(2).
			Background(theme.C("ink")).IgnoreInput(true)
	})
}

// ruler is the top of the board: the heading in the station column and, over
// the track, the flag at now, the clock time of every half hour and the
// ticks.
func (r *radio) ruler(c *ui.Context, g guide, title string) {
	ink := theme.C("ink")
	ui.Row(c.Key("ruler")).FillWidth().Height(rulerH).Gap(whoGap).Children(func() {
		ui.Row(c.Key("title")).Width(whoW).Shrink(0).Padding(0, 0, 4, boardBleed).AlignItems(ui.End).Children(func() {
			theme.Text(c, title, "page").TextColor(ink).SingleLine().Role(ui.RoleHeading).Level(2)
		})
		scale := ui.Box(c.Key("scale")).Grow(1).MinWidth(0).Height(rulerH)
		if g.trackW <= 0 {
			return
		}
		faint, muted := theme.C("ink-faint"), theme.C("ink-muted")
		line := theme.C("control-line")
		scale.Draw(func(p *ui.Painter, rc ui.Rect) {
			for _, t := range g.ticks {
				h := float32(4)
				if t.hour {
					h = 8
				}
				x := float32(math.Round(float64(rc.X + g.win.x(t.ms, rc.W))))
				p.Fill(ui.Rect{X: x, Y: rc.Y + rc.H - h, W: 1, H: h}, line, 0)
			}
		})
		scale.Children(func() {
			for _, t := range g.ticks {
				x := g.win.x(t.ms, g.trackW)
				// A label gives way to the flag, and stays off the track's edges.
				if x < 40 || x > g.trackW-40 || math.Abs(float64(x-g.nowX)) < 52 {
					continue
				}
				col := faint
				if t.hour {
					col = muted
				}
				theme.Text(c.Key(t.ms), clockAt(t.ms), "meta").TextColor(col).FontFeatures("tnum").SingleLine().
					Absolute().Left(x - 40).Width(80).Top(flagH + 4).TextAlign(ui.Center)
			}
			theme.Text(c.Key("flag"), clockAt(g.now), "meta").FontWeight(600).FontFeatures("tnum").SingleLine().
				TextColor(theme.C("ground")).Background(ink).Radius(theme.Current().Radius["sm"]).Padding(1, 6).
				Absolute().Left(g.nowX - 1).Top(0)
		})
	})
}

// lane is one station of the board. The station column is the station's link
// (or, for a station with no page, the whole lane is its play button): that
// button lies under the column's texts, which let the pointer through, and
// the column's own control (the play button, the equaliser, the menu) lies
// over it. A programme's box links to the station's page too, but is no Tab
// stop of its own.
func (r *radio) lane(c *ui.Context, s services.Station, detail *services.StationDetail, g guide, ctx kit.ItemsCtx, menus *kit.ActionMenus) {
	tk := theme.Current()
	tuned := ctx.Current != nil && ctx.Current.ID == s.ID
	playing := tuned && ctx.Playing
	// A live station with no schedule has nothing to open: it plays. The
	// station's own page decides (its schedule), over the list's guess.
	page := s.HasPage
	var schedule []services.StationEvent
	if detail != nil {
		page, schedule = detail.Station.HasPage, detail.Schedule
	}
	cells := cellsFor(s, schedule, g)
	// The programmes' boxes are links to the page; a lane that only says
	// something (the tagline, why Sona can't play) has none, so the lane's
	// button runs under the whole row.
	linked := page && len(cells) > 0 && !cells[0].info
	whole := !linked
	row := ui.Row(c.Key(s.ID)).FillWidth().Height(rowH).Gap(whoGap).Radius(tk.Radius["md"]).Role(ui.RoleListItem)
	row.Children(func() {
		hit := ui.ButtonBase(c.Key("hit")).Absolute().Top(0).Left(0).Bottom(0).Radius(tk.Radius["md"])
		if whole {
			hit.Right(0)
		} else {
			hit.Width(whoW)
		}
		if page {
			hit.Role(ui.RoleLink).Label(s.Name)
		} else {
			action := i18n.T("Play")
			if tuned && ctx.Playing {
				action = i18n.T("Pause")
			}
			// Named by its text, with the action after it.
			hit.Role(ui.RoleButton).Label(s.Name + " " + action)
			if !s.Playable {
				hit.Disabled(true)
			}
		}
		hot := hit.Hovered() || row.FocusWithin()
		if hot && whole {
			row.Background(theme.C("hover"))
		}
		theme.Fade(row, "feedback")

		who := ui.Row(c.Key("who")).Width(whoW).Shrink(0).Padding(8, 8, 8, boardBleed).Gap(12).AlignItems(ui.Center).Radius(tk.Radius["md"])
		if hot && !whole {
			who.Background(theme.C("hover"))
		}
		theme.Fade(who, "feedback")
		who.Children(func() {
			kit.Artwork(c.Key("art"), r.env.Art, s.Art.Medium, rowArt, kit.ArtOpts{Edge: true}).IgnoreInput(true)
			nameInk := theme.C("ink")
			if tuned {
				nameInk = theme.C("play-ink")
			}
			ui.Column(c.Key("name")).Grow(1).MinWidth(0).IgnoreInput(true).Children(func() {
				theme.Text(c, s.Name, "label").TextColor(nameInk).MaxLines(2).TextAlign(ui.Start)
			})
			// The spacer keeps the name clear of the control.
			ui.Box(c.Key("control-room")).Width(rowControl).Height(rowControl).Shrink(0)
		})
		// The control sits over the column's button: a positioned box built
		// after it paints over it.
		ui.Row(c.Key("control")).Absolute().Top((rowH - rowControl) / 2).Left(whoW - 8 - rowControl).Width(rowControl).Height(rowControl).Center().Children(func() {
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

		var link func()
		if linked {
			link = func() { r.env.Nav.Push(stationURL(s.ID)) }
		}
		track := ui.Box(c.Key("track")).Grow(1).MinWidth(0).Height(rowH)
		track.Draw(hourLines(g))
		track.Children(func() {
			for _, cl := range cells {
				r.cell(c, cl, link)
			}
			// The progress of the programme on air reads outside its link,
			// as a bar along the foot of its box up to the now line.
			for _, cl := range cells {
				if p := cl.progress; p != nil {
					fill := theme.C("ink-muted")
					if playing {
						fill = theme.C("play")
					}
					r.progress(c, cl, p, fill)
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
		if !page && menus != nil {
			// The more button is not a Tab stop: Shift+F10 or the Menu key on
			// the row's button open the same menu, as on every list row.
			menus.Row(hit, kit.ActionTarget{Item: kit.StationItem(s), Name: s.Name})
		}
	})
}

// progress is the programme on air's bar: a 2 DIP line along the foot of its
// box from the box's left (past the rounded corner) to the now line, a
// progress indicator for assistive technology that reads how far through the
// whole programme it is and what is left.
func (r *radio) progress(c *ui.Context, cl cell, p *cellProgress, fill ui.Color) {
	x0 := cl.x0 + 1 + 3
	w := cl.x0 + 1 + (cl.x1-cl.x0-2)*float32(min(max(p.drawn, 0), 1)) - x0
	e := ui.Box(c.Key("bar")).Absolute().Left(x0).Top((rowH+laneH)/2 - 2).Width(max(w, 0)).Height(2).Background(fill).
		Role(ui.RoleProgress).Label(i18n.T("Time through this programme")).IgnoreInput(true)
	e.Range(0, 100, float64(int(p.value*100+0.5)))
	e.Value(p.text)
}

// hourLines paints a hairline at every hour behind a track's boxes.
func hourLines(g guide) func(*ui.Painter, ui.Rect) {
	line := theme.C("line")
	return func(p *ui.Painter, rc ui.Rect) {
		for _, t := range g.ticks {
			if t.hour {
				x := float32(math.Round(float64(rc.X + g.win.x(t.ms, rc.W))))
				p.Fill(ui.Rect{X: x, Y: rc.Y, W: 1, H: rc.H}, line, 0)
			}
		}
	}
}

// cell draws one box of a lane at its place on the track. link, when not nil,
// makes it a link (named for its programme and slot) that is not a Tab stop:
// the station column's link is the one keyboard users reach.
func (r *radio) cell(c *ui.Context, cl cell, link func()) {
	tk := theme.Current()
	var el ui.Element
	if link != nil {
		el = ui.ButtonBase(c.Key(cl.key)).Role(ui.RoleLink).Label(cl.name).TabStop(false)
	} else {
		el = ui.Row(c.Key(cl.key))
	}
	el.Absolute().Left(cl.x0 + 1).Top((rowH - laneH) / 2).Width(cl.x1 - cl.x0 - 2).Height(laneH).
		Radius(tk.Radius["md"]).Clip().Justify(ui.Start).AlignItems(ui.Center)
	hover := link != nil && el.Hovered()
	titleStyle, titleInk, weight := "label", theme.C("ink"), 0
	switch cl.look {
	case lookCurrent:
		el.Background(theme.C("active"))
		weight = 600
	case lookNamed:
		bg := theme.C("hover")
		if hover {
			bg = theme.C("active")
		}
		el.Background(bg)
	default:
		el.Border(1, theme.C("line"))
		if hover {
			el.Background(theme.C("hover"))
		}
		titleStyle, titleInk = "sub", theme.C("ink-muted")
	}
	theme.Fade(el, "feedback")
	muted := theme.C("ink-muted")
	el.Children(func() {
		if cl.from != "" {
			ui.Row(c.Key("from")).Absolute().Left(8).Top(0).Bottom(0).Gap(2).AlignItems(ui.Center).TextColor(muted).IgnoreInput(true).Children(func() {
				theme.Glyph(c, "chevron-left", 12, theme.GlyphOpts{Color: muted, Inherit: true})
				theme.Text(c, cl.from, "meta").FontFeatures("tnum").SingleLine()
			})
		}
		if cl.text.w >= 72 && (cl.title != "" || cl.off) {
			pad := cl.text.x0 - (cl.x0 + 1)
			ui.Column(c.Key("text")).Margin(0, 0, 0, pad).Width(cl.text.w).Shrink(0).MinWidth(0).IgnoreInput(true).Children(func() {
				switch {
				case cl.off:
					ui.Row(c.Key("off")).Gap(6).AlignItems(ui.Center).MinWidth(0).TextColor(muted).Children(func() {
						ui.Icon(c, theme.Icon("circle-slash")).FontSize(14).Shrink(0)
						theme.Text(c, cl.title, "meta").MaxLines(2).Shrink(1).MinWidth(0)
					})
				default:
					title := theme.Text(c, cl.title, titleStyle).TextColor(titleInk).SingleLine()
					if weight > 0 {
						title.FontWeight(weight)
					}
					if cl.sub != "" || cl.left != "" {
						ui.Row(c.Key("sub")).Gap(4).MinWidth(0).TextColor(muted).Children(func() {
							if cl.left != "" {
								theme.Text(c, cl.left, "meta").FontFeatures("tnum").SingleLine().Shrink(0)
								if cl.sub != "" {
									theme.Text(c, "·", "meta").Role(ui.RoleNone).Shrink(0)
								}
							}
							if cl.sub != "" {
								theme.Text(c, cl.sub, "meta").FontFeatures("tnum").SingleLine().Shrink(1).MinWidth(0)
							}
						})
					}
				}
			})
		}
		if cl.more {
			ui.Row(c.Key("more")).Absolute().Right(4).Top(0).Bottom(0).AlignItems(ui.Center).IgnoreInput(true).Children(func() {
				theme.Glyph(c, "chevron-right", 12, theme.GlyphOpts{Color: theme.C("ink-faint"), Inherit: true})
			})
		}
	})
	if link != nil && el.Clicked() {
		link()
	}
}

// toggle pauses or resumes playback, off the main thread.
func (r *radio) toggle() {
	go func() {
		if err := r.pb.Toggle(context.Background()); err != nil && r.env.Log != nil {
			r.env.Log.Warn("radio: toggle playback", "err", err)
		}
	}()
}

// onAirSkeleton is the board's frame while the live stations load: the
// ruler's heading and lanes at their real height, the art and the programmes
// as bars.
func (r *radio) onAirSkeleton(c *ui.Context, w *widthOf) {
	hover := theme.C("hover")
	tk := theme.Current()
	root := ui.Column(c.Key("board-skeleton")).FillWidth().Margin(0, 0, 0, -boardBleed).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	width := w.track(c, root)
	root.Children(func() {
		r.ruler(c, guide{}, i18n.T("Live now"))
		trackW := width - whoW - whoGap
		if trackW < 120 {
			return
		}
		// Programme bars as shares of the track, two to a lane.
		shares := [][2]float32{{0.46, 0.34}, {0.62, 0.28}, {0.3, 0.5}, {0.52, 0.3}, {0.4, 0.4}, {0.7, 0.2}}
		for i, sh := range shares {
			ui.Row(c.Key(i)).FillWidth().Height(rowH).Gap(whoGap).Children(func() {
				ui.Row(c.Key("who")).Width(whoW).Shrink(0).Padding(8, 8, 8, boardBleed).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(rowArt, rowArt).Radius(tk.Radius["sm"]).Background(hover).Shrink(0)
					ui.Box(c).Width(96).Height(14).Radius(tk.Radius["sm"]).Background(hover)
				})
				ui.Row(c.Key("track")).Grow(1).MinWidth(0).Height(rowH).AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Box(c.Key(0)).Width(trackW * sh[0]).Height(laneH).Radius(tk.Radius["md"]).Background(hover)
					ui.Box(c.Key(1)).Width(trackW * sh[1]).Height(laneH).Radius(tk.Radius["md"]).Background(hover)
				})
			})
		}
	})
}
