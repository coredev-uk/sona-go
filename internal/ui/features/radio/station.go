package radio

import (
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The station page's geometry, in DIPs: the station's identity in a column of
// its own beside the page's lists once the content is stationSplit wide, over
// them below that. The lists take the rest of the width up to listsMax; past
// that the two columns stay together at the page's left edge. The identity's
// cover has two size classes, chosen by the content's width (identitySize), so
// Go serves exact boxes of a few sizes, not one per window width.
const (
	stationSplit = 800
	identityW    = 264 // the identity's column and cover
	identityWide = 320 // ... from identityWideAt of content
	// identityWideAt is the content width from which the identity takes the
	// larger class.
	identityWideAt = 1500
	identityGap    = 48
	listsMax       = 1100
	whenW          = 96 // the time column of the schedule
	railCol        = 28 // the timeline's rail
	slotArt        = 48
	slotPad        = 12
)

// identitySize is the width of the station identity's column, and its cover's
// edge, in a content area content DIPs wide: 264, 320 from 1500.
func identitySize(content float32) float32 {
	if content >= identityWideAt {
		return identityWide
	}
	return identityW
}

// stationPage is the station page's state: its scroll container, the identity's
// menu and the moment its schedule is read against.
type stationPage struct {
	kit.PageFrame
	menus *kit.ActionMenus
	clock refresh
}

// split is true when the identity stands beside the lists.
func (pg *stationPage) split() bool {
	if pg.Width <= 0 {
		return true // the first frame; the next one knows the width
	}
	return pg.Inner() >= stationSplit
}

// identityW is the width of the identity's column at the last frame's width.
func (pg *stationPage) identityW() float32 { return identitySize(pg.Inner()) }

// stationView is StationPage: what a station is and who runs it, play, and
// for a live station the schedule from the programme on air. A station Sona
// cannot play says why where the play control would be.
func (r *radio) stationView(c *ui.Context, rt *ui.Route) {
	pg := &r.stationPg
	id := rt.Param("id")
	kit.MenusIn(&pg.menus, r.env.Actions, nil)
	res := r.station(id)
	pg.Scroll(c, id, func(*kit.Pane, float32) {
		switch {
		case res.Err != nil && !res.HasData:
			kit.QueryError(c, r.env.Queries, i18n.T("Couldn't load this station"), res, kit.StationKey(id))
		case !res.HasData:
			stationSkeleton(c, pg.split(), pg.identityW())
		default:
			r.stationBody(c, res.Data)
		}
		pg.menus.Popup(c)
	})
}

// stationSkeleton is the page's frame while the station loads: the identity's
// cover and lines, and the first slots of the timeline. column is the
// identity column's width when it stands beside the lists.
func stationSkeleton(c *ui.Context, split bool, column float32) {
	tk := theme.Current()
	hover := theme.C("hover")
	bar := func(w, h float32, top float32) {
		kit.Bar(c, w, h).Margin(top, 0, 0, 0)
	}
	identity := func() {
		size := column
		if !split {
			size = 112
		}
		kit.Bar(c, size, size)
	}
	lines := func() {
		bar(200, 32, 0)
		bar(136, 14, 12)
		bar(96, 12, 12)
		ui.Box(c).Size(40, 40).Margin(20, 0, 0, 0).Radius(tk.Radius["full"]).Background(hover)
	}
	slots := func() {
		bar(80, 16, 0)
		for i := range 3 {
			ui.Row(c.Key(i)).FillWidth().Margin(12, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				kit.Bar(c, whenW+railCol, 14)
				ui.Box(c).FillWidth().Height(72).Radius(tk.Radius["md"]).Background(hover)
			})
		}
	}
	root := ui.Row(c.Key("loading")).FillWidth().Gap(identityGap).AlignItems(ui.Start).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	if !split {
		root = ui.Column(c.Key("loading")).FillWidth().Gap(32).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	}
	root.Children(func() {
		if split {
			ui.Column(c.Key("identity")).Width(column).Shrink(0).Children(func() {
				identity()
				ui.Column(c.Key("lines")).Margin(20, 0, 0, 0).Children(lines)
			})
		} else {
			ui.Row(c.Key("identity")).Gap(20).AlignItems(ui.End).Children(func() {
				identity()
				ui.Column(c.Key("lines")).Children(lines)
			})
		}
		ui.Column(c.Key("slots")).Grow(1).Basis(0).MinWidth(0).MaxWidth(listsMax).Children(slots)
	})
}

// link is a link of the page to another, only to a page that is registered;
// plain text where it is not. c carries the link's key.
func (r *radio) link(c *ui.Context, label, path string, ink ui.Color) {
	if r.env.Routed(path) {
		l := ui.Link(c, label, path).TextColor(ink)
		if l.Hovered() {
			l.Underline()
		}
		return
	}
	ui.Text(c, label).TextColor(ink)
}

// stationBody is the station: its identity, the note of a station Sona can't
// play, a broadcaster's "On air", a live station's schedule and a track
// station's first songs.
func (r *radio) stationBody(c *ui.Context, d services.StationDetail) {
	pg := &r.stationPg
	s := d.Station
	lists := func() {
		first := true
		if !s.Playable {
			r.unplayable(c, s)
		}
		if s.Kind == services.StationBroadcast && s.Playable {
			r.broadcaster(c, s, !first)
			first = false
		}
		if len(d.Schedule) > 0 && r.schedule(c, s, d.Schedule, !first) {
			first = false
		}
		if len(d.Upcoming) > 0 {
			r.upcoming(c, d.Upcoming, !first)
		}
	}
	if !pg.split() {
		ui.Column(c.Key("identity")).Margin(0, 0, 32, 0).Children(func() { r.identity(c, d, 0) })
		ui.Column(c.Key("lists")).Children(lists)
		return
	}
	column := pg.identityW()
	ui.Row(c.Key("page")).FillWidth().Gap(identityGap).AlignItems(ui.Start).Children(func() {
		// The identity stays in view while a long schedule scrolls.
		ui.Column(c.Key("identity")).Width(column).Shrink(0).Sticky().Top(16).Children(func() { r.identity(c, d, column) })
		ui.Column(c.Key("lists")).Grow(1).Basis(0).MinWidth(0).MaxWidth(listsMax).Children(lists)
	})
}

// identity is what the station is: its cover, name, who runs it, the
// catalogue line, notes and the play control with its menu. Beside the lists
// (column, the identity column's width) the cover leads a column; stacked
// (column 0) it is a small cover beside the lines.
func (r *radio) identity(c *ui.Context, d services.StationDetail, column float32) {
	pg := &r.stationPg
	tk := theme.Current()
	s := d.Station
	var byline func()
	switch {
	case d.Seed != nil:
		byline = func() { r.seedLine(c, *d.Seed) }
	case s.ShowID != "":
		path := "/curator/" + url.PathEscape(s.ShowID)
		byline = func() { r.link(c.Key(path), s.ShowName, path, theme.C("ink")) }
	case s.HostName != "":
		byline = func() { ui.Text(c, s.HostName).TextColor(theme.C("ink")) }
	case s.Tagline != "":
		byline = func() { ui.Text(c, s.Tagline).TextColor(theme.C("ink")) }
	}
	cover := func(size float32) {
		if d.Header.Motion != "" {
			kit.MotionCover(c, r.env.Motion, r.env.Art, s.Art.Large, d.Header.Motion, size, kit.MotionOpts{VideoSize: identityWide})
			return
		}
		kit.Artwork(c.Key("cover"), r.env.Art, s.Art.Large, size, kit.ArtOpts{Edge: true})
	}
	lines := func() {
		display := tk.Type["display"]
		ui.Text(c, s.Name).FontSize(display.Size).FixedLineHeight(display.LineHeight).FontWeight(int(display.Weight)).LetterSpacing(display.Tracking).
			TextColor(theme.C("ink")).MaxLines(3).Role(ui.RoleHeading).Level(1)
		if byline != nil {
			ui.Row(c.Key("byline")).Margin(8, 0, 0, 0).MinWidth(0).TextColor(theme.C("ink")).FontSize(tk.Type["heading"].Size).
				FontWeight(500).Children(byline)
		}
		theme.Text(c, stationLine(s), "meta").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
		if s.Notes != "" {
			theme.Text(c, s.Notes, "sub").TextColor(theme.C("ink-muted")).MaxLines(5).Margin(12, 0, 0, 0)
		}
		ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(func() {
			// Playing takes the listed songs; the page then lists the ones
			// after them.
			if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: s.Name, Disabled: !s.Playable}); play {
				r.playStation(s.ID, func(started bool) {
					if started {
						r.env.Queries.Invalidate(kit.StationKey(s.ID))
					}
				})
			}
			pg.menus.Trigger(c, kit.ActionTarget{Item: kit.StationItem(s), Name: s.Name}, kit.TriggerOpts{Header: true})
		})
	}
	if column > 0 {
		cover(column)
		ui.Column(c.Key("lines")).Margin(20, 0, 0, 0).Children(lines)
		return
	}
	ui.Row(c.Key("row")).FillWidth().Gap(20).AlignItems(ui.End).Children(func() {
		cover(112)
		ui.Column(c.Key("lines")).Grow(1).Basis(0).MinWidth(0).Children(lines)
	})
}

// unplayable is the dashed note of a station Sona cannot play, which says why
// where the play control would be.
func (r *radio) unplayable(c *ui.Context, s services.Station) {
	tk := theme.Current()
	ui.Row(c.Key("note")).MaxWidth(textMax).Margin(0, 0, 32, 0).Gap(12).AlignItems(ui.Start).Padding(14, 16).
		Radius(tk.Radius["lg"]).Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed).Role(ui.RoleGroup).Label(i18n.T("Sona can't play this station")).Children(func() {
		ui.Icon(c, theme.Icon("circle-slash")).FontSize(16).TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			theme.Text(c, i18n.T("Sona can't play this station"), "label").TextColor(theme.C("ink"))
			theme.Text(c, kit.StationUnavailableText(s.Unavailable), "sub").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
		})
	})
}

// section is a block of the page's lists: a group named by its heading, with
// a rule across the column above it when it is not the first.
func (r *radio) section(c *ui.Context, key, title string, ruled bool, body func()) {
	col := ui.Column(c.Key(key)).Role(ui.RoleGroup).Label(title)
	if ruled {
		col.Margin(40, 0, 0, 0).Padding(20, 0, 0, 0)
	}
	col.Children(func() {
		if ruled {
			ui.Box(c.Key("rule")).Absolute().Top(0).Left(0).Right(0).Height(1).Background(theme.C("line"))
		}
		theme.Text(c, title, "heading").TextColor(theme.C("ink")).Margin(0, 0, 12, 0).Role(ui.RoleHeading).Level(2)
		body()
	})
}

// broadcaster is a local broadcaster's "On air": it has no schedule, so the
// page (reached by a link; the station's tile plays it instead) says what it
// has on air while it plays, and what is known of it.
func (r *radio) broadcaster(c *ui.Context, s services.Station, ruled bool) {
	st := r.env.Play.State()
	tuned := st.Current != nil && st.Current.ID == s.ID
	r.section(c, "on-air", i18n.T("On air"), ruled, func() {
		switch {
		case tuned && st.OnAir != nil:
			theme.Text(c, kit.OnAirLine(*st.OnAir), "ui").TextColor(theme.C("ink"))
		case tuned:
			theme.Text(c, i18n.T("This station isn't naming what it is playing."), "sub").TextColor(theme.C("ink-muted"))
		default:
			theme.Text(c, i18n.T("Play the station to see what is on air."), "sub").TextColor(theme.C("ink-muted"))
		}
		if tuned && st.Broadcaster != nil {
			info := st.Broadcaster
			if meta := i18n.Meta(info.Genre, info.Location); meta != "" {
				theme.Text(c, meta, "sub").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
			}
			if info.Description != "" {
				theme.Text(c, info.Description, "sub").TextColor(theme.C("ink-muted")).Margin(8, 0, 0, 0)
			}
		}
	})
}

// upcoming is the songs a track station plays first, in order: the batch Go
// holds for it, so pressing Play starts with exactly these.
func (r *radio) upcoming(c *ui.Context, songs []services.Song, ruled bool) {
	tk := theme.Current()
	title := i18n.T("Plays first")
	r.section(c, "upcoming", title, ruled, func() {
		ui.Column(c.Key("songs")).Role(ui.RoleList).Label(title).Children(func() {
			for i, song := range songs {
				ui.Row(c.Key(i)).Height(tk.Metric["track-row"]).PaddingX(8).Gap(16).AlignItems(ui.Center).Radius(tk.Radius["md"]).
					Role(ui.RoleListItem).Children(func() {
					kit.Artwork(c.Key("art"), r.env.Art, song.Art.Small, tk.Metric["track-art"], kit.ArtOpts{})
					ui.Column(c.Key("names")).Grow(1).Basis(0).MinWidth(0).Children(func() {
						ink, muted := theme.C("ink"), theme.C("ink-muted")
						if song.AlbumID != "" {
							r.songLink(c, song.Name, "/album/"+url.PathEscape(song.AlbumID)+"?song="+url.QueryEscape(song.ID), "ui", ink)
						} else {
							theme.Text(c, song.Name, "ui").TextColor(ink).SingleLine()
						}
						if song.ArtistID != "" {
							r.songLink(c, song.Artist, "/artist/"+url.PathEscape(song.ArtistID), "sub", muted)
						} else {
							theme.Text(c, song.Artist, "sub").TextColor(muted).SingleLine()
						}
					})
					theme.Text(c, song.Album, "sub").TextColor(theme.C("ink-muted")).Grow(0.8).Basis(0).MinWidth(0).SingleLine().TextAlign(ui.Start)
					theme.Text(c, i18n.FormatDuration(time.Duration(song.DurationMs)*time.Millisecond), "meta").TextColor(theme.C("ink-muted")).Shrink(0)
				})
			}
		})
	})
}

// schedule is the live schedule as a vertical timeline: each programme that
// has not ended is a box on a rail, its start time beside it; the one on air
// has the rail inked up to how far through it is, with a marker there (amber
// only while Sona is playing this station). It reports whether it showed
// anything.
func (r *radio) schedule(c *ui.Context, s services.Station, events []services.StationEvent, ruled bool) bool {
	pg := &r.stationPg
	now := pg.clock.read(c, r.now)
	st := r.env.Play.State()
	playing := st.State == "playing" || st.State == "loading"
	tuned := st.Current != nil && st.Current.ID == s.ID && playing
	var rows []programme
	for _, p := range programmes(events) {
		if p.endMs > now {
			rows = append(rows, p)
		}
	}
	if len(rows) == 0 {
		return false
	}
	// Nothing to count down to when the station's own hours run to the end of
	// its schedule.
	_, counts := onAirSpan(s.Name, events, now)
	title := i18n.T("Schedule")
	r.section(c, "schedule", title, ruled, func() {
		ui.Column(c.Key("slots")).Role(ui.RoleList).Label(title).Children(func() {
			for i, p := range rows {
				r.slot(c, s, p, now, tuned, counts, i == 0, i == len(rows)-1)
			}
		})
	})
	return true
}

// slot is one programme of the timeline.
func (r *radio) slot(c *ui.Context, s services.Station, p programme, now int64, tuned, counts, first, last bool) {
	tk := theme.Current()
	cur := p.onAir(now)
	named := !same(p.event.Title, s.Name)
	row := ui.Row(c.Key(p.event.ID + "@" + time.UnixMilli(p.startMs).String())).FillWidth().AlignItems(ui.Start).Role(ui.RoleListItem)
	row.Children(func() {
		ui.Column(c.Key("when")).Width(whenW).Shrink(0).Padding(slotPad-1, 0, 0, 0).Children(func() {
			at := theme.Text(c, startLabel(p.event, now), "sub").TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine()
			if cur {
				at.TextColor(theme.C("ink")).FontWeight(600)
				theme.Text(c, i18n.T("On air"), "meta").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
			}
		})
		r.rail(c, p, now, tuned, cur, first, last)

		block := ui.Row(c.Key("block")).Grow(1).Basis(0).MinWidth(0).Margin(0, 0, 8, 0).Padding(slotPad, 14).Gap(14).
			AlignItems(ui.Start).Radius(tk.Radius["md"])
		switch {
		case cur:
			block.Background(theme.C("active"))
		case named:
			block.Background(theme.C("hover"))
		default:
			block.Border(1, theme.C("line"))
		}
		block.Children(func() {
			art := p.event.Art.Small
			if art == "" {
				art = s.Art.Small
			}
			kit.Artwork(c.Key("art"), r.env.Art, art, slotArt, kit.ArtOpts{Edge: true})
			ui.Column(c.Key("what")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				ink := theme.C("ink")
				if cur && tuned {
					ink = theme.C("play-ink")
				}
				theme.Text(c, p.event.Title, "label").TextColor(ink)
				if p.event.Description != "" {
					theme.Text(c, p.event.Description, "sub").TextColor(theme.C("ink-muted")).MaxLines(2).MaxWidth(textMax).Margin(2, 0, 0, 0)
				}
				if cur && counts {
					left := i18n.FormatRunningTime(time.Duration(max(60_000, p.endMs-now)) * time.Millisecond)
					theme.Text(c, i18n.T("{left} left", "left", left), "meta").TextColor(theme.C("ink-muted")).FontFeatures("tnum").Margin(6, 0, 0, 0)
				}
			})
		})
	})
}

// rail is the timeline's line beside a programme: a hairline with a dot at the
// programme's start; for the one on air the line is inked to the moment now
// falls at in the programme and a larger dot marks it, and the rail reads as
// the programme's progress.
func (r *radio) rail(c *ui.Context, p programme, now int64, tuned, cur, first, last bool) {
	e := ui.Box(c.Key("rail")).Width(railCol).Shrink(0).AlignSelf(ui.Stretch)
	line, faint, ground := theme.C("line"), theme.C("control-line"), theme.C("ground")
	mark := theme.C("ink")
	if tuned {
		mark = theme.C("play")
	}
	progress := progressOf(p.span, now)
	if cur {
		pct := float64(int(progress*100 + 0.5))
		e.Role(ui.RoleProgress).Label(i18n.T("Time through this programme")).Range(0, 100, pct).Value(i18n.T("{left} left", "left",
			i18n.FormatRunningTime(time.Duration(max(60_000, p.endMs-now))*time.Millisecond)))
	}
	e.Draw(func(pt *ui.Painter, rc ui.Rect) {
		x := float32(math.Round(float64(rc.X + rc.W/2)))
		top := rc.Y + slotPad + 10 // the dot: level with the first line of the title
		from, to := rc.Y, rc.Y+rc.H
		if first {
			from = top
		}
		if last {
			to = top
		}
		pt.Fill(ui.Rect{X: x, Y: from, W: 1, H: to - from}, line, 0)
		dot := func(y, d float32, col ui.Color) {
			pt.Fill(ui.Rect{X: x + 0.5 - d/2, Y: y - d/2, W: d, H: d}, col, d/2)
		}
		nowY := top
		if cur {
			// The rail is inked from the top of the programme to now.
			nowY = top + float32(progress)*(rc.H-8-(top-rc.Y))
			pt.Fill(ui.Rect{X: x - 0.5, Y: from, W: 2, H: nowY - from}, mark, 0)
		}
		// Where the programme began: a hollow dot.
		dot(top, 9, ground)
		dot(top, 7, faint)
		dot(top, 3, ground)
		if cur {
			dot(nowY, 13, ground)
			dot(nowY, 11, mark)
		}
	})
}

// songLink is a one-line link in a row's text, to a page that exists; plain
// text where it does not.
func (r *radio) songLink(c *ui.Context, label, path, style string, ink ui.Color) {
	t := theme.Current().Type[style]
	if r.env.Routed(path) {
		l := ui.Link(c.Key(path), label, path).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(ink).SingleLine()
		if l.Hovered() {
			l.Underline()
		}
		return
	}
	theme.Text(c, label, style).TextColor(ink).SingleLine()
}

// seedLine is what a track station was made from, as the header's byline:
// "Music like Song by Artist", the song, album and artist linking to their
// pages. The message's words and their order are the language's; its <0>…</0>
// marks the muted parts and the name and the artist are put in where it names
// them.
func (r *radio) seedLine(c *ui.Context, seed services.StationSeed) {
	const nameMark, byMark = "\ue000", "\ue001"
	var msg string
	switch {
	case seed.Subtitle != "" && seed.Only:
		msg = i18n.T("<0>Only songs by </0>{name}<1> by </1>{by}", "name", nameMark, "by", byMark)
	case seed.Subtitle != "":
		msg = i18n.T("<0>Music like </0>{name}<1> by </1>{by}", "name", nameMark, "by", byMark)
	case seed.Only:
		msg = i18n.T("<0>Only songs by </0>{name}", "name", nameMark)
	default:
		msg = i18n.T("<0>Music like </0>{name}", "name", nameMark)
	}
	page := "/artist/"
	switch seed.Kind {
	case services.SeedSong:
		page = "/song/"
	case services.SeedAlbum:
		page = "/album/"
	}
	ink, muted := theme.C("ink"), theme.C("ink-muted")
	ui.RichText(c).Children(func() {
		for i, part := range seedParts(msg) {
			key := c.Key(i)
			switch part.mark {
			case nameMark:
				r.link(key, seed.Name, page+url.PathEscape(seed.ID), ink)
			case byMark:
				if seed.ArtistID != "" {
					r.link(key, seed.Subtitle, "/artist/"+url.PathEscape(seed.ArtistID), ink)
				} else {
					ui.Text(key, seed.Subtitle).TextColor(ink)
				}
			default:
				colour := ink
				if part.muted {
					colour = muted
				}
				ui.Text(key, part.text).TextColor(colour)
			}
		}
	})
}

// part is a piece of a seed line's message: text (muted inside <N>…</N>), or
// the place of the name or of the artist.
type part struct {
	text  string
	muted bool
	mark  string
}

var seedTokens = regexp.MustCompile("</?[0-9]+>|[\ue000\ue001]")

// seedParts splits a seed line's message into its parts, in the message's
// order.
func seedParts(msg string) []part {
	var out []part
	muted := false
	last := 0
	add := func(to int) {
		if to > last {
			out = append(out, part{text: msg[last:to], muted: muted})
		}
	}
	for _, at := range seedTokens.FindAllStringIndex(msg, -1) {
		add(at[0])
		switch tok := msg[at[0]:at[1]]; {
		case tok == "\ue000" || tok == "\ue001":
			out = append(out, part{mark: tok})
		case strings.HasPrefix(tok, "</"):
			muted = false
		default:
			muted = true
		}
		last = at[1]
	}
	add(len(msg))
	return out
}
