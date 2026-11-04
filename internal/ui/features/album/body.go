package album

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// signedIn: the account states in which the library and personal endpoints
// are usable (v0.2.0's hasAccount).
func signedIn(state string) bool { return state == "signed_in" || state == "validating" }

// body is AlbumBody: the head, the sleeve, the songs and the notes in the
// layout of the content's width, then Apple's sections.
func (a *album) body(c *ui.Context, m *model, lay layout, highlight string, now time.Time, auth services.AuthStatus) {
	head := func() { a.head(c, m, now, signedIn(auth.State)) }
	tracks := func() { a.songs(c, m, highlight, now, auth.Restricted) }
	notes := m.notes != "" && m.notes != m.lede
	if lay.wide {
		a.wide(c, m, lay, head, tracks, notes)
	} else {
		a.narrow(c, m, lay, head, tracks, notes)
	}
	if m.page != nil {
		ui.Column(c.Key("sections")).Margin(48, 0, 0, 0).FillWidth().Children(func() {
			a.sections(c, m, now, auth.Restricted)
		})
	}
}

// notesShelf is the full notes, after the songs.
func (a *album) notesShelf(c *ui.Context, m *model, first bool) {
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Notes"), First: first}, func() {
		kit.Notes(c, m.notes, 0)
	})
}

// wide: the sleeve in the left column (cover, disc index, fine print), the
// head, the songs and the notes in the right; reading order head, sleeve,
// songs, notes. The sleeve stays beside the songs while they scroll when the
// pane is tall enough to show all of it.
func (a *album) wide(c *ui.Context, m *model, lay layout, head, tracks func(), notes bool) {
	grid := ui.Grid(c.Key("wide")).FillWidth().
		ColumnTracks(ui.Fixed(lay.cover+32), ui.Fr(1)).
		RowTracks(ui.FitContent(), ui.FitContent(), ui.FitContent(), ui.Fr(1)).
		GapX(40).AlignItems(ui.Start)
	grid.Children(func() {
		ui.Column(c.Key("head")).ColumnStart(2).RowStart(1).MinWidth(0).Padding(16, 0, 0, 0).Children(head)
		sleeve := a.sleeve(c, m, func() {
			ui.Column(c).Padding(16).Children(func() {
				a.cover(c, m, lay.cover)
				if len(m.discs) > 0 {
					ui.Column(c.Key("discs")).Margin(16, 0, 0, 0).Children(func() { a.discIndex(c, m) })
				}
				if len(m.facts) > 0 || m.rights != "" {
					ui.Column(c.Key("fine")).Margin(16, 0, 0, 0).Children(func() { a.finePrint(c, m) })
				}
			})
		})
		sleeve.ColumnStart(1).RowStart(1).RowSpan(4).AlignSelf(ui.Start)
		a.stick(sleeve, grid)
		ui.Column(c.Key("songs")).ColumnStart(2).RowStart(2).Margin(28, 0, 0, 0).MinWidth(0).Children(tracks)
		if notes {
			ui.Column(c.Key("notes")).ColumnStart(2).RowStart(3).Margin(40, 0, 0, 0).MinWidth(0).Children(func() {
				a.notesShelf(c, m, true)
			})
		}
	})
}

// stick holds the sleeve stickyTop below the pane's top while the songs
// scroll, within the grid, when the pane shows all of it (v0.2.0's sticky
// panel and useFitsInPane). MyGo has no sticky position: the sleeve is moved
// by the scroll offset of the frame before.
func (a *album) stick(sleeve, grid ui.Element) {
	if a.paneRect == nil {
		return
	}
	pane := a.paneRect()
	if b := sleeve.LayoutBounds(); b.H > 0 {
		a.sleeveH = b.H
	}
	if b := grid.LayoutBounds(); b.H > 0 {
		a.gridH = b.H
		a.gridTop = b.Y - pane.Y + a.pos.Y
	}
	if a.sleeveH+2*stickyTop > pane.H || pane.H == 0 {
		return
	}
	off := a.pos.Y + stickyTop - a.gridTop
	off = min(off, a.gridH-a.sleeveH)
	if off > 0 {
		sleeve.Translate(0, off)
	}
}

// narrow: the sleeve with the cover and the head side by side, then the
// songs, the fine print and the notes (no disc index: the tracks carry their
// disc headings).
func (a *album) narrow(c *ui.Context, m *model, lay layout, head, tracks func(), notes bool) {
	a.sleeve(c, m, func() {
		ui.Row(c).Padding(20).Gap(20).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Shrink(0).Children(func() { a.cover(c, m, lay.cover) })
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(head)
		})
	}).FillWidth()
	ui.Column(c.Key("songs")).Margin(24, 0, 0, 0).FillWidth().Children(tracks)
	if len(m.facts) > 0 || m.rights != "" {
		ui.Column(c.Key("fine")).Margin(24, 0, 0, 0).Children(func() { a.finePrint(c, m) })
	}
	if notes {
		ui.Column(c.Key("notes")).Margin(0, 0, 0, 0).Children(func() { a.notesShelf(c, m, false) })
	}
}

// sleeve is the page's one surface behind the cover and the record's fine
// print: the wash of Go's pre-blurred backdrop under the ground scrim with a
// 4% ink layer to hold it one tone step off the ground. (The cover's own
// background colour, with its contrast maths, is the palette's, phase 3.)
func (a *album) sleeve(c *ui.Context, m *model, content func()) ui.Element {
	colour := ""
	if len(m.header.Colors) > 0 {
		colour = m.header.Colors[0]
	}
	e := kit.Backdrop(c.Key("sleeve"), a.env.Art, kit.BackdropOpts{Src: m.header.Backdrop, Color: colour, Tone: kit.ToneGround}, func() {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(theme.C("ink").Alpha(0.04))
		ui.Column(c.Key("sleeve-body")).FillWidth().Children(content)
	})
	return e.Radius(theme.Current().Radius["lg"])
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
func (a *album) head(c *ui.Context, m *model, now time.Time, account bool) {
	tk := theme.Current()
	ui.Column(c.Key("head-body")).MinWidth(0).Children(func() {
		typed(c, m.album.Name, "display").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(1)
		ui.Row(c.Key("byline")).Margin(6, 0, 0, 0).MinWidth(0).TextColor(theme.C("ink")).
			FontSize(tk.Type["heading"].Size).FontWeight(500).Children(func() { a.byline(c, m) })
		typed(c, m.meta, "meta").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
		if m.album.Explicit || len(kit.TraitLabels(m.album.Traits)) > 0 {
			ui.Row(c.Key("badges")).Margin(8, 0, 0, 0).Children(func() {
				kit.Badges(c, kit.BadgesOpts{Explicit: m.album.Explicit, Traits: m.album.Traits})
			})
		}
		if m.lede != "" {
			typed(c, m.lede, "sub").TextColor(theme.C("ink-muted")).MaxWidth(500).Margin(12, 0, 0, 0)
		}
		if status := a.status(c, m, now); status != nil {
			ui.Column(c.Key("status")).Margin(16, 0, 0, 0).Children(status)
		}
		ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(func() {
			a.actions(c, m, account)
		})
	})
}

// byline is the artist: links from the catalogue page, else a link to the
// artist when the album knows one, else the name.
func (a *album) byline(c *ui.Context, m *model) {
	ink := theme.C("ink")
	switch {
	case m.page != nil && len(m.page.Artists) > 0:
		ui.RichText(c).Children(func() {
			for i, ar := range m.page.Artists {
				if i > 0 {
					sep := ", "
					if i == len(m.page.Artists)-1 {
						sep = " & "
					}
					ui.Text(c.Key(sep+ar.ID), sep)
				}
				a.artistName(c.Key(ar.ID), ar.ID, ar.Name, ink)
			}
		})
	case m.page == nil && m.album.ArtistID != "":
		ui.RichText(c).Children(func() { a.artistName(c, m.album.ArtistID, m.album.Artist, ink) })
	default:
		ui.Text(c, m.album.Artist).TextColor(ink).SingleLine()
	}
}

// artistName is a link to the artist's page where that page exists, else the
// name.
func (a *album) artistName(c *ui.Context, id, name string, col ui.Color) {
	if a.has("/artist/" + id) {
		ui.Link(c, name, "/artist/"+id).TextColor(col)
		return
	}
	ui.Text(c, name).TextColor(col)
}

// status is the countdown of an album not out yet, or how much of the record
// the library holds; nil for neither.
func (a *album) status(c *ui.Context, m *model, now time.Time) func() {
	switch {
	case m.upcoming:
		return func() { a.releaseStatus(c, m, now) }
	case m.share != nil:
		return func() { a.libraryShare(c, m) }
	}
	return nil
}

// releaseStatus is an upcoming album's release, where its play controls will
// be: the time left to the minute, the day in full, and how many of its songs
// are already out (they play from their rows).
func (a *album) releaseStatus(c *ui.Context, m *model, now time.Time) {
	if m.album.ReleaseAt == 0 {
		// Apple gave only a year: no countdown to give.
		typed(c, i18n.T("Out {releaseDate}", "releaseDate", longDate(m.album.ReleaseDate)), "heading").TextColor(theme.C("ink"))
		return
	}
	left := time.UnixMilli(m.album.ReleaseAt).Sub(now)
	typed(c, i18n.T("Out in {timeLeft}", "timeLeft", timeLeft(left)), "heading").TextColor(theme.C("ink"))
	line := fullReleaseDate(m.album.ReleaseAt)
	if m.songsOut > 0 {
		line += " · " + m.songsOutText()
	}
	typed(c, line, "meta").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
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
// ratings and the album's menu; an album not out yet plays the songs out now,
// and offers Pre-add to a signed-in account.
func (a *album) actions(c *ui.Context, m *model, account bool) {
	al := m.album
	item := kit.StateOfAlbum(al)
	from := services.PlayFrom{Kind: "album", ID: al.ID}
	if m.upcoming {
		// Not out yet: Play takes the songs already out, then Remind me, then
		// Apple's pre-add while signed in.
		if m.songsOut > 0 {
			if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{PlayLabel: m.playOutLabel()}); play {
				a.queue.PlaySongs(m.list, 0, from)
			}
		}
		kit.RemindButton(c, a.env.Queries, a.env.Core, al)
		if account {
			a.states.LibraryButton(c, kit.StateAlbums, item, al.Name, i18n.T("Pre-add"))
		}
	} else {
		play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: al.Name, Shuffle: true, Disabled: m.songsOut == 0})
		if play {
			a.queue.PlaySongs(m.list, 0, from)
		}
		if shuffle {
			a.queue.ShuffleSongs(m.list, from)
		}
		a.states.LibraryButton(c, kit.StateAlbums, item, al.Name, "")
	}
	a.states.RatingButtons(c, kit.StateAlbums, item, al.Name)
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
		Discs:      true,
		Highlight:  highlight,
		Popular:    m.popular,
		Subline:    func(s *services.Song) string { return subline(*s) },
		Container:  &kit.ActionContainer{Kind: "album", ID: m.album.ID},
		Restricted: restricted,
		Current: func(s *services.Song) bool {
			return cur.Current != nil && kit.SongIsCurrent(s, cur.Current.ID, cur.Current.CatalogID)
		},
		Playing: playing,
		Art:     a.env.Art,
		Menus:   a.trackMenus,
		Motion:  a.env.Motion,
		Pane:    &kit.Pane{Pos: &a.pos, Bounds: a.paneRect},
		OnPlay:  func(i int) { a.queue.PlaySongs(list, i, services.PlayFrom{Kind: "album", ID: m.album.ID}) },
	}
	if m.classical {
		composers := m.composers
		o.Title = func(s *services.Song) string { return movementTitle(*s) }
		o.Heading = func(s, prev *services.Song) string {
			if s == nil {
				return ""
			}
			return workHeading(*s, prev, composers)
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
			btn := ui.ButtonBase(c.Key("disc-" + strconv.Itoa(d.number))).Height(32).FillWidth().PaddingX(8).
				Radius(tk.Radius["md"]).Label(name + " · " + count)
			bg := ui.Transparent
			if btn.Hovered() {
				bg = theme.C("ink").Alpha(0.06)
			}
			btn.Background(bg)
			btn.Children(func() {
				ui.Row(c).MinWidth(0).AlignItems(ui.Center).Children(func() {
					typed(c, name, "sub").FontWeight(500).TextColor(theme.C("ink")).SingleLine().Shrink(0)
					typed(c, "\u00a0· "+count, "sub").TextColor(theme.C("ink-muted")).SingleLine().Shrink(1).MinWidth(0)
				})
			})
			if btn.Clicked() && a.list.at != nil {
				// The disc's heading sits 16 DIPs from the pane's top.
				a.tracks.ScrollToBlock(&kit.Pane{Pos: &a.pos, Bounds: a.paneRect}, &a.list.opts, a.list.count, a.list.at, d.first, 16)
			}
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
						typed(c, f.label, "meta").TextColor(theme.C("ink-muted"))
						typed(c, f.value, "meta").TextColor(theme.C("ink"))
					})
				}
			})
		}
		if m.rights != "" {
			e := typed(c, m.rights, "meta").TextColor(theme.C("ink-muted"))
			if len(m.facts) > 0 {
				e.Margin(12, 0, 0, 0)
			}
		}
	})
}
