package player

import (
	"context"
	"errors"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// ratable reports whether the playing item has a song to rate: not live
// radio, a music video or a song outside Apple Music.
func ratable(cur *services.QueueItem) bool {
	return cur != nil && !cur.Live && !cur.Video && (cur.CatalogID != "" || cur.LibraryID != "")
}

// stateItem is the library's name for the playing song.
func stateItem(cur *services.QueueItem) kit.StateItem {
	return kit.StateItem{ID: cur.ID, Source: cur.Source, CatalogID: cur.CatalogID, LibraryID: cur.LibraryID}
}

// ratingButtons are the playing song's Favourite (and, unless compact,
// Suggest Less), built into the current parent; nothing for what has no
// song to rate. The state is the shared item-state layer (Env.ItemStates):
// a change shows at once and Go's answer replaces it.
func (p *player) ratingButtons(c *ui.Context, cur *services.QueueItem, compact bool) {
	states := p.env.ItemStates
	if !ratable(cur) || states == nil {
		return
	}
	it := stateItem(cur)
	res := states.Use(kit.StateSongs, it)
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
		action := kit.RatingButtons(c, kit.RatingOpts{
			Name: cur.Name, Rating: res.Data.Rating, Pending: !res.HasData, Compact: compact, HideSuggestLess: compact,
		})
		states.Rate(kit.StateSongs, it, cur.Name, action)
	})
}

// toggleFavourite favourites the playing song, or undoes it (v0.2.0's
// toggleFavourite): not live radio, music videos or songs outside Apple
// Music. It reads the state first, off the main thread.
func (p *player) toggleFavourite() {
	cur := p.env.Play.State().Current
	states := p.env.ItemStates
	if !ratable(cur) || states == nil {
		return
	}
	name, it := cur.Name, stateItem(cur)
	go func() {
		st, ok := states.Load(context.Background(), kit.StateSongs, it)
		if !ok {
			p.fail(i18n.T("Couldn't favourite {subject}", "subject", i18n.T("“{name}”", "name", name)), errors.New("no item state"))
			return
		}
		p.env.Post(func() {
			action := kit.RateFavorite
			if st.Rating == 1 {
				action = kit.RateUnfavorite
			}
			states.Rate(kit.StateSongs, it, name, action)
		})
	}()
}
