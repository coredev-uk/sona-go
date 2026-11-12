package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// favouritePage is the Favourite songs page's view state.
type favouritePage struct {
	kit.PageFrame
	list songList
}

// favouriteList is the favourite songs in pages of 200 (key `["library",
// "favorite-songs", page]`).
func (l *library) favouriteList() *listView[services.Song] {
	return usePaged(l, &l.favourite.list.pager, func(n int) store.Key {
		return append(append(store.Key(nil), store.KeyLibraryFavourites...), n)
	}, func(ctx context.Context, off, lim int) (listPage[services.Song], error) {
		p, err := l.lib.FavoriteSongs(ctx, off, lim)
		return listPage[services.Song]{p.Items, p.Total}, err
	})
}

// favouritePage is FavoriteSongsPage.
func (l *library) favouritePage(c *ui.Context, r *ui.Route) {
	pg := &l.favourite
	list := l.favouriteList()
	auth := l.authStatus()

	subline := ""
	if list.Total > 0 {
		subline = i18n.SongCount(list.Total)
	}
	from := services.PlayFrom{Kind: "", ID: i18n.T("Favourite songs")}
	pg.Scroll(c, "favourites", func(pane *kit.Pane, _ float32) {
		l.pageHead(c, headOpts{title: i18n.T("Favourite songs"), count: subline, actions: func() {
			play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{
				PlayLabel: i18n.T("Play your favourite songs"), Shuffle: true, Disabled: list.Total == 0,
			})
			if play {
				l.env.Queue.PlaySongs(loadedSongs(list.Loaded()), 0, from)
			}
			if shuffle {
				l.env.Queue.ShuffleSongs(loadedSongs(list.Loaded()), from)
			}
		}})
		switch {
		case list.Err() != nil:
			kit.ErrorBlock(c, i18n.T("Couldn't read your favourite songs from this computer"), list.Err(),
				kit.ErrorBlockOpts{OnRetry: list.Retry, Retrying: list.Retrying()})
		case !list.Pending() && list.Total == 0 && auth.HasData && account.IsSignedOut(auth.Data.State):
			kit.EmptyState(c, i18n.T("You need to sign in to see your favourite songs"),
				i18n.T("Songs you favourite in Sona or any Apple Music app are collected here."),
				kit.EmptyOpts{Frame: kit.FrameBox, Action: l.signIn()})
		case !list.Pending() && list.Total == 0:
			kit.EmptyState(c, i18n.T("No favourite songs yet"),
				i18n.T("Choose Favourite on any song, here or in Apple Music, and it is kept here and in your library."),
				kit.EmptyOpts{Frame: kit.FrameBox})
		default:
			l.songTable(c, &pg.list, pane, list, songTableOpts{
				label: i18n.T("Favourite songs"),
				onPlay: func(i int) {
					l.env.Queue.PlaySongs(loadedSongs(list.Loaded()), i, from)
				},
			})
		}
	})
}
