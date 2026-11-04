// Package radio is the Radio feature: the Radio page (what is on air now on
// Apple Music's live stations as a now-and-next board, the user's own
// stations, Apple's editorial radio in bands), station pages, playing
// stations from any card, and Create Station on songs, albums and artists.
//
// Every service call runs off the main thread (store.Use fetches, goroutines
// handing back through Env.Post); a station that plays replaces the queue
// through Env.Queue, which asks or offers Undo over a long queue the user
// built.
package radio

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// The module's routes.
const (
	radioPath   = "/radio"
	stationPath = "/station/{id}"
)

// radioAPI is what the feature asks of RadioService
// (*services.RadioService).
type radioAPI interface {
	Grouping(ctx context.Context) (services.EditorialPage, error)
	Live(ctx context.Context) ([]services.Station, error)
	Recent(ctx context.Context) ([]services.Station, error)
	Station(ctx context.Context, id string) (services.StationDetail, error)
	Play(ctx context.Context, id string) error
	CreateStation(ctx context.Context, seed, catalogID string) (services.Station, error)
}

// transport is what the board asks of PlaybackService
// (*services.PlaybackService): a station that plays is paused and resumed
// from its row.
type transport interface {
	Toggle(ctx context.Context) error
}

// radio is the feature's state.
type radio struct {
	env *module.Env
	svc radioAPI
	pb  transport
	// now is the wall clock a schedule is read against; the pages read it
	// every 30 seconds (tests move it).
	now func() time.Time

	// lastStation is the station that last fed the queue, to notice a new
	// one.
	lastStation string

	radioPg   radioPage
	stationPg stationPage
}

// New is the radio module.
func New(env *module.Env) module.Module {
	core := env.Core
	pb := services.Service[services.PlaybackService](core)
	r := newRadio(env, services.Service[services.RadioService](core), pb)
	// A station anywhere (a tile, a shelf, an album's "More to hear") plays
	// through the shared item players.
	env.Players.Register(services.ItemStation, func(it services.Item) {
		id := it.ID
		if id == "" && it.Station != nil {
			id = it.Station.ID
		}
		r.playStation(id, nil)
	})
	return module.Module{
		ID: "radio",
		Routes: []module.Route{
			{Pattern: radioPath, Title: func(*ui.Route) string { return i18n.T("Radio") }, Build: r.radioView},
			{Pattern: stationPath, Title: func(*ui.Route) string { return i18n.T("Station") }, Build: r.stationView},
		},
		Nav: []module.NavItem{{
			Section: module.NavPrimary,
			Label:   func() string { return i18n.T("Radio") },
			Icon:    "radio",
			To:      radioPath,
			Order:   18,
		}},
		Actions: r.actions(),
		Init:    r.init,
	}
}

func newRadio(env *module.Env, svc radioAPI, pb transport) *radio {
	return &radio{env: env, svc: svc, pb: pb, now: time.Now}
}

// Query keys, as v0.2.0's features/radio/queries.ts: every key starts with
// "radio" so sign-in changes can drop them all.
var (
	keyGrouping = store.Key{"radio", "grouping"}
	keyLive     = store.Key{"radio", "live"}
	// keyRecent is Apple's history of the user's stations: user data.
	keyRecent = store.Key{"radio", "recent"}
)

func keyStation(id string) store.Key { return store.Key{"radio", "station", id} }

const (
	// editorialStale: editorial radio changes hourly at most; Go caches it
	// and serves it offline.
	editorialStale = 10 * time.Minute
	recentStale    = time.Minute
	stationStale   = 5 * time.Minute
)

func (r *radio) grouping() store.Result[services.EditorialPage] {
	return store.Use(r.env.Queries, keyGrouping, r.svc.Grouping, store.Options{Stale: editorialStale})
}

func (r *radio) live() store.Result[[]services.Station] {
	return store.Use(r.env.Queries, keyLive, r.svc.Live, store.Options{Stale: editorialStale})
}

// recent is the user's stations; it asks only when signed in.
func (r *radio) recent() store.Result[[]services.Station] {
	return store.Use(r.env.Queries, keyRecent, r.svc.Recent, store.Options{Stale: recentStale})
}

// station is a station page's data; a live station's schedule is read again
// when it is stale, and the Radio page's board shares it with each station's
// own page.
func (r *radio) station(id string) store.Result[services.StationDetail] {
	return store.Use(r.env.Queries, keyStation(id), func(ctx context.Context) (services.StationDetail, error) {
		return r.svc.Station(ctx, id)
	}, store.Options{Stale: stationStale})
}

// accountState is the sign-in state ("signed_in", "signed_out"…); "" until Go
// has answered.
func (r *radio) accountState() string {
	res := store.UseAuth(r.env.Queries, r.env.Core)
	if !res.HasData {
		return ""
	}
	return res.Data.State
}

// isSignedOut: known to be signed out; not while Go still reads the stored
// sign-in from the keyring (restoring), and not when Apple only asked to sign
// in again (needs_reauth: the library is still here).
func isSignedOut(state string) bool {
	return state != "" && state != "restoring" && state != "needs_reauth" && !account.HasAccount(state)
}

// init follows the station Go plays: Apple's history of the user's stations
// changes once a new station starts playing (v0.2.0's playback:state handler).
// The sign-in state the page reads is the shared ["auth"] query, which the
// feature that owns the account keeps current from "auth:changed".
func (r *radio) init() {
	store.On(r.env.Bus, services.EventPlaybackState, func(s services.PlaybackState) {
		id := ""
		switch {
		case s.Station != nil:
			id = s.Station.ID
		case s.Current != nil && s.Current.Live:
			id = s.Current.ID
		}
		if id != "" && id != r.lastStation {
			r.env.Queries.Invalidate(keyRecent)
		}
		r.lastStation = id
	})
}

// actions are the radio item actions: Play on a station, and Create Station
// on songs, albums and artists that have a catalogue ID (v0.2.0's index.ts).
func (r *radio) actions() []kit.ActionDef {
	create := func(id string, kind kit.ItemKind, seed string, of func(kit.Item) (catalogID, name string)) kit.ActionDef {
		return kit.ActionDef{
			ID: id, Target: kind, Icon: "radio", Group: kit.GroupQueue, Order: 30,
			Label: func(*kit.ActionContext) string { return i18n.T("Create station") },
			Available: func(a *kit.ActionContext) bool {
				catalogID, _ := of(a.Item())
				return catalogID != ""
			},
			Run: func(a *kit.ActionContext) {
				catalogID, name := of(a.Item())
				r.createStation(seed, catalogID, name)
			},
		}
	}
	return []kit.ActionDef{
		{
			ID: "radio.play-station", Target: kit.KindStation, Icon: "play", Group: kit.GroupQueue, Order: 10,
			Label:   func(*kit.ActionContext) string { return i18n.T("Play") },
			Enabled: func(a *kit.ActionContext) bool { return a.Item().Station().Playable },
			Run:     func(a *kit.ActionContext) { r.playStation(a.Item().Station().ID, nil) },
		},
		create("radio.create-station", kit.KindSong, seedSong, func(i kit.Item) (string, string) {
			s := i.Song()
			return s.CatalogID, s.Name
		}),
		create("radio.create-station-album", kit.KindAlbum, seedAlbum, func(i kit.Item) (string, string) {
			a := i.Album()
			return a.CatalogID, a.Name
		}),
		create("radio.create-station-artist", kit.KindArtist, seedArtist, func(i kit.Item) (string, string) {
			a := i.Artist()
			return a.CatalogID, a.Name
		}),
	}
}
