package video

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry in DIPs.
const (
	padTop    = 24
	padBottom = 48
	narrow    = 960
	// The lead picture is between heroMinH and heroMaxH DIPs high (a picture,
	// not a wall); captionH is what the caption row and its gap take under it.
	heroMinH = 360
	heroMaxH = 630
	captionH = 112
	// stackBelow is the picture width under which the actions go under the
	// title block instead of beside it.
	stackBelow = 720
	// tileMin is the smallest tile of the shelves of videos.
	tileMin = 296
)

// Format badges: technical names, the same in every language.
const (
	format4K  = "4K"
	formatHDR = "HDR"
)

// yearOf is the year of an ISO date ("" when it has none).
func yearOf(date string) string {
	if len(date) >= 4 {
		if _, err := strconv.Atoi(date[:4]); err == nil {
			return date[:4]
		}
	}
	return ""
}

// longDate is a release date in words ("10 October 2007") for a full ISO
// date, the year for a partial one, "" for none.
func longDate(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return yearOf(date)
	}
	return i18n.FormatDate(t, i18n.DateLong)
}

// videoPage is the id of the page being built, from the route.
func (v *video) page(c *ui.Context, r *ui.Route) {
	id := r.Param("id")
	if v.shown != id {
		v.scroll = ui.ScrollState{} // another video: from its top
		v.shown = id
	}
	q := v.videoQuery(id)
	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if v.pageW > 0 && v.pageW < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	devstate.Scroll(c, &v.scroll) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(&v.scroll).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	defer func() {
		// The next frame's gutters and the picture's exact width.
		if w := sc.Bounds().W; w != v.pageW {
			v.pageW = w
			c.Invalidate()
		}
	}()
	sc.Children(func() {
		scrollbar := tk.Metric["scrollbar"]
		ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Children(func() {
			switch {
			case q.HasData:
				v.body(c, q.Data, v.widths(c, gutter))
			case q.Err != nil:
				back := kit.EmptyAction{}
				if v.env.Routed("/search") {
					back = kit.EmptyAction{Label: i18n.T("Go to Search"), Run: func() { v.env.Nav.Push("/search") }}
				}
				kit.DetailError(c, kit.NounMusicVideo, q.Err, kit.DetailErrorOpts{
					Retry:    func() { v.env.Queries.Invalidate(keyVideo(id)) },
					Retrying: q.Fetching,
					Back:     back,
				})
			default:
				v.skeleton(c, v.widths(c, gutter))
			}
		})
	})
}

// widths is the page's usable width inside its gutters and the picture's. The
// picture is as wide as the content, but no taller than the window can show
// above the fold with its caption (never under heroMinH or over heroMaxH
// high), and a whole number of DIPs, so it is asked for at the box it is
// drawn in. Before the first layout the content is taken to be the window
// less the sidebar.
type widths struct{ content, hero float32 }

func (v *video) widths(c *ui.Context, gutter float32) widths {
	tk := theme.Current()
	ww, wh := c.Size()
	w := v.pageW
	if w <= 0 {
		w = ww - tk.Metric["sidebar"]
	}
	content := float32(math.Floor(float64(max(w-2*gutter-tk.Metric["scrollbar"], 0))))
	// What is left of the window under the bars, the top padding and the
	// caption.
	fold := wh - tk.Metric["top-bar"] - tk.Metric["player-bar"] - padTop - captionH
	h := min(max(fold, heroMinH), heroMaxH)
	return widths{content: content, hero: min(content, float32(math.Floor(float64(h*16/9))))}
}

// skeleton is the loaded page at its real sizes: the 16:9 picture, the
// caption's bars and a row of tiles, as flat --hover shapes. It is a
// labelled "Loading" status.
func (v *video) skeleton(c *ui.Context, w widths) {
	hover := theme.C("hover")
	sm := theme.Current().Radius["sm"]
	bar := func(width, height float32) ui.Element {
		return ui.Box(c).Width(width).Height(height).Radius(sm).Background(hover).Shrink(0)
	}
	ui.Column(c).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Box(c.Key("picture")).Size(w.hero, w.hero*9/16).Radius(sm).Background(hover).Shrink(0)
		ui.Column(c.Key("caption")).Width(w.hero).Margin(20, 0, 0, 0).Children(func() {
			bar(w.hero*0.4, 36)
			bar(w.hero*0.2, 22).Margin(6, 0, 0, 0)
			bar(w.hero*0.3, 16).Margin(8, 0, 0, 0)
		})
		bar(120, 22).Margin(48, 0, 12, 0)
		cols, tile := kit.CoverColumns(w.content, tileMin)
		ui.Row(c.Key("tiles")).Gap(kit.TileGapX).Children(func() {
			for i := range cols {
				ui.Column(c.Key(i)).Width(tile).Shrink(0).Children(func() {
					ui.Box(c).Size(tile, tile*9/16).Radius(sm).Background(hover)
					bar(tile*0.6, 14).Margin(8, 0, 0, 0)
					bar(tile*0.3, 12).Margin(6, 0, 0, 0)
				})
			}
		})
	})
}

// body is the loaded page, as a screening: the picture at the full content
// width, its caption row under it, the note on definition, the notes, and
// the shelves of videos.
func (v *video) body(c *ui.Context, d services.CatalogVideo, w widths) {
	vid := d.Video
	caps, known := v.caps()
	videoPlays := known && caps.Video
	v.hero(c, d, videoPlays, w.hero)

	if known || d.Song != nil {
		note := ui.RichText(c).MaxWidth(68*8).Margin(24, 0, 0, 0).TextColor(theme.C("ink-muted"))
		sub := theme.Current().Type["sub"]
		note.FontSize(sub.Size).FixedLineHeight(sub.LineHeight)
		note.Children(func() {
			switch {
			case !known: // neither the note nor the play control before Go has answered
			case videoPlays:
				ui.Text(c.Key("plays"), i18n.T("Sona plays music videos in standard definition: Apple allows HD only on players with hardware protection."))
			default:
				ui.Text(c.Key("cannot"), i18n.T("Sona can't play music videos here: the video opens in Apple Music in your browser."))
			}
			if d.Song != nil {
				if known {
					ui.Text(c.Key("gap"), " ")
				}
				v.songLine(c, *d.Song)
			}
		})
	}

	if d.Notes != "" {
		kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Notes")}, func() { kit.Notes(c, d.Notes, 0) })
	}
	var seeAll func()
	if p := "/artist/" + vid.ArtistID + "/view/music-videos"; vid.ArtistID != "" && v.env.Routed(p) {
		seeAll = func() { v.env.Nav.Push(p) }
	}
	by := kit.CoverShelfOpts{CoverOpts: v.coverOpts(), More: seeAll}
	v.shelf(c, &v.byline, i18n.T("More by {artistName}", "artistName", vid.Artist), d.MoreByArtist, true, by)
	v.shelf(c, &v.more, i18n.T("More like this"), d.MoreInGenre, false, kit.CoverShelfOpts{CoverOpts: v.coverOpts()})
	if vid.ReleaseDate != "" {
		date := longDate(vid.ReleaseDate)
		meta := theme.Current().Type["meta"]
		ui.Text(c, i18n.T("Released {releaseDate}", "releaseDate", date)).Margin(32, 0, 0, 0).
			FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(theme.C("ink-muted"))
	}
	if v.menus != nil {
		v.menus.Popup(c)
	}
}

// songLine is "The song is {link} from {album}." (or without the album): the
// song's name links to its page where there is one.
func (v *video) songLine(c *ui.Context, s services.Song) {
	const mark = "\x00"
	text := i18n.T("The song is {link}.", "link", mark)
	if s.Album != "" {
		text = i18n.T("The song is {link} from {albumName}.", "link", mark, "albumName", s.Album)
	}
	before, after, _ := strings.Cut(text, mark)
	id := s.CatalogID
	if id == "" {
		id = s.ID
	}
	ui.Text(c.Key("song-before"), before)
	if path := "/song/" + id; v.env.Routed(path) {
		ui.Link(c.Key("song"), s.Name, path).TextColor(theme.C("ink"))
	} else {
		ui.Text(c.Key("song"), s.Name).TextColor(theme.C("ink"))
	}
	ui.Text(c.Key("song-after"), after)
}

// hero is the page's lead: the 16:9 picture at width w (the cover is the
// colour, so there is no wash behind it) and, directly under it at the same
// width, the caption row: the title block at the left, the actions at the
// right. Under stackBelow the actions go under the title block.
func (v *video) hero(c *ui.Context, d services.CatalogVideo, videoPlays bool, w float32) {
	vid := d.Video
	kit.Artwork(c, v.env.Art, vid.Art.Large, w, kit.ArtOpts{Aspect: 16.0 / 9, Edge: true}).Shrink(0)

	title := func() {
		tk := theme.Current()
		d0 := tk.Type["display"]
		ui.Text(c, vid.Name).FontSize(d0.Size).FixedLineHeight(d0.LineHeight).FontWeight(int(d0.Weight)).LetterSpacing(d0.Tracking).
			TextColor(theme.C("ink")).MaxLines(2).Role(ui.RoleHeading).Level(1)
		ui.Row(c.Key("byline")).Margin(6, 0, 0, 0).MinWidth(0).TextColor(theme.C("ink")).FontSize(tk.Type["heading"].Size).
			FontWeight(500).Children(func() { kit.ArtistLinks(c, d.Artists, vid.Artist, ui.Color{}) })
		ui.Row(c.Key("meta")).Wrap().GapX(10).GapY(4).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
			ui.Text(c, v.catalogueLine(vid)).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
				FontWeight(int(tk.Type["meta"].Weight)).TextColor(theme.C("ink-muted"))
			kit.Badges(c, kit.BadgesOpts{Explicit: vid.Explicit, Extra: formats(vid)})
		})
	}
	actions := func() {
		if videoPlays {
			if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: vid.Name, Bind: &v.playBtn}); play {
				v.playVideos([]services.MusicVideo{vid}, 0, &v.playBtn)
			}
		}
		if vid.URL != "" {
			variant := kit.Default
			if videoPlays {
				variant = kit.Outline
			}
			if kit.Button(c.Key("watch"), i18n.T("Watch in Apple Music"), kit.ButtonOpts{Variant: variant, Size: kit.SizeSM, Icon: "external-link"}).Clicked() {
				v.openInAppleMusic(vid.URL)
			}
		}
		if s := d.Song; s != nil && s.Playable {
			if kit.Button(c.Key("song"), i18n.T("Play the song"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "play"}).Clicked() {
				v.q.PlaySongs([]services.Song{*s}, 0, services.PlayFrom{ID: s.ID})
			}
		}
		if videoPlays && v.menus != nil {
			v.menus.Trigger(c, kit.ActionTarget{Item: kit.MusicVideoItem(vid), Name: vid.Name}, kit.TriggerOpts{Header: true})
		}
	}
	if w < stackBelow {
		ui.Column(c.Key("caption")).Width(w).Margin(20, 0, 0, 0).Children(func() {
			ui.Column(c.Key("title")).MinWidth(0).Children(title)
			ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(16, 0, 0, 0).Children(actions)
		})
		return
	}
	ui.Row(c.Key("caption")).Width(w).Margin(20, 0, 0, 0).Gap(32).AlignItems(ui.Start).Children(func() {
		ui.Column(c.Key("title")).Grow(1).Basis(0).MinWidth(0).Children(title)
		ui.Row(c.Key("actions")).Shrink(0).Gap(8).AlignItems(ui.Center).Children(actions)
	})
}

// catalogueLine is "Music video · 2019 · 3:35 · Pop".
func (v *video) catalogueLine(vid services.MusicVideo) string {
	parts := []string{i18n.T("Music video")}
	for _, p := range []string{yearOf(vid.ReleaseDate), i18n.FormatDuration(time.Duration(vid.DurationMs) * time.Millisecond), vid.Genre} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// formats are the video's format marks.
func formats(vid services.MusicVideo) []string {
	var out []string
	if vid.Has4K {
		out = append(out, format4K)
	}
	if vid.HasHDR {
		out = append(out, formatHDR)
	}
	return out
}

// coverOpts are the shelves' tile options: 16:9 tiles, one menu for the page.
func (v *video) coverOpts() kit.CoverOpts {
	return kit.CoverOpts{Video: true, Min: tileMin, Art: v.env.Art, Motion: v.env.Motion, Menus: v.menus, NoPopup: true}
}

// shelf is one shelf of music video tiles (nothing for an empty list). byYear
// puts the year under a title, as "More by" does, else the artist.
func (v *video) shelf(c *ui.Context, st *kit.CoverShelfState, title string, videos []services.MusicVideo, byYear bool, o kit.CoverShelfOpts) {
	if len(videos) == 0 {
		return
	}
	caps, known := v.caps()
	plays := known && caps.Video
	items := make([]kit.CoverItem, len(videos))
	for i, m := range videos {
		items[i] = v.tile(m, byYear, plays)
	}
	kit.CoverShelf(c, st, title, items, o)
}

// tile is a music video's tile (v0.2.0's videoTile): 16:9, linking to its
// page, with a play control and the item menu only where videos play.
func (v *video) tile(m services.MusicVideo, byYear, plays bool) kit.CoverItem {
	id := m.CatalogID
	if id == "" {
		id = m.ID
	}
	sub := m.Artist
	if byYear {
		sub = yearOf(m.ReleaseDate)
	}
	t := kit.CoverItem{
		Key: m.ID, Title: m.Name, Subtitle: sub, Art: m.Art.Medium, Video: true, Explicit: m.Explicit,
		Open: func() { v.env.Nav.Push("/music-video/" + id) },
	}
	if plays {
		t.Play = &kit.CoverPlay{Label: m.Name, RunFrom: func(opener *ui.Handle) { v.playVideos([]services.MusicVideo{m}, 0, opener) }}
		item := kit.MusicVideoItem(m)
		t.Item = &item
	}
	return t
}
