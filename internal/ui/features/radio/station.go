package radio

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// stationPage is the station page's state: its scroll container, the header's
// menu and the moment its schedule is read against.
type stationPage struct {
	frame
	// shown is the station the scroll offset belongs to.
	shown string
	menus *kit.ActionMenus
	clock refresh
}

// stationView is StationPage: what a station is and who runs it, play, and
// for a live station the schedule from the programme on air. A station Sona
// cannot play says why where the play control would be.
func (r *radio) stationView(c *ui.Context, rt *ui.Route) {
	pg := &r.stationPg
	id := rt.Param("id")
	if pg.shown != id {
		pg.pos = ui.ScrollState{} // another station: from its top
		pg.shown = id
	}
	if pg.menus == nil {
		pg.menus = kit.NewActionMenus(r.env.Actions, nil)
	}
	res := r.station(id)
	devstate.Scroll(c, &pg.pos)
	pg.scroll(c, func(gutter float32) {
		switch {
		case res.Err != nil && !res.HasData:
			kit.ErrorBlock(c, i18n.T("Couldn't load this station"), res.Err, kit.ErrorBlockOpts{
				OnRetry: func() { r.env.Queries.Invalidate(kit.StationKey(id)) }, Retrying: res.Fetching,
			})
		case !res.HasData:
			stationSkeleton(c)
		default:
			r.stationBody(c, res.Data, gutter)
		}
		pg.menus.Popup(c)
	})
}

// stationSkeleton is the header's frame while the station loads.
func stationSkeleton(c *ui.Context) {
	hover := theme.C("hover")
	sm := theme.Current().Radius["sm"]
	ui.Row(c.Key("loading")).FillWidth().AlignItems(ui.End).Gap(28).Padding(4, 0, 28, 0).Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Box(c).Size(200, 200).Radius(sm).Background(hover).Shrink(0)
		ui.Column(c).Grow(1).Padding(0, 0, 4, 0).Children(func() {
			ui.Box(c).Width(320).Height(32).Radius(sm).Background(hover)
			ui.Box(c).Width(192).Height(12).Margin(12, 0, 0, 0).Radius(sm).Background(hover)
			ui.Box(c).Size(40, 40).Margin(24, 0, 0, 0).Radius(theme.Current().Radius["full"]).Background(hover)
		})
	})
}

// A link of the page to another: only to a page that is registered.
func (r *radio) link(c *ui.Context, label, path string, ink ui.Color) {
	if r.env.HasRoute != nil && r.env.HasRoute(path) {
		l := ui.Link(c.Key(path), label, path).TextColor(ink)
		if l.Hovered() {
			l.Underline()
		}
		return
	}
	ui.Text(c.Key(path), label).TextColor(ink)
}

// stationBody is the station: its header, the note of a station Sona can't
// play, a broadcaster's "On air", a live station's schedule and a track
// station's first songs.
func (r *radio) stationBody(c *ui.Context, d services.StationDetail, gutter float32) {
	pg := &r.stationPg
	s := d.Station
	m := theme.Current().Metric["scrollbar"]
	var byline func()
	switch {
	case d.Seed != nil:
		byline = func() { r.seedLine(c, *d.Seed) }
	case s.ShowID != "":
		byline = func() { r.link(c, s.ShowName, "/curator/"+url.PathEscape(s.ShowID), theme.C("ink")) }
	case s.HostName != "":
		byline = func() { ui.Text(c, s.HostName).TextColor(theme.C("ink")) }
	case s.Tagline != "":
		byline = func() { ui.Text(c, s.Tagline).TextColor(theme.C("ink")) }
	}
	var colour string
	if len(d.Header.Colors) > 0 {
		colour = d.Header.Colors[0]
	}
	opts := kit.CollectionHeaderOpts{
		Title: s.Name, Art: s.Art.Large, Loader: r.env.Art,
		Byline: byline, Meta: stationLine(s), Description: s.Notes,
		Wash: &kit.BackdropOpts{Src: d.Header.Backdrop, Color: colour, Tone: kit.ToneGround},
		Actions: func() {
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
		},
	}
	if d.Header.Motion != "" {
		opts.Cover = func(c *ui.Context, size float32) {
			kit.MotionCover(c, r.env.Motion, r.env.Art, s.Art.Large, d.Header.Motion, size, kit.MotionOpts{VideoSize: 200})
		}
	}
	// The header bleeds to the content's edges: the page's right padding
	// holds the scroll bar's room too.
	kit.CollectionHeader(c, opts).Padding(32, gutter+m, 28, gutter).Margin(-padTop, -(gutter + m), 24, -gutter)

	if !s.Playable {
		r.unplayable(c, s)
	}
	if s.Kind == services.StationBroadcast && s.Playable {
		r.broadcaster(c, s)
	}
	if len(d.Schedule) > 0 {
		r.schedule(c, s.ID, d.Schedule)
	}
	if len(d.Upcoming) > 0 {
		r.upcoming(c, d.Upcoming)
	}
}

// unplayable is the dashed note of a station Sona cannot play, which says why
// where the play control would be.
func (r *radio) unplayable(c *ui.Context, s services.Station) {
	tk := theme.Current()
	ui.Row(c.Key("note")).MaxWidth(textMax).Margin(0, 0, 32, 0).Gap(12).AlignItems(ui.Start).Padding(14, 16).
		Radius(tk.Radius["lg"]).Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed).Role(ui.RoleGroup).Label(i18n.T("Sona can't play this station")).Children(func() {
		ui.Icon(c, theme.Icon("circle-slash")).FontSize(16).TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			text(c, i18n.T("Sona can't play this station"), "label").TextColor(theme.C("ink"))
			text(c, kit.StationUnavailableText(s.Unavailable), "sub").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
		})
	})
}

// heading is a section's heading on the page: 16/22, 8 above its content.
func heading(c *ui.Context, s string) {
	text(c, s, "heading").TextColor(theme.C("ink")).Margin(0, 0, 8, 0).Role(ui.RoleHeading).Level(2)
}

// broadcaster is a local broadcaster's "On air": it has no schedule, so the
// page (reached by a link; the station's tile plays it instead) says what it
// has on air while it plays, and what is known of it.
func (r *radio) broadcaster(c *ui.Context, s services.Station) {
	st := r.env.Play.State()
	tuned := st.Current != nil && st.Current.ID == s.ID
	title := i18n.T("On air")
	ui.Column(c.Key("on-air")).MaxWidth(textMax).Margin(0, 0, 32, 0).Role(ui.RoleGroup).Label(title).Children(func() {
		heading(c, title)
		switch {
		case tuned && st.OnAir != nil:
			text(c, kit.OnAirLine(*st.OnAir), "ui").TextColor(theme.C("ink"))
		case tuned:
			text(c, i18n.T("This station isn't naming what it is playing."), "sub").TextColor(theme.C("ink-muted"))
		default:
			text(c, i18n.T("Play the station to see what is on air."), "sub").TextColor(theme.C("ink-muted"))
		}
		if tuned && st.Broadcaster != nil {
			info := st.Broadcaster
			var parts []string
			for _, p := range []string{info.Genre, info.Location} {
				if p != "" {
					parts = append(parts, p)
				}
			}
			if len(parts) > 0 {
				text(c, strings.Join(parts, " · "), "sub").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
			}
			if info.Description != "" {
				text(c, info.Description, "sub").TextColor(theme.C("ink-muted")).Margin(8, 0, 0, 0)
			}
		}
	})
}

// listMax is the widest the schedule and the first songs get.
const listMax = 880

// upcoming is the songs a track station plays first, in order: the batch Go
// holds for it, so pressing Play starts with exactly these.
func (r *radio) upcoming(c *ui.Context, songs []services.Song) {
	tk := theme.Current()
	title := i18n.T("Plays first")
	ui.Column(c.Key("upcoming")).MaxWidth(listMax).Role(ui.RoleGroup).Label(title).Children(func() {
		heading(c, title)
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
							text(c, song.Name, "ui").TextColor(ink).SingleLine()
						}
						if song.ArtistID != "" {
							r.songLink(c, song.Artist, "/artist/"+url.PathEscape(song.ArtistID), "sub", muted)
						} else {
							text(c, song.Artist, "sub").TextColor(muted).SingleLine()
						}
					})
					text(c, song.Album, "sub").TextColor(theme.C("ink-muted")).Grow(0.8).Basis(0).MinWidth(0).SingleLine()
					text(c, i18n.FormatDuration(time.Duration(song.DurationMs)*time.Millisecond), "meta").TextColor(theme.C("ink-muted")).Shrink(0)
				})
			}
		})
	})
}

// songLink is a one-line link in a row's text, to a page that exists; plain
// text where it does not.
func (r *radio) songLink(c *ui.Context, label, path, style string, ink ui.Color) {
	t := theme.Current().Type[style]
	if r.env.HasRoute != nil && r.env.HasRoute(path) {
		l := ui.Link(c.Key(path), label, path).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(ink).SingleLine()
		if l.Hovered() {
			l.Underline()
		}
		return
	}
	text(c, label, style).TextColor(ink).SingleLine()
}

// schedule is the live schedule as a timetable: time on the left, the
// programme on the right; the one on air shows how far through its slot it is
// (amber only while Sona is playing this station).
func (r *radio) schedule(c *ui.Context, stationID string, events []services.StationEvent) {
	pg := &r.stationPg
	now := pg.clock.read(c, r.now)
	st := r.env.Play.State()
	playing := st.State == "playing" || st.State == "loading"
	tuned := st.Current != nil && st.Current.ID == stationID && playing
	on, hasOn := kit.OnAirProgramme(events, now)
	title := i18n.T("Schedule")
	ui.Column(c.Key("schedule")).MaxWidth(listMax).Role(ui.RoleGroup).Label(title).Children(func() {
		heading(c, title)
		ui.Column(c.Key("slots")).Role(ui.RoleList).Label(title).Children(func() {
			first := true
			for _, e := range events {
				if e.EndMs <= now {
					continue
				}
				isOn := hasOn && e.ID == on.ID && e.StartMs == on.StartMs
				row := ui.Row(c.Key(e.ID+"@"+time.UnixMilli(e.StartMs).String())).FillWidth().Gap(16).AlignItems(ui.Start).
					Padding(14, 0).Role(ui.RoleListItem)
				top := !first
				row.Children(func() {
					if top {
						ui.Box(c.Key("rule")).Absolute().Top(0).Left(0).Right(0).Height(1).Background(theme.C("line"))
					}
					ui.Column(c.Key("when")).Width(112).Shrink(0).Padding(2, 0, 0, 0).Children(func() {
						label := text(c, slotLabel(spanOf(e), now), "meta").TextColor(theme.C("ink-muted"))
						if isOn {
							label.TextColor(theme.C("ink")).FontWeight(500)
						}
						if isOn {
							text(c, i18n.T("On air"), "meta").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
						}
					})
					kit.Artwork(c.Key("art"), r.env.Art, e.Art.Small, 48, kit.ArtOpts{Edge: true})
					ui.Column(c.Key("what")).Grow(1).Basis(0).MinWidth(0).Children(func() {
						ink := theme.C("ink")
						if isOn && tuned {
							ink = theme.C("play-ink")
						}
						text(c, e.Title, "label").TextColor(ink)
						if e.Description != "" {
							text(c, e.Description, "sub").TextColor(theme.C("ink-muted")).MaxLines(2).MaxWidth(textMax).Margin(2, 0, 0, 0)
						}
						if isOn {
							fill := theme.C("ink-muted")
							if tuned {
								fill = theme.C("play")
							}
							progressTrack(c, "bar", i18n.T("Time through this programme"), "", progressOf(spanOf(e), now), fill).
								FillWidth().MaxWidth(320).Margin(10, 0, 0, 0)
						}
					})
				})
				first = false
			}
		})
	})
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
				r.inline(key, seed.Name, page+url.PathEscape(seed.ID), ink)
			case byMark:
				if seed.ArtistID != "" {
					r.inline(key, seed.Subtitle, "/artist/"+url.PathEscape(seed.ArtistID), ink)
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

// inline is a link inside a paragraph, to a page that exists; plain text
// where it does not.
func (r *radio) inline(c *ui.Context, label, path string, ink ui.Color) {
	if r.env.HasRoute != nil && r.env.HasRoute(path) {
		l := ui.Link(c, label, path).TextColor(ink)
		if l.Hovered() {
			l.Underline()
		}
		return
	}
	ui.Text(c, label).TextColor(ink)
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
