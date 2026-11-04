package album

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// open is a tile's way to a page: a navigation where the page is ported,
// else nil (the tile is not a link).
func (a *album) open(path string) func() {
	if !a.has(path) {
		return nil
	}
	return func() { a.push(path) }
}

// albumTile is an album as a cover tile, linking to its page. The subtitle
// says what tells neighbouring tiles apart: an album's year on an artist's
// own shelves, else its artist. An album not out yet carries its release date
// on the cover.
func (a *album) albumTile(al services.Album, byYear bool, now time.Time) kit.CoverItem {
	sub := al.Artist
	if byYear {
		sub = yearOf(al.ReleaseDate)
	}
	item := kit.AlbumItem(al)
	return kit.CoverItem{
		Key: al.ID, Title: al.Name, Subtitle: sub, Art: al.Art.Medium, Motion: al.Motion,
		Open: a.open("/album/" + al.ID), Item: &item, Badge: releaseBadge(al, now),
	}
}

func (a *album) playlistTile(p services.Playlist) kit.CoverItem {
	item := kit.PlaylistItem(p)
	return kit.CoverItem{
		Key: p.ID, Title: p.Name, Subtitle: p.Curator, Art: p.Art.Medium, Motion: p.Motion,
		Open: a.open("/playlist/" + p.ID), Item: &item,
	}
}

// videoTile is a music video's tile: a link to its page. (Its play control and
// queue actions come with the video module's Env hook.)
func (a *album) videoTile(v services.MusicVideo) kit.CoverItem {
	id := v.CatalogID
	if id == "" {
		id = v.ID
	}
	t := kit.CoverItem{
		Key: v.ID, Title: v.Name, Subtitle: yearOf(v.ReleaseDate), Art: v.Art.Medium, Video: true, Explicit: v.Explicit,
		Open: a.open("/music-video/" + id),
	}
	return t
}

// episodeTile is a radio episode about the album as a cover tile: its page, a
// play control, its menu.
func (a *album) episodeTile(s services.Station) kit.CoverItem {
	sub := s.ShowName
	if sub == "" {
		sub = s.Tagline
	}
	item := kit.StationItem(s)
	t := kit.CoverItem{
		Key: s.ID, Title: s.Name, Subtitle: sub, Art: s.Art.Medium, Motion: s.Motion,
		Open: a.open("/station/" + s.ID), Item: &item,
	}
	if s.Playable {
		id := s.ID
		t.Play = &kit.CoverPlay{Label: s.Name, Run: func() {
			a.queue.Replace(i18n.T("Couldn't play this station"), func(ctx context.Context) error {
				return a.radio.Play(ctx, id)
			}, nil)
		}}
	}
	return t
}

// sections are Apple's sections for the album, in its order; its radio
// episodes after its videos.
func (a *album) sections(c *ui.Context, m *model, now time.Time, restricted bool) {
	page, al := m.page, m.album
	albums := func(list []services.Album, byYear bool) []kit.CoverItem {
		out := make([]kit.CoverItem, len(list))
		for i, x := range list {
			out[i] = a.albumTile(x, byYear, now)
		}
		return out
	}
	playlists := make([]kit.CoverItem, len(page.AppearsOn))
	for i, p := range page.AppearsOn {
		playlists[i] = a.playlistTile(p)
	}
	videos := make([]kit.CoverItem, len(page.Videos))
	for i, v := range page.Videos {
		videos[i] = a.videoTile(v)
	}
	episodes := make([]kit.CoverItem, len(page.MoreToHear))
	for i, s := range page.MoreToHear {
		episodes[i] = a.episodeTile(s)
	}
	var more func()
	if al.ArtistID != "" && a.has("/artist/"+al.ArtistID+"/view/"+services.ViewFullAlbums) {
		more = func() { a.push("/artist/" + al.ArtistID + "/view/" + services.ViewFullAlbums) }
	}
	type shelf struct {
		title string
		items []kit.CoverItem
		video bool
		more  func()
	}
	shelves := []shelf{
		{i18n.T("Other versions"), albums(page.OtherVersions, true), false, nil},
		{i18n.T("Music videos"), videos, true, nil},
		{i18n.T("More to hear"), episodes, false, nil},
		{i18n.T("More by {artistName}", "artistName", al.Artist), albums(page.MoreByArtist, true), false, more},
		{i18n.T("Featured in playlists"), playlists, false, nil},
		{i18n.T("You might also like"), albums(page.Similar, false), false, nil},
	}
	first := true
	for i, s := range shelves {
		kit.CoverShelf(c.Key(i), &a.shelves[i], s.title, s.items, kit.CoverShelfOpts{
			CoverOpts: kit.CoverOpts{
				Label: s.title, Video: s.video, Art: a.env.Art, Motion: a.env.Motion,
				Menus: a.menus, NoPopup: true, Restricted: restricted,
			},
			First: first && len(s.items) > 0,
			More:  s.more,
		})
		if len(s.items) > 0 {
			first = false
		}
	}
}
