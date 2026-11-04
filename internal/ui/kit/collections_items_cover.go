package kit

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// ItemsCtx is what the shared item components (ItemShelf, ItemGrid,
// EditorialSections, WideRow, LinkList, StationList, TrackShelf) need from the
// app and the feature building them: v0.2.0's router, browser, play registry
// and now-playing context. Build it once per frame (it holds no state); the
// funcs run on the main thread.
type ItemsCtx struct {
	// Nav opens a Sona page by its route path ("/album/{id}").
	Nav func(path string)
	// HasRoute reports whether a Sona page is registered (Env.HasRoute): a
	// tile whose page is not, builds without Open instead of sending the
	// user to the start page. Nil: every page is taken to exist.
	HasRoute func(path string) bool
	// OpenURL opens a web page in the system browser (a web-only item).
	OpenURL func(url string)
	// Players plays items, song lists and videos.
	Players *ItemPlayers
	// OnOpen runs whenever an item is opened (search remembers opened
	// items); for a page-less item it runs before OpenWithoutPage.
	OnOpen func(services.Item)
	// Restricted: the account may not play explicit items.
	Restricted bool
	// Art decodes the covers at the exact size they show.
	Art *ArtLoader
	// Menus is the item menu of the page; nil: no menus.
	Menus *ActionMenus
	// NoPopup: the page draws Menus.Popup itself, once, as several lists
	// share one ActionMenus (EditorialSections sets it for what it holds and
	// draws the popup after the last).
	NoPopup bool
	// Motion decides whether motion covers and the equaliser move; nil:
	// they stay still.
	Motion *MotionPolicy

	// What is playing (the feature supplies it from store.Playback):
	// Current reports whether a song is the one playing, CurrentStation the
	// same for a station; Playing is whether it plays or is paused; OnAir
	// is what the playing station's stream names; Toggle pauses or resumes.
	Current        func(services.Song) bool
	CurrentStation func(services.Station) bool
	Playing        bool
	OnAir          *services.OnAir
	Toggle         func()
}

func (x *ItemsCtx) nav(path string) {
	if x.Nav != nil && path != "" {
		x.Nav(path)
	}
}

func (x *ItemsCtx) opened(it services.Item) {
	if x.OnOpen != nil {
		x.OnOpen(it)
	}
}

// coverOpts are the cover collections' options from the context.
func (x *ItemsCtx) coverOpts(label string, min float32, video bool) CoverOpts {
	return CoverOpts{
		Label: label, Min: min, Video: video, Art: x.Art, Motion: x.Motion, Menus: x.Menus,
		NoPopup: x.NoPopup, Restricted: x.Restricted,
	}
}

// OpenWithoutPage opens an item that has no Sona page (ItemLink is ""): a
// station with nothing to show on a page plays; a web-only item (an interview,
// a concert listing) opens in the browser.
func OpenWithoutPage(it services.Item, ctx ItemsCtx) {
	switch {
	case PlaysWhenOpened(it, ctx.Players):
		ctx.Players.PlayItem(it)
	case it.URL != "" && ctx.OpenURL != nil:
		ctx.OpenURL(it.URL)
	}
}

// PlaysWhenOpened is true for an item that plays when it is opened: a station
// with no page.
func PlaysWhenOpened(it services.Item, p *ItemPlayers) bool {
	return ItemLink(it) == "" && it.Station != nil && p.CanPlay(it)
}

// CoverItemOf is an Item as a cover tile (v0.2.0's toCoverItem): its page, a
// play control when it plays directly, an upcoming album's release date, and
// its "more" target. index is the item's place in its list, for the tile's
// key. ctx.OnOpen runs whenever the tile is opened.
func CoverItemOf(it services.Item, ctx ItemsCtx, index int) CoverItem {
	link := ItemLink(it)
	id := it.ID
	if id == "" {
		id = it.URL
	}
	art := it.Art.Medium
	if art == "" {
		art = it.Wide.Medium
	}
	tile := CoverItem{
		Key:      strconv.Itoa(index) + ":" + it.Kind + ":" + id,
		Title:    it.Name,
		Subtitle: ItemSubtitle(it),
		Art:      art,
		Motion:   itemMotion(it),
		Round:    ItemRound(it),
		Video:    ItemVideo(it),
	}
	if link != "" {
		if ctx.HasRoute == nil || ctx.HasRoute(link) {
			tile.Open = func() {
				ctx.opened(it)
				ctx.nav(link)
			}
		}
	} else {
		tile.Open = func() {
			ctx.opened(it)
			OpenWithoutPage(it, ctx)
		}
	}
	// A station with no page plays when opened: its tile names that and
	// shows the play mark, but has no second play control.
	opens := PlaysWhenOpened(it, ctx.Players)
	tile.PlaysOnOpen = opens
	if !opens && ctx.Players.CanPlay(it) {
		tile.Play = &CoverPlay{Label: it.Name, Run: func() { ctx.Players.PlayItem(it) }, RunFrom: func(h *ui.Handle) { ctx.Players.PlayItemFrom(it, h) }}
	}
	if it.Album != nil {
		tile.Badge = itemsReleaseBadge(*it.Album, ctx.Players.now())
	}
	if target, ok := ActionItemOf(it, ctx.Players); ok {
		tile.Item = &target
	}
	return tile
}

// itemMotion is the motion artwork of the album, playlist, artist or station
// behind an item ("" for none).
func itemMotion(it services.Item) string {
	switch {
	case it.Album != nil:
		return it.Album.Motion
	case it.Playlist != nil:
		return it.Playlist.Motion
	case it.Artist != nil:
		return it.Artist.Motion
	case it.Station != nil:
		return it.Station.Motion
	}
	return ""
}

// IsUpcoming is v0.2.0's isUpcoming: an album is still to come when it is
// prerelease and has no full release date, or that date is ahead.
func IsUpcoming(a services.Album, now time.Time) bool {
	return a.Prerelease && (a.ReleaseAt == 0 || now.UnixMilli() < a.ReleaseAt)
}

// itemsReleaseBadge is the badge on an upcoming album's cover ("Out 19 Nov",
// with the year when it is not this one's); "" for an album that is out or
// has no full date (v0.2.0's releaseBadge).
func itemsReleaseBadge(a services.Album, now time.Time) string {
	if !IsUpcoming(a, now) || a.ReleaseAt == 0 {
		return ""
	}
	date := i18n.ShortReleaseDate(time.UnixMilli(a.ReleaseAt), now)
	return i18n.T("Out {releaseDate}", "releaseDate", date)
}
