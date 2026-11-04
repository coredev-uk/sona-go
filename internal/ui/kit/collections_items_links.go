package kit

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
)

// This file is v0.2.0's shared/items/links.ts and linkGrid.ts: where an
// item leads, how it is drawn and what its actions are. Routes are the
// router's path strings ("/album/{id}"), what ItemsCtx.Nav takes.

// ItemAspect is the shape of a row of covers: square, or 16:9 video frames.
type ItemAspect uint8

const (
	// AspectSquare is a square cover.
	AspectSquare ItemAspect = iota
	// AspectVideo is a 16:9 frame (music videos).
	AspectVideo
)

// itemsPage is the route of the page of the thing with an ID: prefix is the
// route's start, "/album/".
func itemsPage(prefix, id string) string { return prefix + url.PathEscape(id) }

// ItemLink is the Sona page an item opens, by kind, as a route path ("" for
// items that have none: those that exist only on the web (kind "link",
// opened with OpenWithoutPage) and stations with nothing to show on a page (a
// local broadcaster has no schedule), which play when they are opened). A
// charts item carries its genre as a query ("/charts?genre=14"); a concerts
// item opens the in-app page.
func ItemLink(it services.Item) string {
	page := func(prefix string) string { return itemsPage(prefix, it.ID) }
	switch it.Kind {
	case services.ItemAlbum:
		return page("/album/")
	case services.ItemPlaylist:
		return page("/playlist/")
	case services.ItemArtist:
		return page("/artist/")
	case services.ItemSong:
		return page("/song/")
	case services.ItemStation:
		if it.Station != nil && !it.Station.HasPage {
			return ""
		}
		return page("/station/")
	case services.ItemMusicVideo:
		return page("/music-video/")
	case services.ItemCurator:
		return page("/curator/")
	case services.ItemRoom:
		return page("/room/")
	case services.ItemMultiroom:
		return page("/multiroom/")
	case services.ItemGrouping:
		return page("/grouping/")
	case services.ItemCharts:
		if it.ID != "" {
			return "/charts?genre=" + url.QueryEscape(it.ID)
		}
		return "/charts"
	case services.ItemProfile:
		return page("/profile/")
	case services.ItemConcerts:
		return "/concerts"
	}
	return ""
}

// ItemRound reports whether an item's cover is drawn as a circle: artists
// and profiles are; everything else is square.
func ItemRound(it services.Item) bool {
	return it.Kind == services.ItemArtist || it.Kind == services.ItemProfile
}

// videoURL matches the web pages of videos Sona cannot open: posts and music
// movies.
var videoURL = regexp.MustCompile(`/(post|music-movie)/`)

// ItemVideo reports whether an item's cover is a 16:9 frame: music videos,
// and link items to a post or music movie that have art.
func ItemVideo(it services.Item) bool {
	return it.Kind == services.ItemMusicVideo ||
		(it.Kind == services.ItemLink && it.Art.Medium != "" && videoURL.MatchString(it.URL))
}

// RowAspect is the tiles' shared aspect: video when the list is not empty and
// every item is a video frame, else square.
func RowAspect(items []services.Item) ItemAspect {
	if len(items) == 0 {
		return AspectSquare
	}
	for i := range items {
		if !ItemVideo(items[i]) {
			return AspectSquare
		}
	}
	return AspectVideo
}

// ItemSubtitle is the line under an item's title: Apple's reason or badge,
// then the artist, curator or host ("New release · Radiohead"); empty parts
// are dropped.
func ItemSubtitle(it services.Item) string {
	switch {
	case it.Caption == "":
		return it.Subtitle
	case it.Subtitle == "":
		return it.Caption
	}
	return it.Caption + " · " + it.Subtitle
}

// RoomLink is a section's "See all": its editorial room as a route path, ""
// when the section has none.
func RoomLink(s services.Section) string {
	if s.Room == "" {
		return ""
	}
	return itemsPage("/room/", s.Room)
}

// ActionItemOf is what an Item is for actions, selection and drag; false
// for the kinds without actions. A music video counts only where videos
// play (p).
func ActionItemOf(it services.Item, p *ItemPlayers) (Item, bool) {
	switch {
	case it.Song != nil:
		return SongItem(*it.Song), true
	case it.Album != nil:
		return AlbumItem(*it.Album), true
	case it.Playlist != nil:
		return PlaylistItem(*it.Playlist), true
	case it.Station != nil:
		return StationItem(*it.Station), true
	case it.Artist != nil:
		return ArtistItem(*it.Artist), true
	case it.MusicVideo != nil && p.CanPlayVideo():
		return MusicVideoItem(*it.MusicVideo), true
	}
	return Item{}, false
}

// itemsLinkMin is the smallest a link button gets, in DIPs: room for "Moods
// and Activities" on one line, "From Around the World" on two. itemsLinkGap
// is the gap between link buttons, both ways.
const (
	itemsLinkMin = 184
	itemsLinkGap = 8
)

// itemsLinkGrid is how a list of count links sets in a row width DIPs wide
// (v0.2.0's linkGrid): as many columns as fit at itemsLinkMin, filled top to
// bottom, so an alphabetical list (Apple's genres) reads down each column and
// Tab follows the eye. cols is the tracks the row is cut into (a short list
// leaves the last ones empty, so its buttons stay as wide as the list's above
// or below it); rows is what the fullest column holds. The columns are filled
// so the last one holds at least half a column: a lone button stranded at the
// top of the last column reads as a mistake, so the column is made taller
// until it has company.
func itemsLinkGrid(width float32, count int) (cols, rows int) {
	cols = max(1, int((width+itemsLinkGap)/(itemsLinkMin+itemsLinkGap)))
	rows = max(1, (count+cols-1)/cols)
	for rows < count {
		last := count - ((count+rows-1)/rows-1)*rows
		if 2*last >= rows {
			break
		}
		rows++
	}
	return cols, rows
}

// itemsArtAspect is the width over the height of the artwork behind a Go art
// path, which ends in "<w>x<h>.jpg" ("<n>.jpg" means n by n): v0.2.0's
// artAspect. It is 1 when the path names no size.
func itemsArtAspect(path string) float32 {
	if q := strings.IndexByte(path, '?'); q >= 0 {
		path = path[:q]
	}
	name := path[strings.LastIndexByte(path, '/')+1:]
	name, ok := strings.CutSuffix(name, ".jpg")
	if !ok {
		return 1
	}
	ws, hs, two := strings.Cut(name, "x")
	w, err := strconv.Atoi(ws)
	if err != nil {
		return 1
	}
	h := w
	if two {
		if h, err = strconv.Atoi(hs); err != nil {
			return 1
		}
	}
	if w <= 0 || h <= 0 {
		return 1
	}
	return float32(w) / float32(h)
}
