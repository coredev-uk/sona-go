package concerts

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// concertsAPI is what the pages ask of ConcertsService.
type concertsAPI interface {
	Concerts(ctx context.Context, req services.ConcertsRequest) (services.ConcertsPage, error)
	ForArtist(ctx context.Context, id string) (services.ArtistConcerts, error)
	Location() services.ConcertLocation
	Places(query string) []services.ConcertPlace
	SetLocation(id string) (services.ConcertLocation, error)
}

// pageSize is how many concerts a listing asks for at first, and for each
// "Show more concerts".
const pageSize = 100

// staleListing: listings change as shows are announced, not by the minute;
// Go caches them for hours.
const staleListing = 10 * time.Minute

// Query keys, as v0.2.0's queries.ts: every key starts with "concerts" so a
// sign-in or a location change drops them all.
var (
	keyConcerts = store.Key{"concerts"}
	keyLocation = store.Key{"concerts", "location"}
)

// data is the concerts queries over the Env: the Concerts page and the
// shelves other pages draw share them.
type data struct {
	env *module.Env
	api concertsAPI
}

// dataOf is the queries over the real service.
func dataOf(env *module.Env) *data {
	return &data{env: env, api: services.Service[services.ConcertsService](env.Core)}
}

// requestOf is the listing's request: the date range, the genre and how
// many to ask for.
func requestOf(r dayRange, hasRange bool, genre string, limit int) services.ConcertsRequest {
	req := services.ConcertsRequest{Genre: genre, Limit: limit}
	if hasRange {
		req.From, req.To = r.from, r.to
	}
	return req
}

// concerts is the concerts near the chosen place. The caller keeps the
// list on screen while a changed filter loads (TanStack's keepPreviousData).
func (d *data) concerts(req services.ConcertsRequest) store.Result[services.ConcertsPage] {
	return store.Use(d.env.Queries, store.Key{"concerts", "page", req},
		func(ctx context.Context) (services.ConcertsPage, error) { return d.api.Concerts(ctx, req) },
		store.Options{Stale: staleListing})
}

// artist is an artist's upcoming concerts.
func (d *data) artist(id string) store.Result[services.ArtistConcerts] {
	return store.Use(d.env.Queries, store.Key{"concerts", "artist", id},
		func(ctx context.Context) (services.ArtistConcerts, error) { return d.api.ForArtist(ctx, id) },
		store.Options{Stale: staleListing})
}

// location is where concerts are looked for now (also carried by every page).
func (d *data) location() store.Result[services.ConcertLocation] {
	return store.Use(d.env.Queries, keyLocation,
		func(context.Context) (services.ConcertLocation, error) { return d.api.Location(), nil },
		store.Options{Stale: store.Forever})
}

// places are the cities for the picker: the storefront's own and a few
// well-known when nothing is typed.
func (d *data) places(query string) store.Result[[]services.ConcertPlace] {
	return store.Use(d.env.Queries, store.Key{"concerts", "places", query},
		func(context.Context) ([]services.ConcertPlace, error) { return d.api.Places(query), nil },
		store.Options{Stale: store.Forever})
}

// setLocation chooses the city ("" the storefront's main one) off the main
// thread; every listing is asked again. A failure toasts; done says which.
func (d *data) setLocation(id string, done func(ok bool)) {
	go func() {
		loc, err := d.api.SetLocation(id)
		d.env.Post(func() {
			if err != nil {
				kit.ToastFailure(i18n.T("Couldn't save your city"), err)
			} else {
				store.Set(d.env.Queries, keyLocation, loc)
				d.env.Queries.Invalidate(keyConcerts)
			}
			done(err == nil)
		})
	}()
}
