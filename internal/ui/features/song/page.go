package song

import (
	"context"
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
	padTop        = 28   // pt-7
	padBottom     = 48   // pb-12
	narrow        = 960  // the page's gutters shrink below this width
	sideBySide    = 1100 // lyrics and credits share a row from here
	twoColumns    = 720  // a credit group's people in two columns from here
	avatar        = 40   // a credited person's picture
	lyricsMeasure = 540  // max-w-[60ch]
)

// pageState is the view state of the page being shown; it starts over when
// another song opens.
type pageState struct {
	id  string
	pos ui.ScrollState
	// full shows the whole lyrics instead of Apple's excerpt.
	full bool
	// lyricsErr is why the full lyrics failed: they are not asked again
	// until Try again (the query has no retry, as v0.2.0's retry: false).
	lyricsErr error
	retrying  bool

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

	tk := theme.Current()
	ww, _ := c.Size()
	gutter := tk.Metric["gutter"]
	if ww < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	sc := ui.Scroll(c).TrackScroll(&pg.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+tk.Metric["scrollbar"], padBottom, gutter).Children(func() {
			switch {
			case q.HasData:
				s.content(c, q.Data)
			case q.Err != nil:
				s.failed(c, id, q)
			default:
				kit.DetailSkeleton(c, false)
			}
		})
	})
}

// failed is DetailError: the song could not be loaded.
func (s *song) failed(c *ui.Context, id string, q store.Result[services.CatalogSong]) {
	back := kit.EmptyAction{Label: i18n.T("Go to Songs")}
	if s.has("/library/songs") {
		back.Run = func() { s.env.Nav.Push("/library/songs") }
	}
	kit.DetailError(c, kit.NounSong, q.Err, kit.DetailErrorOpts{
		Retry:    func() { s.env.Queries.Invalidate(keySongPage(id)) },
		Retrying: q.Fetching,
		Back:     back,
		SignIn:   s.signIn(),
	})
}

// has reports whether path is a page of a registered module.
func (s *song) has(path string) bool {
	return s.env.HasRoute != nil && s.env.HasRoute(path)
}

// content is a loaded song page.
func (s *song) content(c *ui.Context, page services.CatalogSong) {
	pg := &s.page
	sg := page.Song
	hasLyrics := page.LyricsExcerpt != ""
	hasCredits := len(page.Credits) > 0

	s.header(c, page)
	if hasLyrics || hasCredits {
		s.lyricsAndCredits(c, page, hasLyrics, hasCredits)
	} else {
		ui.Text(c, i18n.T("Apple Music has no credits or lyrics for this song.")).
			FontSize(theme.Current().Type["secondary"].Size).FixedLineHeight(theme.Current().Type["secondary"].LineHeight).
			TextColor(theme.C("ink-muted"))
	}

	ictx := s.env.ItemsCtx(s.menus)
	if len(page.Videos) > 0 && s.has("/music-video/x") {
		items := make([]services.Item, len(page.Videos))
		for i, v := range page.Videos {
			items[i] = services.Item{
				Kind: services.ItemMusicVideo, ID: firstOf(v.CatalogID, v.ID), Name: v.Name,
				Subtitle: i18n.YearOf(v.ReleaseDate), Art: v.Art, Explicit: v.Explicit, MusicVideo: &v,
			}
		}
		kit.ItemShelf(c, &pg.videos, i18n.T("Music videos"), items, ictx, kit.ItemShelfOpts{})
	}
	if len(page.MoreByArtist) > 0 && s.has("/album/x") {
		items := make([]services.Item, len(page.MoreByArtist))
		for i, a := range page.MoreByArtist {
			items[i] = services.Item{
				Kind: services.ItemAlbum, ID: a.ID, Name: a.Name, Subtitle: i18n.YearOf(a.ReleaseDate), Art: a.Art, Album: &a,
			}
		}
		o := kit.ItemShelfOpts{}
		if more := "/artist/" + sg.ArtistID + "/view/full-albums"; sg.ArtistID != "" && s.has(more) {
			o.More = more
		}
		artist := sg.Artist
		kit.ItemShelf(c, &pg.albums, i18n.T("More by {artistName}", "artistName", artist), items, ictx, o)
	}
	s.footer(c, page)
	if s.menus != nil {
		s.menus.Popup(c)
	}
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// header is the CollectionHeader of the song.
func (s *song) header(c *ui.Context, page services.CatalogSong) {
	sg := page.Song
	meta := make([]string, 0, 4)
	for _, p := range []string{i18n.T("Song"), i18n.YearOf(sg.ReleaseDate), i18n.FormatDuration(time.Duration(sg.DurationMs) * time.Millisecond), sg.Genre} {
		if p != "" {
			meta = append(meta, p)
		}
	}
	var color string
	if len(page.Header.Colors) > 0 {
		color = page.Header.Colors[0]
	}
	kit.CollectionHeader(c, kit.CollectionHeaderOpts{
		Title:  sg.Name,
		Art:    sg.Art.Large,
		Loader: s.env.Art,
		Wash:   &kit.BackdropOpts{Src: page.Header.Backdrop, Color: color, Tone: kit.ToneGround},
		Byline: func() { s.byline(c, page) },
		Meta:   strings.Join(meta, " · "),
		Badges: func() { kit.Badges(c, kit.BadgesOpts{Explicit: sg.Explicit, Traits: sg.Traits}) },
		Actions: func() {
			if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: sg.Name, Disabled: !sg.Playable}); play {
				s.env.Queue.PlaySongs([]services.Song{sg}, 0, services.PlayFrom{ID: sg.ID})
			}
			s.env.ItemStates.LibraryButton(c, kit.StateSongs, kit.StateOfSong(sg), sg.Name, "")
			s.env.ItemStates.RatingButtons(c, kit.StateSongs, kit.StateOfSong(sg), sg.Name)
			if s.menus != nil {
				s.menus.Trigger(c, kit.ActionTarget{Item: kit.SongItem(sg), Name: sg.Name}, kit.TriggerOpts{Header: true})
			}
		},
	})
}

// byline is the artists as links (their names when Apple credits none) and
// " · album" linking to the album with this song marked.
func (s *song) byline(c *ui.Context, page services.CatalogSong) {
	kit.ArtistLinks(c.Key("artists"), page.Artists, page.Song.Artist, ui.Color{}, kit.ArtistLinksOpts{HasRoute: s.env.HasRoute})
	al := page.Album
	if al == nil {
		return
	}
	ui.RichText(c.Key("album")).Children(func() {
		muted := theme.C("ink-muted")
		ui.Text(c.Key("dot"), " · ").TextColor(muted)
		if path := "/album/" + al.ID; s.has(path) {
			ui.Link(c.Key("link"), al.Name, path+"?song="+page.Song.ID).TextColor(muted)
		} else {
			ui.Text(c.Key("link"), al.Name).TextColor(muted)
		}
	})
}

// windowWidth is the viewport the CSS breakpoints compare with: the page's
// column plus the sidebar and its line (the shell's chrome beside it).
func windowWidth(c *ui.Context) float32 {
	ww, _ := c.Size()
	return ww + theme.Current().Metric["sidebar"] + 1
}

// lyricsAndCredits puts the two shelves side by side from 1100 DIPs, one
// above the other below.
func (s *song) lyricsAndCredits(c *ui.Context, page services.CatalogSong, lyrics, credits bool) {
	if windowWidth(c) >= sideBySide {
		ui.Row(c.Key("both")).FillWidth().AlignItems(ui.Start).Gap(64).Children(func() {
			ui.Column(c.Key("lyrics")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				if lyrics {
					s.lyricsShelf(c, page)
				}
			})
			ui.Column(c.Key("credits")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				if credits {
					s.creditsShelf(c, page.Credits)
				}
			})
		})
		return
	}
	ui.Column(c.Key("both")).FillWidth().Gap(40).Children(func() {
		if lyrics {
			s.lyricsShelf(c, page)
		}
		if credits {
			s.creditsShelf(c, page.Credits)
		}
	})
}

// lyricsShelf is LyricsBlock: the excerpt Apple sends, and the whole lyrics
// on request.
func (s *song) lyricsShelf(c *ui.Context, page services.CatalogSong) {
	pg := &s.page
	id := firstOf(page.Song.CatalogID, page.Song.ID)
	var full store.Result[services.Lyrics]
	if pg.full && (pg.lyricsErr == nil || pg.retrying) {
		full = store.UseLyricsFrom(s.env.Queries, id, func(ctx context.Context) (services.Lyrics, error) {
			return s.lyrics.Lyrics(ctx, id)
		})
		switch {
		case full.HasData:
			pg.lyricsErr, pg.retrying = nil, false
		case full.Err != nil && !full.Fetching:
			pg.lyricsErr, pg.retrying = full.Err, false
		}
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
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Lyrics"), First: true, Actions: func() {
		if kit.ShelfToggle(c, toggle, pg.full).Clicked() {
			pg.full = !pg.full
			if pg.full && pg.lyricsErr != nil {
				// Opening again after a failure asks again.
				pg.lyricsErr, pg.retrying = nil, false
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
		case pg.lyricsErr != nil:
			kit.ErrorBlock(c, i18n.T("Couldn't load the lyrics"), pg.lyricsErr, kit.ErrorBlockOpts{
				OnRetry: func() {
					pg.retrying = true
					s.env.Queries.Invalidate(store.KeyLyrics(id))
				},
				Retrying: pg.retrying,
			}).Margin(12, 0, 0, 0)
		case !full.HasData:
			ui.Text(c, i18n.T("Loading the lyrics…")).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
				TextColor(theme.C("ink-muted")).Margin(8, 0, 0, 0)
		}
		if w := strings.Join(full.Data.Songwriters, ", "); full.HasData && w != "" {
			ui.Text(c, i18n.T("Written by {songwriters}", "songwriters", w)).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
				TextColor(theme.C("ink-muted")).Margin(16, 0, 0, 0)
		}
	})
}

// creditsShelf is Credits: one labelled section per credit group, each
// person with a picture or initials, their name and roles.
func (s *song) creditsShelf(c *ui.Context, groups []services.CreditGroup) {
	cols := 1
	if windowWidth(c) >= twoColumns {
		cols = 2
	}
	tk := theme.Current().Type
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Credits"), First: true}, func() {
		ui.Column(c.Key("groups")).FillWidth().Gap(32).Children(func() {
			for gi, g := range groups {
				title := creditTitle(g)
				ui.Column(c.Key(gi)).FillWidth().Role(ui.RoleGroup).Label(title).Children(func() {
					ui.Text(c, title).FontSize(tk["secondary"].Size).FixedLineHeight(tk["secondary"].LineHeight).FontWeight(600).
						TextColor(theme.C("ink-muted")).Margin(0, 0, 8, 0).Role(ui.RoleHeading).Level(3)
					ui.Column(c.Key("people")).FillWidth().Gap(12).Children(func() {
						for from := 0; from < len(g.Credits); from += cols {
							ui.Row(c.Key(from)).FillWidth().AlignItems(ui.Start).Gap(32).Children(func() {
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

// footer is the release date, composer and ISRC lines.
func (s *song) footer(c *ui.Context, page services.CatalogSong) {
	tk := theme.Current().Type["meta"]
	line := func(key, text string, first bool) {
		t := ui.Text(c.Key(key), text).FontSize(tk.Size).FixedLineHeight(tk.LineHeight).TextColor(theme.C("ink-muted"))
		if !first {
			t.Margin(2, 0, 0, 0)
		}
	}
	var lines [][2]string
	if page.Song.ReleaseDate != "" {
		lines = append(lines, [2]string{"released", i18n.T("Released {releaseDate}", "releaseDate", i18n.FormatISODate(page.Song.ReleaseDate, i18n.DateLong))})
	}
	if page.Composer != "" {
		lines = append(lines, [2]string{"composer", i18n.T("Composed by {composer}", "composer", page.Composer)})
	}
	if page.ISRC != "" {
		lines = append(lines, [2]string{"isrc", i18n.T("ISRC {isrc}", "isrc", page.ISRC)})
	}
	if len(lines) == 0 {
		return
	}
	ui.Column(c.Key("footer")).Margin(32, 2, 0, 2).Role(ui.RoleGroup).Children(func() {
		for i, l := range lines {
			line(l[0], l[1], i == 0)
		}
	})
}

// signIn opens the account dialog from the "not signed in" error; nil (no
// button) without the shell's surfaces.
func (s *song) signIn() func() {
	if s.env.Surfaces == nil {
		return nil
	}
	return s.env.Surfaces.OpenAccount
}
