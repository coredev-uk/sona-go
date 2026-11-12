package artist

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// Cover tiles for the card-level DTOs (v0.2.0's shared/lib/tiles.ts),
// through kit.CoverItemOf: each links to its item's page when the app has
// one. The subtitle says what tells neighbouring tiles apart: an album's year
// on an artist's own shelves ("year"), else its artist.

func albumItem(al services.Album, by string) services.Item {
	sub := al.Artist
	if by == "year" {
		sub = i18n.YearOf(al.ReleaseDate)
	}
	return services.Item{Kind: services.ItemAlbum, ID: al.ID, Name: al.Name, Subtitle: sub, Art: al.Art, Album: &al}
}

func playlistItem(p services.Playlist) services.Item {
	return services.Item{Kind: services.ItemPlaylist, ID: p.ID, Name: p.Name, Subtitle: p.Curator, Art: p.Art, Playlist: &p}
}

func artistItem(ar services.Artist) services.Item {
	return services.Item{Kind: services.ItemArtist, ID: ar.ID, Name: ar.Name, Art: ar.Art, Artist: &ar}
}

// videoItem links to the video's catalogue page (a library copy's catalogue
// ID).
func videoItem(v services.MusicVideo, by string) services.Item {
	sub := v.Artist
	if by == "year" {
		sub = i18n.YearOf(v.ReleaseDate)
	}
	id := v.CatalogID
	if id == "" {
		id = v.ID
	}
	return services.Item{Kind: services.ItemMusicVideo, ID: id, Name: v.Name, Subtitle: sub, Art: v.Art, Explicit: v.Explicit, MusicVideo: &v}
}

// tile is an item as a cover tile; one whose page the app does not have is
// not a link.
func (a *artist) tile(ctx kit.ItemsCtx, it services.Item, i int) kit.CoverItem {
	t := kit.CoverItemOf(it, ctx, i)
	t.Explicit = it.Explicit
	if !a.env.Routed(kit.ItemLink(it)) {
		t.Open = nil
	}
	return t
}

func (a *artist) albumTiles(albums []services.Album, by string) []kit.CoverItem {
	out := make([]kit.CoverItem, len(albums))
	ctx := a.itemsCtx(nil)
	for i, al := range albums {
		out[i] = a.tile(ctx, albumItem(al, by), i)
	}
	return out
}

// shelfTiles are a shelf's tiles: the artist's own albums by year, the rest
// by name.
func (a *artist) shelfTiles(sh services.ArtistShelf) []kit.CoverItem {
	var items []services.Item
	switch sh.Kind {
	case services.ShelfAlbums:
		by := "year"
		if sh.View == services.ViewAppearsOn {
			by = "artist"
		}
		for _, al := range sh.Albums {
			items = append(items, albumItem(al, by))
		}
	case services.ShelfPlaylists:
		for _, p := range sh.Playlists {
			items = append(items, playlistItem(p))
		}
	case services.ShelfVideos:
		for _, v := range sh.Videos {
			items = append(items, videoItem(v, "year"))
		}
	case services.ShelfArtists:
		for _, ar := range sh.Artists {
			items = append(items, artistItem(ar))
		}
	}
	return a.tiles(items)
}

func (a *artist) tiles(items []services.Item) []kit.CoverItem {
	out := make([]kit.CoverItem, len(items))
	ctx := a.itemsCtx(nil)
	for i, it := range items {
		out[i] = a.tile(ctx, it, i)
	}
	return out
}

// itemsCtx is the shared item context over the Env, navigating as the page
// does and restricted as the account says.
func (a *artist) itemsCtx(menus *kit.ActionMenus) kit.ItemsCtx {
	ctx := a.env.ItemsCtx(menus)
	ctx.Nav = a.open
	ctx.Restricted = a.restricted()
	return ctx
}
