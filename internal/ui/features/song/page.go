package song

import (
	"cmp"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry from the reference (AppShell's main and SongPage), in
// DIPs. The breakpoints are the viewport's (Tailwind's min-[…]), which is
// the window's width here.
const (
	wideFrom    = 860 // the cover, title and actions are a column beside the body from here
	avatar      = 40  // a credited person's picture
	coverWide   = 240 // the cover beside the body
	coverNarrow = 136 // the cover in the band
	spineTop    = 32  // above the cover, from the top of the page
	spineGap    = 28  // the wash's padding after the cover
	splitGap    = 40  // between the wash and the body
	bodyMax     = 760 // the body's widest
	nameColumn  = 112 // a fact's name
	personMin   = 210 // the least width of a credited person
	colGap      = 24  // between people

	lyricsMeasure = 540 // max-w-[60ch]
)

// pageState is the view state of the page being shown; it starts over when
// another song opens.
type pageState struct {
	id  string
	pos ui.ScrollState
	// full shows the whole lyrics instead of Apple's excerpt.
	full bool
	// failed is the full lyrics' last answer being an error: opening them
	// again asks again.
	failed bool
	// retrying is Try again's ask in flight: the failure stays up, saying so.
	// Opening the lyrics again asks too, but shows them loading.
	retrying bool

	videos, albums kit.CoverShelfState
}

// songPage is SongPage: the header, the lyrics and credits, the music videos
// and more by the artist, in one scroll container.
func (s *song) songPage(c *ui.Context, r *ui.Route) {
	id := r.Param("id")
	pg := &s.page
	if pg.id != id {
		*pg = pageState{id: id}
	}
	q := store.Use(s.env.Queries, keySongPage(id), func(ctx context.Context) (services.CatalogSong, error) {
		return s.catalog.SongPage(ctx, id)
	}, store.Options{Stale: store.Forever})

	ww, _ := c.Size()
	kit.PageScroll(c, &pg.pos, kit.PageGutter(ww), func(*kit.Pane) {
		switch {
		case q.HasData:
			s.content(c, q.Data)
		case q.Err != nil:
			s.failed(c, id, q)
		default:
			s.skeleton(c)
		}
	})
}

// failed is DetailError: the song could not be loaded.
func (s *song) failed(c *ui.Context, id string, q store.Result[services.CatalogSong]) {
	back := kit.BackAction(i18n.T("Go to Songs"), "/library/songs", s.env.Routed("/library/songs"), func(path string) { s.env.Nav.Push(path) })
	kit.DetailQueryError(c, kit.NounSong, s.env.Queries, q, back, s.signIn(), keySongPage(id))
}

// content is a loaded song page.
func (s *song) content(c *ui.Context, page services.CatalogSong) {
	pg := &s.page
	sg := page.Song
	sh := shapeOf(c)

	wash := kit.BackdropOpts{Src: page.Header.Backdrop, Tone: kit.ToneGround}
	if len(page.Header.Colors) > 0 {
		wash.Color = page.Header.Colors[0]
	}
	s.frame(c, sh, wash, func(size float32) {
		kit.Artwork(c, s.env.Art, sg.Art.Large, size, kit.ArtOpts{Edge: true})
	}, func() { s.lead(c, page, sh) }, func() { s.body(c, page, sh) })

	ictx := s.env.ItemsCtx(s.menus)
	if len(page.Videos) > 0 && s.env.Routed("/music-video/x") {
		items := make([]services.Item, len(page.Videos))
		for i, v := range page.Videos {
			items[i] = services.Item{
				Kind: services.ItemMusicVideo, ID: cmp.Or(v.CatalogID, v.ID), Name: v.Name,
				Subtitle: i18n.YearOf(v.ReleaseDate), Art: v.Art, Explicit: v.Explicit, MusicVideo: &v,
			}
		}
		kit.ItemShelf(c, &pg.videos, i18n.T("Music videos"), items, ictx, kit.ItemShelfOpts{})
	}
	if len(page.MoreByArtist) > 0 && s.env.Routed("/album/x") {
		items := make([]services.Item, len(page.MoreByArtist))
		for i, a := range page.MoreByArtist {
			items[i] = services.Item{
				Kind: services.ItemAlbum, ID: a.ID, Name: a.Name, Subtitle: i18n.YearOf(a.ReleaseDate), Art: a.Art, Album: &a,
			}
		}
		o := kit.ItemShelfOpts{}
		if more := "/artist/" + sg.ArtistID + "/view/full-albums"; sg.ArtistID != "" && s.env.Routed(more) {
			o.More = more
		}
		artist := sg.Artist
		kit.ItemShelf(c, &pg.albums, i18n.T("More by {artistName}", "artistName", artist), items, ictx, o)
	}
	if s.menus != nil {
		s.menus.Popup(c)
	}
}

// shape is the composition the page has at its width.
type shape struct {
	// wide puts the cover, title and actions in a column of their own beside
	// the facts, credits and lyrics; otherwise they head the page as a band.
	wide   bool
	gutter float32
	// cols is the people to a row of a credit group.
	cols int
}

func shapeOf(c *ui.Context) shape {
	tk := theme.Current()
	ww, _ := c.Size()
	sh := shape{wide: ww >= wideFrom, gutter: kit.PageGutter(ww)}
	room := ww - 2*sh.gutter - tk.Metric["scrollbar"]
	if sh.wide {
		room = ww - (sh.gutter + coverWide + spineGap) - splitGap - sh.gutter - tk.Metric["scrollbar"]
	}
	room = min(room, bodyMax)
	sh.cols = max(1, min(3, int((room+colGap)/(personMin+colGap))))
	return sh
}

// frame is the page's top: from wideFrom the cover, title and actions are
// a spine of the cover's wash that bleeds off the top and left of the
// column, with the body beside it; narrower they are a full-width band over
// the same wash with the body under it. The loading skeleton is built by it
// too, so nothing moves when the song arrives.
func (s *song) frame(c *ui.Context, sh shape, wash kit.BackdropOpts, cover func(size float32), lead, body func()) {
	if sh.wide {
		ui.Row(c.Key("liner")).FillWidth().AlignItems(ui.Start).Children(func() {
			kit.Backdrop(c.Key("spine"), s.env.Art, wash, func() {
				ui.Column(c).FillWidth().Padding(spineTop, spineGap, 28, sh.gutter).Children(func() {
					cover(coverWide)
					ui.Column(c.Key("lead")).FillWidth().Margin(20, 0, 0, 0).Children(lead)
				})
			}).Width(sh.gutter+coverWide+spineGap).Shrink(0).Margin(-kit.PagePadTop, 0, 0, -sh.gutter)
			ui.Column(c.Key("body")).Grow(1).Basis(0).MinWidth(0).MaxWidth(bodyMax).Margin(1, 0, 0, splitGap).Children(body)
		})
		return
	}
	kit.Backdrop(c.Key("band"), s.env.Art, wash, func() {
		ui.Column(c).FillWidth().Padding(spineTop, sh.gutter, 28, sh.gutter).Children(func() {
			ui.Row(c.Key("row")).FillWidth().AlignItems(ui.End).Gap(24).Children(func() {
				cover(coverNarrow)
				ui.Column(c.Key("lead")).Grow(1).Basis(0).MinWidth(0).Children(lead)
			})
		})
	}).AlignSelf(ui.Stretch).Shrink(0).Margin(-kit.PagePadTop, -sh.gutter, 28, -sh.gutter)
	ui.Column(c.Key("body")).FillWidth().Children(body)
}

// lead is the title, the artists, the album, the badges and the actions of
// the song.
func (s *song) lead(c *ui.Context, page services.CatalogSong, sh shape) {
	sg := page.Song
	tk := theme.Current().Type
	d := tk["display"]
	lines := 2
	if sh.wide {
		lines = 3
	}
	ui.Text(c, sg.Name).FontSize(d.Size).FixedLineHeight(d.LineHeight).FontWeight(int(d.Weight)).LetterSpacing(d.Tracking).
		TextColor(theme.C("ink")).MaxLines(lines).Role(ui.RoleHeading).Level(1)
	ui.Column(c.Key("byline")).FillWidth().Margin(8, 0, 0, 0).TextColor(theme.C("ink")).
		FontSize(tk["heading"].Size).FontWeight(500).Children(func() { s.byline(c, page) })
	if sg.Explicit || len(kit.TraitLabels(sg.Traits)) > 0 {
		ui.Row(c.Key("badges")).Margin(12, 0, 0, 0).Children(func() {
			kit.Badges(c, kit.BadgesOpts{Explicit: sg.Explicit, Traits: sg.Traits})
		})
	}
	ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(func() {
		if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: sg.Name, Disabled: !sg.Playable}); play {
			s.env.Queue.PlaySongs([]services.Song{sg}, 0, services.PlayFrom{ID: sg.ID})
		}
		s.env.ItemStates.LibraryButton(c, kit.StateSongs, kit.StateOfSong(sg), sg.Name, "")
		s.env.ItemStates.RatingButtons(c, kit.StateSongs, kit.StateOfSong(sg), sg.Name)
		if s.menus != nil {
			s.menus.Trigger(c, kit.ActionTarget{Item: kit.SongItem(sg), Name: sg.Name}, kit.TriggerOpts{Header: true})
		}
	})
}

// byline is the artists as links (their names when Apple credits none), and
// under them the album, linking to it with this song marked.
func (s *song) byline(c *ui.Context, page services.CatalogSong) {
	kit.ArtistLinks(c.Key("artists"), page.Artists, page.Song.Artist, ui.Color{}, kit.ArtistLinksOpts{HasRoute: s.env.Routed}).
		AlignSelf(ui.Start)
	al := page.Album
	if al == nil {
		return
	}
	sub := theme.Current().Type["secondary"]
	muted := theme.C("ink-muted")
	if path := "/album/" + al.ID; s.env.Routed(path) {
		ui.Link(c.Key("album"), al.Name, path+"?song="+page.Song.ID).TextColor(muted).FontSize(sub.Size).FontWeight(400).
			AlignSelf(ui.Start).Margin(2, 0, 0, 0)
	} else {
		ui.Text(c.Key("album"), al.Name).TextColor(muted).FontSize(sub.Size).FontWeight(400).Margin(2, 0, 0, 0)
	}
}

// body is the facts, the credits and the lyrics, in that order: what the
// song is, who made it, what it says.
func (s *song) body(c *ui.Context, page services.CatalogSong, sh shape) {
	first := !s.ledger(c, page)
	hasLyrics, hasCredits := page.LyricsExcerpt != "", len(page.Credits) > 0
	if hasCredits {
		s.creditsShelf(c, page.Credits, sh.cols, first)
		first = false
	}
	if hasLyrics {
		s.lyricsShelf(c, page, first)
	}
	if !hasLyrics && !hasCredits {
		sub := theme.Current().Type["secondary"]
		ui.Text(c, i18n.T("Apple Music has no credits or lyrics for this song.")).
			FontSize(sub.Size).FixedLineHeight(sub.LineHeight).TextColor(theme.C("ink-muted")).Margin(32, 0, 0, 0)
	}
}

// skeleton is the page loading: the real frame with flat bars where the
// text will be, a labelled "Loading" status for assistive technology.
func (s *song) skeleton(c *ui.Context) {
	sh := shapeOf(c)
	hover := theme.C("hover")
	bar := func(w, h float32) ui.Element {
		return ui.Box(c).Width(w).Height(h).Radius(6).Background(hover).Shrink(0)
	}
	ui.Column(c.Key("loading")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		s.frame(c, sh, kit.BackdropOpts{}, func(size float32) {
			ui.Box(c).Size(size, size).Radius(4).Background(hover).Shrink(0)
		}, func() {
			ui.Box(c).Height(36).WidthPercent(60).Radius(6).Background(hover)
			ui.Box(c).Height(16).WidthPercent(45).Radius(6).Background(hover).Margin(14, 0, 0, 0)
			ui.Box(c).Height(13).WidthPercent(35).Radius(6).Background(hover).Margin(8, 0, 0, 0)
			ui.Row(c.Key("actions")).Gap(8).Margin(20, 0, 0, 0).Children(func() {
				ui.Box(c).Size(40, 40).Radius(20).Background(hover)
				ui.Box(c).Size(72, 32).Radius(6).Background(hover)
			})
		}, func() {
			bar(72, 16).Margin(6, 0, 6, 0)
			ui.Box(c.Key("ledger-rule")).FillWidth().Height(1).Background(theme.C("line")).Margin(0, 0, 12, 0)
			for i := range 5 {
				ui.Row(c.Key(i)).FillWidth().Height(18).AlignItems(ui.Center).Gap(16).Margin(0, 0, 6, 0).Children(func() {
					bar(nameColumn, 14)
					ui.Box(c).Height(14).WidthPercent(float32(25 + (i*19)%35)).Radius(6).Background(hover)
				})
			}
			bar(64, 16).Margin(40, 0, 18, 0)
			ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line"))
			bar(88, 14).Margin(12, 0, 12, 0)
			for from := 0; from < 2*sh.cols; from += sh.cols {
				ui.Row(c.Key("people"+strconv.Itoa(from))).FillWidth().Gap(colGap).Margin(0, 0, 14, 0).Children(func() {
					for range sh.cols {
						ui.Row(c).Grow(1).Basis(0).Gap(12).Children(func() {
							ui.Box(c).Size(avatar, avatar).Radius(avatar / 2).Background(hover).Shrink(0)
							ui.Column(c).Gap(6).Margin(6, 0, 0, 0).Children(func() {
								bar(110, 14)
								bar(80, 12)
							})
						})
					}
				})
			}
		})
	})
}

// lyricsShelf is LyricsBlock: the excerpt Apple sends, and the whole lyrics
// on request.
func (s *song) lyricsShelf(c *ui.Context, page services.CatalogSong, first bool) {
	pg := &s.page
	id := cmp.Or(page.Song.CatalogID, page.Song.ID)
	var full store.Result[services.Lyrics]
	if pg.full {
		// The query keeps a failure until Try again (NoRetry).
		full = store.UseLyrics(s.env.Queries, s.lyrics, id)
		pg.failed = full.Err != nil && !full.HasData
		pg.retrying = pg.retrying && full.Fetching
	}
	lines := strings.Split(page.LyricsExcerpt, "\n")
	if pg.full && full.HasData && len(full.Data.Lines) > 0 {
		lines = lines[:0]
		for _, l := range full.Data.Lines {
			lines = append(lines, l.Text)
		}
	}
	toggle := i18n.T("View full lyrics")
	if pg.full {
		toggle = i18n.T("Show less")
	}
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Lyrics"), First: first, Actions: func() {
		if kit.ShelfToggle(c, toggle, pg.full).Clicked() {
			pg.full = !pg.full
			if pg.full && pg.failed {
				pg.failed = false
				s.env.Queries.Invalidate(store.KeyLyrics(id))
			}
		}
	}}, func() {
		tk := theme.Current().Type["heading"]
		ui.Column(c.Key("lines")).MaxWidth(lyricsMeasure).Children(func() {
			for i, l := range lines {
				if l == "" {
					ui.Box(c.Key(i)).Height(16) // a blank line is a verse break
					continue
				}
				ui.Text(c.Key(i), l).FontSize(tk.Size).FixedLineHeight(28).FontWeight(500).TextColor(theme.C("ink"))
			}
			if !pg.full {
				ui.Text(c.Key("more"), "…").FontSize(tk.Size).FixedLineHeight(28).FontWeight(500).TextColor(theme.C("ink-faint"))
			}
		})
		if !pg.full {
			return
		}
		sub := theme.Current().Type["secondary"]
		switch {
		case full.Err != nil && !full.HasData && (!full.Fetching || pg.retrying):
			kit.ErrorBlock(c, i18n.T("Couldn't load the lyrics"), full.Err, kit.ErrorBlockOpts{
				OnRetry: func() {
					pg.retrying = true
					s.env.Queries.Invalidate(store.KeyLyrics(id))
				},
				Retrying: pg.retrying,
			}).Margin(12, 0, 0, 0)
		case !full.HasData:
			// A status, announced once as it appears: the focus stays on
			// the toggle that asked.
			msg := i18n.T("Loading the lyrics…")
			wait := ui.Text(c, msg).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
				TextColor(theme.C("ink-muted")).Margin(8, 0, 0, 0).Role(ui.RoleStatus)
			kit.AnnounceMounted(c, wait, msg)
		}
		if w := strings.Join(full.Data.Songwriters, ", "); full.HasData && w != "" {
			ui.Text(c, i18n.T("Written by {songwriters}", "songwriters", w)).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
				TextColor(theme.C("ink-muted")).Margin(16, 0, 0, 0)
		}
	})
}

// creditsShelf is Credits: one labelled section per credit group under a
// rule, each person with a picture or initials, their name and roles, cols
// people to a row.
func (s *song) creditsShelf(c *ui.Context, groups []services.CreditGroup, cols int, first bool) {
	tk := theme.Current().Type
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Credits"), First: first}, func() {
		ui.Column(c.Key("groups")).FillWidth().Gap(20).Children(func() {
			for gi, g := range groups {
				title := creditTitle(g)
				ui.Column(c.Key(gi)).FillWidth().Role(ui.RoleGroup).Label(title).Children(func() {
					ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line"))
					ui.Text(c, title).FontSize(tk["secondary"].Size).FixedLineHeight(tk["secondary"].LineHeight).FontWeight(600).
						TextColor(theme.C("ink-muted")).Margin(12, 0, 12, 0).Role(ui.RoleHeading).Level(3)
					ui.Column(c.Key("people")).FillWidth().Gap(14).Children(func() {
						for from := 0; from < len(g.Credits); from += cols {
							ui.Row(c.Key(from)).FillWidth().AlignItems(ui.Start).Gap(colGap).Children(func() {
								for i := from; i < from+cols; i++ {
									if i >= len(g.Credits) {
										ui.Box(c.Key(i)).Grow(1).Basis(0)
										continue
									}
									s.person(c.Key(i), g.Credits[i])
								}
							})
						}
					})
				})
			}
		})
	})
}

// person is one credit: the avatar, the name and the roles joined by ", ".
func (s *song) person(c *ui.Context, p services.Credit) {
	tk := theme.Current().Type
	ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Start).Children(func() {
		kit.PersonAvatar(c, s.env.Art, p.Name, p.Art.Small, avatar).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(avatar).Justify(ui.Center).Children(func() {
			ui.Text(c, p.Name).FontSize(tk["body"].Size).FixedLineHeight(tk["body"].LineHeight).TextColor(theme.C("ink")).SingleLine()
			ui.Text(c, strings.Join(p.Roles, ", ")).FontSize(tk["secondary"].Size).FixedLineHeight(tk["secondary"].LineHeight).
				TextColor(theme.C("ink-muted"))
		})
	})
}

// ledger is the song's facts, one per row: a muted name and its value. A
// fact Apple does not give has no row (the DTO has no label or ℗ line); it
// reports whether there is a ledger at all.
func (s *song) ledger(c *ui.Context, page services.CatalogSong) bool {
	sg := page.Song
	var rows [][2]string
	add := func(name, value string) {
		if value != "" {
			rows = append(rows, [2]string{name, value})
		}
	}
	if sg.ReleaseDate != "" {
		add(i18n.T("Released"), i18n.FormatISODate(sg.ReleaseDate, i18n.DateLong))
	}
	if sg.DurationMs > 0 {
		add(i18n.T("Duration"), i18n.FormatDuration(time.Duration(sg.DurationMs)*time.Millisecond))
	}
	add(i18n.T("Genre"), sg.Genre)
	if sg.TrackNumber > 0 {
		add(i18n.T("Track"), strconv.Itoa(sg.TrackNumber))
	}
	if sg.DiscNumber > 1 {
		add(i18n.T("Disc"), strconv.Itoa(sg.DiscNumber))
	}
	add(i18n.T("Written by"), page.Composer)
	add(i18n.T("ISRC"), page.ISRC)
	if len(rows) == 0 {
		return false
	}
	tk := theme.Current().Type["secondary"]
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Details"), First: true}, func() {
		ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line")).Margin(0, 0, 12, 0)
		ui.Column(c.Key("facts")).FillWidth().Gap(6).Children(func() {
			for i, r := range rows {
				ui.Row(c.Key(i)).FillWidth().AlignItems(ui.Start).Gap(16).Children(func() {
					ui.Text(c.Key("name"), r[0]).FontSize(tk.Size).FixedLineHeight(tk.LineHeight).
						TextColor(theme.C("ink-muted")).Width(nameColumn).Shrink(0)
					ui.Text(c.Key("value"), r[1]).FontSize(tk.Size).FixedLineHeight(tk.LineHeight).
						TextColor(theme.C("ink")).Grow(1).Basis(0).MinWidth(0)
				})
			}
		})
	})
	return true
}

// signIn opens the account dialog from the "not signed in" error; nil (no
// button) without the shell's surfaces.
func (s *song) signIn() func() {
	if s.env.Surfaces == nil {
		return nil
	}
	return s.env.Surfaces.OpenAccount
}
