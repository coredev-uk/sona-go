package album

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// body is AlbumBody: the head, the sleeve, the songs and the notes in the
// layout of the content's width, then Apple's sections.
//
// The cover's colours (colours.go) are the page's: its accent takes amber's
// place on the whole page, and the sleeve is printed in the cover's own
// colour with its tokens redeclared.
func (a *album) body(c *ui.Context, m *model, lay layout, highlight string, now time.Time, auth services.AuthStatus) {
	k := a.colours(m)
	k.accentScope(func() {
		head := func() { a.head(c, m, k, now, account.HasAccount(auth.State)) }
		tracks := func() { a.songs(c, m, highlight, now, auth.Restricted) }
		notes := m.notes != "" && m.notes != m.lede
		if lay.wide {
			a.wide(c, m, k, lay, head, tracks, notes)
		} else {
			a.narrow(c, m, k, lay, head, tracks, notes)
		}
		if m.page != nil {
			ui.Column(c.Key("sections")).Padding(48, a.gr, padBottom, a.gl).FillWidth().Children(func() {
				a.sections(c, m, now, auth.Restricted)
			})
		}
	})
}

// notesShelf is the full notes, after the songs.
func (a *album) notesShelf(c *ui.Context, m *model, first bool) {
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Notes"), First: first}, func() {
		kit.Notes(c, m.notes, 0)
	})
}

// sleeveRoom is the width the wide sleeve gives its text and actions beyond
// the cover's, so the action row keeps to one line.
const sleeveRoom = 48

// wide is the gatefold: the sleeve a full-height column flush to the pane's
// top, left and bottom edges, holding the cover, the head, the disc index and
// the fine print; the listing, the facing page, on the ground beside it with
// the notes after the songs. The page's own gutter is the sleeve's inner
// margin, so the sleeve's text and the shelves below share one left edge. The
// sleeve's contents stay at the pane's top while the songs scroll when the pane
// shows all of them.
func (a *album) wide(c *ui.Context, m *model, k *colours, lay layout, head, tracks func(), notes bool) {
	grid := ui.Grid(c.Key("wide")).FillWidth().
		ColumnTracks(ui.Fixed(lay.cover+sleeveRoom+2*a.gl), ui.Fr(1)).
		RowTracks(ui.Fr(1)).AlignItems(ui.Stretch)
	if a.paneRect != nil {
		// The sleeve runs to the pane's bottom edge however short the
		// record; the shelves begin under the gatefold.
		grid.MinHeight(a.paneRect().H)
	}
	grid.Children(func() {
		a.sleeve(c, m, k, func() {
			in := ui.Column(c.Key("sleeve-in")).FillWidth().Padding(padTop, a.gl, padBottom, a.gl)
			in.Children(func() {
				a.cover(c, m, lay.cover)
				ui.Column(c.Key("head")).Margin(24, 0, 0, 0).MinWidth(0).Children(head)
				if len(m.discs) > 0 || len(m.facts) > 0 || m.rights != "" {
					ui.Box(c.Key("rule")).Margin(28, 0, 0, 0).FillWidth().Height(1).Background(theme.C("line"))
				}
				if len(m.discs) > 0 {
					ui.Column(c.Key("discs")).Margin(20, 0, 0, 0).Children(func() { a.discIndex(c, m) })
				}
				if len(m.facts) > 0 || m.rights != "" {
					ui.Column(c.Key("fine")).Margin(20, 0, 0, 0).Children(func() { a.finePrint(c, m) })
				}
			})
			a.stick(in)
		}).ColumnStart(1).RowStart(1)
		ui.Column(c.Key("listing")).ColumnStart(2).RowStart(1).AlignSelf(ui.Start).MinWidth(0).
			Padding(padTop, a.gr, padBottom, 40).Children(func() {
			ui.Column(c.Key("songs")).MinWidth(0).FillWidth().Children(tracks)
			if notes {
				ui.Column(c.Key("notes")).Margin(40, 0, 0, 0).MinWidth(0).Children(func() {
					a.notesShelf(c, m, true)
				})
			}
		})
	})
}

// stick holds the sleeve's contents at the pane's top while the songs scroll,
// within the sleeve's column, when the pane shows all of them (measured from
// the contents' height in the frame before).
func (a *album) stick(in ui.Element) {
	if a.paneRect == nil {
		return
	}
	if b := in.LayoutBounds(); b.H > 0 {
		a.sleeveH = b.H
	}
	if pane := a.paneRect(); pane.H == 0 || a.sleeveH > pane.H {
		return
	}
	in.Sticky().Top(0)
}

// narrow: the sleeve a full-bleed band across the pane's top, the cover
// beside the head; then the songs, the fine print and the notes on the ground
// (no disc index: the tracks carry their disc headings).
func (a *album) narrow(c *ui.Context, m *model, k *colours, lay layout, head, tracks func(), notes bool) {
	a.sleeve(c, m, k, func() {
		ui.Row(c).FillWidth().Padding(24, a.gr, 24, a.gl).Gap(20).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Shrink(0).Children(func() { a.cover(c, m, lay.cover) })
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(head)
		})
	}).FillWidth()
	ui.Column(c.Key("listing")).FillWidth().Padding(24, a.gr, 0, a.gl).Children(func() {
		ui.Column(c.Key("songs")).FillWidth().Children(tracks)
		if len(m.facts) > 0 || m.rights != "" {
			ui.Column(c.Key("fine")).Margin(24, 0, 0, 0).Children(func() { a.finePrint(c, m) })
		}
		if notes {
			ui.Column(c.Key("notes")).Children(func() { a.notesShelf(c, m, false) })
		}
	})
}

// sleeve is the page's one surface behind the cover and the record's fine
// print, flat and square: it is cut by the pane's edges, not a panel on the
// ground. It is printed in the cover's own background colour, one tone step
// off the ground, with its colour tokens redeclared so everything inside
// follows it (text 4.5:1 on it: theme/palette). A cover with no colour known
// (a library-only album) keeps the wash of Go's pre-blurred backdrop under the
// ground scrim, with a 4% ink layer to hold it one tone step off the ground.
func (a *album) sleeve(c *ui.Context, m *model, k *colours, content func()) ui.Element {
	if k != nil {
		e := ui.Box(c.Key("sleeve")).Background(k.surface)
		e.Children(func() {
			theme.Scope(k.sleeveTok, func() {
				ui.Column(c.Key("sleeve-body")).FillWidth().FillHeight().Children(content)
			})
		})
		return e
	}
	colour := ""
	if len(m.header.Colors) > 0 {
		colour = m.header.Colors[0]
	}
	return kit.Backdrop(c.Key("sleeve"), a.env.Art, kit.BackdropOpts{Src: m.header.Backdrop, Color: colour, Tone: kit.ToneGround}, func() {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(theme.C("ink").Alpha(0.04))
		ui.Column(c.Key("sleeve-body")).FillWidth().FillHeight().Children(content)
	})
}

// cover is the cover at its station: Apple's motion artwork where there is
// some and motion is allowed, else the still, decorative.
func (a *album) cover(c *ui.Context, m *model, size float32) {
	art := m.album.Art.Large
	if m.motion != "" {
		kit.MotionCover(c, a.env.Motion, a.env.Art, art, m.motion, size, kit.MotionOpts{})
		return
	}
	kit.Artwork(c, a.env.Art, art, size, kit.ArtOpts{Edge: true})
}

// head is AlbumHead: the title (wrapping as far as it needs, never moving
// the cover), the artist, the catalogue line, the badges, Apple's one-line
// note, the album's state, then its actions.
func (a *album) head(c *ui.Context, m *model, k *colours, now time.Time, account bool) {
	tk := theme.Current()
	release := m.upcoming || m.justOut
	ui.Column(c.Key("head-body")).MinWidth(0).Children(func() {
		theme.Text(c, m.album.Name, "display").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(1)
		ui.Row(c.Key("byline")).Margin(6, 0, 0, 0).MinWidth(0).TextColor(theme.C("ink")).
			FontSize(tk.Type["heading"].Size).FontWeight(500).Children(func() { a.byline(c, m, k) })
		theme.Text(c, m.meta, "meta").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
		// An album not out yet leads with its release and the actions that
		// answer it; Apple's badges and note follow.
		note := func(top float32) {
			if m.album.Explicit || len(kit.TraitLabels(m.album.Traits)) > 0 {
				ui.Row(c.Key("badges")).Margin(top, 0, 0, 0).Children(func() {
					kit.Badges(c, kit.BadgesOpts{Explicit: m.album.Explicit, Traits: m.album.Traits})
				})
			}
			if m.lede != "" {
				theme.Text(c, m.lede, "sub").TextColor(theme.C("ink-muted")).MaxWidth(500).Margin(12, 0, 0, 0)
			}
		}
		actions := func(top float32) {
			ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(top, 0, 0, 0).Children(func() {
				a.actions(c, m, account)
			})
		}
		if release {
			ui.Column(c.Key("status")).Role(ui.RoleGroup).Label(i18n.T("Release")).Margin(16, 0, 0, 0).Children(func() {
				a.releaseStatus(c, m, now, account)
			})
			actions(16)
			note(16)
			return
		}
		note(8)
		if status := a.status(c, m); status != nil {
			ui.Column(c.Key("status")).Margin(16, 0, 0, 0).Children(status)
		}
		actions(20)
	})
}

// byline is the artist: links from the catalogue page, else a link to the
// artist when the album knows one, else the name. Links go only where the
// artist page exists.
func (a *album) byline(c *ui.Context, m *model, k *colours) {
	ink := theme.C("ink")
	if k != nil && k.accent {
		// The artist is the record's link out, in the cover's accent.
		ink = theme.C("play-ink")
	}
	opts := kit.ArtistLinksOpts{HasRoute: a.env.HasRoute}
	if opts.HasRoute == nil {
		opts.HasRoute = func(string) bool { return false }
	}
	switch {
	case m.page != nil:
		kit.ArtistLinks(c, m.page.Artists, m.album.Artist, ink, opts)
	case m.album.ArtistID != "":
		kit.ArtistLinks(c, []services.Artist{{ID: m.album.ArtistID, Name: m.album.Artist}}, m.album.Artist, ink, opts)
	default:
		ui.Text(c, m.album.Artist).TextColor(ink).SingleLine()
	}
}

// status is how much of the record the library holds; nil when it holds it
// all (an album not out yet has its releaseStatus instead).
func (a *album) status(c *ui.Context, m *model) func() {
	if m.share == nil {
		return nil
	}
	return func() { a.libraryShare(c, m) }
}

// releaseStatus is an album not out yet's lead: its release as the page's
// biggest line after the title (the time left to the minute, or the year when
// Apple gives no day), under it the day in full with what the account has
// done about it ("Pre-added", "Reminder set"), and where the album has just
// come out, that it is, while the songs are fetched.
func (a *album) releaseStatus(c *ui.Context, m *model, now time.Time, account bool) {
	var lead, line string
	switch {
	case m.justOut:
		lead, line = i18n.T("Out now"), fullReleaseDate(m.album.ReleaseAt)
	case m.album.ReleaseAt == 0:
		// Apple gave only a year: no countdown to give.
		lead = i18n.T("Out {releaseDate}", "releaseDate", i18n.FormatISODate(m.album.ReleaseDate, i18n.DateLong))
		line = i18n.T("The day hasn't been announced yet")
	default:
		left := time.UnixMilli(m.album.ReleaseAt).Sub(now)
		lead, line = i18n.T("Out in {timeLeft}", "timeLeft", i18n.FormatTimeLeft(left)), fullReleaseDate(m.album.ReleaseAt)
	}
	parts := []string{line}
	switch {
	case m.justOut:
		parts = append(parts, i18n.T("Updating the songs"))
	default:
		if account && a.env.ItemStates.InLibrary(kit.StateAlbums, kit.StateOfAlbum(m.album)) {
			parts = append(parts, i18n.T("Pre-added"))
		}
		if kit.Reminded(a.env.Queries, a.env.Core, m.album) {
			parts = append(parts, i18n.T("Reminder set"))
		}
	}
	theme.Text(c, lead, "title").TextColor(theme.C("ink"))
	theme.Text(c, strings.Join(parts, " · "), "sub").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
}

// libraryShare is "3 of 13 songs in your library · Show complete album".
func (a *album) libraryShare(c *ui.Context, m *model) {
	sub := theme.Current().Type["sub"]
	ui.RichText(c).FontSize(sub.Size).TextColor(theme.C("ink")).Children(func() {
		ui.Text(c.Key("share"), libraryShareText(*m.share)+" · ")
		ui.Link(c.Key("complete"), i18n.T("Show complete album"), "/album/"+m.album.CatalogID).TextColor(theme.C("ink")).Underline()
	})
}

// actions are the head's buttons: Play (and Shuffle), the library button,
// ratings and the album's menu. An album not out yet plays the songs out
// now (the disc), offers Pre-add to a signed-in account and Remind me where
// the platform can; the first of Play, Pre-add and Remind me that applies
// leads, and only without a Play disc is it ink-filled.
func (a *album) actions(c *ui.Context, m *model, account bool) {
	al := m.album
	item := kit.StateOfAlbum(al)
	from := services.PlayFrom{Kind: "album", ID: al.ID}
	if m.upcoming {
		disc := m.songsOut > 0
		if disc {
			if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{PlayLabel: m.playOutLabel()}); play {
				a.env.Queue.PlaySongs(m.list, 0, from)
			}
		}
		if account {
			a.env.ItemStates.LibraryButtonWith(c, kit.StateAlbums, item, al.Name, kit.LibraryOpts{
				AddLabel: i18n.T("Pre-add"), DoneLabel: i18n.T("Pre-added"), Lead: !disc,
			})
		}
		kit.RemindButton(c, a.env.Queries, a.env.Core, al, !disc && !account)
	} else {
		play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: al.Name, Shuffle: true, Disabled: m.songsOut == 0})
		if play {
			a.env.Queue.PlaySongs(m.list, 0, from)
		}
		if shuffle {
			a.env.Queue.ShuffleSongs(m.list, from)
		}
		a.env.ItemStates.LibraryButton(c, kit.StateAlbums, item, al.Name, "")
	}
	a.env.ItemStates.RatingButtons(c, kit.StateAlbums, item, al.Name)
	a.menus.Trigger(c.Key("menu"), kit.ActionTarget{Item: kit.AlbumItem(al), Name: al.Name}, kit.TriggerOpts{Header: true})
}

// songs is the track list: the number column is the track, the Artist column
// only for compilations; a double-click or Enter plays the album from that
// row.
func (a *album) songs(c *ui.Context, m *model, highlight string, now time.Time, restricted bool) {
	list := m.list
	at := func(i int) *services.Song { return &list[i] }
	cur := a.env.Play.State()
	playing := cur.State == "playing"
	subline := m.subline(now, restricted)
	o := kit.TrackListOpts{
		Label:      i18n.T("Songs on {albumName}", "albumName", m.album.Name),
		Columns:    kit.TrackColumns{Artist: m.compilation},
		Discs:      !m.mixed,
		Highlight:  highlight,
		Popular:    m.popular,
		Subline:    func(s *services.Song) string { return subline(*s) },
		Container:  &kit.ActionContainer{Kind: "album", ID: m.album.ID},
		Restricted: restricted,
		// An album not out yet has no lengths to give: no row of dashes.
		BlankDuration: m.upcoming || m.justOut,
		Current:       cur.Current,
		Playing:       playing,
		Art:           a.env.Art,
		Menus:         a.trackMenus,
		Motion:        a.env.Motion,
		Pane:          &kit.Pane{Pos: &a.pos, Bounds: a.paneRect},
		OnPlay:        func(i int) { a.env.Queue.PlaySongs(list, i, services.PlayFrom{Kind: "album", ID: m.album.ID}) },
	}
	// The heading before a song: its group's on an album not out yet, else
	// (or after it) a classical album's work.
	group := m.groupHeading(now)
	var work func(s, prev *services.Song) string
	if m.classical {
		composers := m.composers
		o.Title = func(s *services.Song) string { return movementTitle(*s) }
		work = func(s, prev *services.Song) string { return workHeading(*s, prev, composers) }
	}
	if group != nil || work != nil {
		o.Heading = func(s, prev *services.Song) string {
			if s == nil {
				return ""
			}
			var g, w string
			if group != nil {
				g = group(s, prev)
			}
			if work != nil {
				// Under a group's heading the work starts over, even when it
				// continues the last group's: its name is said again beside
				// the group's.
				p := prev
				if g != "" {
					p = nil
				}
				w = work(s, p)
			}
			switch {
			case g != "" && w != "":
				return g + " · " + w
			case g != "":
				return g
			}
			return w
		}
	}
	if a.has("/artist/0") {
		o.OpenArtist = func(id string) { a.push("/artist/" + id) }
	}
	a.list = listView{opts: o, count: len(list), at: at}
	kit.TrackList(c, &a.tracks, len(list), at, o)
}

// discIndex is the multi-disc album's discs, each with its catalogue line
// ("Disc 2 · 14 songs, 58 min"). Choosing one moves to its first song in the
// list, keyboard focus included.
func (a *album) discIndex(c *ui.Context, m *model) {
	tk := theme.Current()
	ui.Column(c).Margin(0, -8, 0, -8).Gap(2).Role(ui.RoleList).Label(i18n.T("Discs")).Children(func() {
		for _, d := range m.discs {
			count := catalogueLine("", "", d.songs, d.durationMs)
			name := i18n.T("Disc {disc}", "disc", d.number)
			// A list holds items: the button is the item's only child.
			ui.Column(c.Key("item-" + strconv.Itoa(d.number))).FillWidth().Role(ui.RoleListItem).Children(func() {
				btn := ui.ButtonBase(c.Key("disc-" + strconv.Itoa(d.number))).Height(32).FillWidth().PaddingX(8).
					Radius(tk.Radius["md"]).Label(name + " · " + count)
				bg := ui.Transparent
				if btn.Hovered() {
					bg = theme.C("ink").Alpha(0.06)
				}
				btn.Background(bg)
				btn.Children(func() {
					ui.Row(c).MinWidth(0).AlignItems(ui.Center).Children(func() {
						theme.Text(c, name, "sub").FontWeight(500).TextColor(theme.C("ink")).SingleLine().Shrink(0)
						theme.Text(c, "\u00a0· "+count, "sub").TextColor(theme.C("ink-muted")).SingleLine().Shrink(1).MinWidth(0)
					})
				})
				if btn.Clicked() && a.list.at != nil {
					// The disc's heading sits 16 DIPs from the pane's top.
					a.tracks.ScrollToBlock(&kit.Pane{Pos: &a.pos, Bounds: a.paneRect}, &a.list.opts, a.list.count, a.list.at, d.first, 16)
				}
			})
		}
	})
}

// finePrint is the back of the sleeve: the record's facts in meta type, each
// a muted name over its entry, then the ℗ line, which names itself.
func (a *album) finePrint(c *ui.Context, m *model) {
	ui.Column(c).FillWidth().Children(func() {
		if len(m.facts) > 0 {
			ui.Column(c.Key("facts")).Gap(8).FillWidth().Children(func() {
				for _, f := range m.facts {
					ui.Column(c.Key(f.label)).FillWidth().Children(func() {
						theme.Text(c, f.label, "meta").TextColor(theme.C("ink-muted"))
						theme.Text(c, f.value, "meta").TextColor(theme.C("ink"))
					})
				}
			})
		}
		if m.rights != "" {
			e := theme.Text(c, m.rights, "meta").TextColor(theme.C("ink-muted"))
			if len(m.facts) > 0 {
				e.Margin(12, 0, 0, 0)
			}
		}
	})
}
