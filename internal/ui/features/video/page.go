package video

import (
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

// The page's geometry from the reference (AppShell's main, CollectionHeader)
// in DIPs.
const (
	padTop    = 28 // pt-7
	padBottom = 48 // pb-12
	narrow    = 960
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
	sc := kit.GlassScroll(ui.Scroll(c).TrackScroll(&v.scroll).Grow(1).Basis(0).MinHeight(0).MinWidth(0))
	defer func() { v.pageW = sc.Bounds().W }() // the next frame's gutters
	sc.Children(func() {
		scrollbar := tk.Metric["scrollbar"]
		ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Children(func() {
			switch {
			case q.HasData:
				v.body(c, q.Data)
			case q.Err != nil:
				back := kit.EmptyAction{}
				if v.env.HasRoute != nil && v.env.HasRoute("/search") {
					back = kit.EmptyAction{Label: i18n.T("Go to Search"), Run: func() { v.env.Nav.Push("/search") }}
				}
				kit.DetailError(c, kit.NounMusicVideo, q.Err, kit.DetailErrorOpts{
					Retry:    func() { v.env.Queries.Invalidate(keyVideo(id)) },
					Retrying: q.Fetching,
					Back:     back,
				})
			default:
				kit.DetailSkeleton(c, false)
			}
		})
	})
}

// hasRoute is Env.HasRoute, false in a hand-built Env.
func (v *video) hasRoute(path string) bool { return v.env.HasRoute != nil && v.env.HasRoute(path) }

// body is the loaded page: the header, the notes under it, the shelves and
// the release date.
func (v *video) body(c *ui.Context, d services.CatalogVideo) {
	vid := d.Video
	caps, known := v.caps()
	videoPlays := known && caps.Video
	v.pageHeader(c, d, videoPlays)

	note := ui.RichText(c).MaxWidth(68*8).Margin(-8, 0, 8, 0).TextColor(theme.C("ink-muted"))
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
			ui.Text(c.Key("gap"), " ")
			v.songLine(c, *d.Song)
		}
	})

	if d.Notes != "" {
		kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Notes")}, func() { kit.Notes(c, d.Notes, 0) })
	}
	var seeAll func()
	if p := "/artist/" + vid.ArtistID + "/view/music-videos"; vid.ArtistID != "" && v.hasRoute(p) {
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
	if path := "/song/" + id; v.hasRoute(path) {
		ui.Link(c.Key("song"), s.Name, path).TextColor(theme.C("ink"))
	} else {
		ui.Text(c.Key("song"), s.Name).TextColor(theme.C("ink"))
	}
	ui.Text(c.Key("song-after"), after)
}

// header is the collection header: the 16:9 art, the title, the artists, the
// catalogue line, the badges and the actions.
func (v *video) pageHeader(c *ui.Context, d services.CatalogVideo, videoPlays bool) {
	vid := d.Video
	parts := []string{i18n.T("Music video")}
	for _, p := range []string{yearOf(vid.ReleaseDate), i18n.FormatDuration(time.Duration(vid.DurationMs) * time.Millisecond), vid.Genre} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	var formats []string
	if vid.Has4K {
		formats = append(formats, format4K)
	}
	if vid.HasHDR {
		formats = append(formats, formatHDR)
	}
	h := d.Header
	color := ""
	if len(h.Colors) > 0 {
		color = h.Colors[0]
	}
	kit.CollectionHeader(c, kit.CollectionHeaderOpts{
		Title: vid.Name, Art: vid.Art.Large, Loader: v.env.Art, Video: true,
		Wash:   &kit.BackdropOpts{Src: h.Backdrop, Color: color, Tone: kit.ToneGround},
		Byline: func() { kit.ArtistLinks(c, d.Artists, vid.Artist, ui.Color{}) },
		Meta:   strings.Join(parts, " · "),
		Badges: func() { kit.Badges(c, kit.BadgesOpts{Explicit: vid.Explicit, Extra: formats}) },
		Actions: func() {
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
		},
	})
}

// coverOpts are the shelves' tile options: 16:9 tiles, one menu for the page.
func (v *video) coverOpts() kit.CoverOpts {
	return kit.CoverOpts{Video: true, Art: v.env.Art, Motion: v.env.Motion, Menus: v.menus, NoPopup: true}
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
