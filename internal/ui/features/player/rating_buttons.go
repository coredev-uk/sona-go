package player

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// ratingService is what the playing song's Favourite asks of the library
// (*services.LibraryService).
type ratingService interface {
	States(ctx context.Context, refs []services.ItemRef) ([]services.ItemState, error)
	Favorite(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	SuggestLess(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
}

// barExtras is the bar's parts that are not the transport: the zero value
// works (no rating buttons without a service).
type barExtras struct {
	// rate is the library service; initBar takes it from the Core, tests set
	// it.
	rate ratingService
}

// initBar is the bar's start-up (called by init, main thread): the library
// service for the rating buttons. A hand-built Env has no Core.
func (p *player) initBar() {
	if p.env.Core != nil && p.extras.rate == nil {
		p.extras.rate = services.Service[services.LibraryService](p.env.Core)
	}
}

// ratable reports whether the playing item has a song to rate: not live
// radio, a music video or a song outside Apple Music.
func ratable(cur *services.QueueItem) bool {
	return cur != nil && !cur.Live && !cur.Video && (cur.CatalogID != "" || cur.LibraryID != "")
}

// songRef is the library's name for the playing song.
func songRef(cur *services.QueueItem) services.ItemRef {
	return services.ItemRef{Kind: "songs", ID: cur.ID, Source: cur.Source, CatalogID: cur.CatalogID, LibraryID: cur.LibraryID}
}

// stateKey is the song's item-state query (v0.2.0's keys.itemState).
func stateKey(cur *services.QueueItem) store.Key {
	return store.Key{"library", "state", "songs", cur.ID}
}

// ratingButtons are the playing song's Favourite (and, unless compact,
// Suggest Less), built into the current parent; nothing for what has no
// song to rate. The state is the shared item-state query: a change shows at
// once and Go's answer replaces it.
func (p *player) ratingButtons(c *ui.Context, cur *services.QueueItem, compact bool) {
	svc := p.extras.rate
	if !ratable(cur) || svc == nil {
		return
	}
	ref := songRef(cur)
	res := store.Use(p.env.Queries, stateKey(cur), func(ctx context.Context) (services.ItemState, error) {
		st, err := svc.States(ctx, []services.ItemRef{ref})
		if err != nil {
			return services.ItemState{}, err
		}
		if len(st) == 0 {
			return services.ItemState{}, errors.New("no item state")
		}
		return st[0], nil
	}, store.Options{Stale: time.Minute})
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
		action := kit.RatingButtons(c, kit.RatingOpts{
			Name: cur.Name, Rating: res.Data.Rating, Pending: !res.HasData, Compact: compact, HideSuggestLess: compact,
		})
		if action != kit.RateNone {
			p.rateSong(cur, res.Data, res.HasData, action)
		}
	})
}

// rateSong changes the playing song's rating: the new state shows at once,
// Go's answer replaces it, and a failure restores the old one and says so.
func (p *player) rateSong(cur *services.QueueItem, prev services.ItemState, known bool, action kit.RatingAction) {
	if !known {
		prev = services.ItemState{Kind: "songs", ID: cur.ID, InLibrary: cur.Source == services.SourceLibrary, LibraryID: cur.LibraryID}
	}
	item := *cur
	p.changeRating(&item, prev, action)
}

// changeRating is rateSong's work, shared with the F shortcut.
func (p *player) changeRating(cur *services.QueueItem, prev services.ItemState, action kit.RatingAction) {
	svc := p.extras.rate
	if svc == nil {
		return
	}
	next := prev
	switch action {
	case kit.RateFavorite:
		next.Rating = 1
	case kit.RateSuggestLess:
		next.Rating = -1
	default:
		next.Rating = 0
	}
	key, ref := stateKey(cur), songRef(cur)
	store.Set(p.env.Queries, key, next)
	p.run(func() string { return ratingFailure(action, cur.Name) }, func(ctx context.Context) error {
		var st services.ItemState
		var err error
		switch action {
		case kit.RateFavorite, kit.RateUnfavorite:
			st, err = svc.Favorite(ctx, ref, action == kit.RateFavorite)
		default:
			st, err = svc.SuggestLess(ctx, ref, action == kit.RateSuggestLess)
		}
		if err != nil {
			store.Set(p.env.Queries, key, prev)
			return err
		}
		store.Set(p.env.Queries, key, st)
		p.env.Queries.Invalidate(store.Key{"library", "favorite-songs"})
		return nil
	})
}

// ratingFailure is the toast title of a rating that failed (v0.2.0's
// library-actions.ts).
func ratingFailure(action kit.RatingAction, name string) string {
	subject := i18n.T("“{name}”", "name", name)
	switch action {
	case kit.RateFavorite:
		return i18n.T("Couldn't favourite {subject}", "subject", subject)
	case kit.RateUnfavorite:
		return i18n.T("Couldn't undo favourite for {subject}", "subject", subject)
	case kit.RateSuggestLess:
		return i18n.T("Couldn't suggest less of {subject}", "subject", subject)
	}
	return i18n.T("Couldn't undo suggest less for {subject}", "subject", subject)
}

// toggleFavourite favourites the playing song, or undoes it (v0.2.0's
// toggleFavourite): not live radio, music videos or songs outside Apple
// Music. It reads the state first, off the main thread.
func (p *player) toggleFavourite() {
	cur := p.env.Play.State().Current
	svc := p.extras.rate
	if !ratable(cur) || svc == nil {
		return
	}
	item := *cur
	ref := songRef(&item)
	go func() {
		sts, err := svc.States(context.Background(), []services.ItemRef{ref})
		if err == nil && len(sts) == 0 {
			err = errors.New("no item state")
		}
		if err != nil {
			p.fail(ratingFailure(kit.RateFavorite, item.Name), err)
			return
		}
		st := sts[0]
		p.env.Post(func() {
			action := kit.RateFavorite
			if st.Rating == 1 {
				action = kit.RateUnfavorite
			}
			p.changeRating(&item, st, action)
		})
	}()
}
