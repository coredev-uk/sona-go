package radio

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// This file is v0.2.0's features/radio/commands.ts: playing a station and
// Create Station, with the station-specific words of their failures.

// stationFailure is what a failed station command tells the user (v0.2.0's
// `failed`): a sign-in the station needs, a station Apple does not have, else
// the words of the error's code.
func stationFailure(err error) string {
	switch code := services.CodeOf(err); code {
	case services.CodeNotSignedIn, services.CodeMUTRejected:
		return i18n.T("Sign in to Apple Music from the account menu in the top bar.")
	case services.CodeNotFound:
		return i18n.T("Apple Music has no station for this.")
	default:
		return i18n.ErrorText(code)
	}
}

// stationFailed toasts what a station command could not do, and why. Safe
// from any goroutine.
func stationFailed(what string, err error) {
	kit.ToastErrorf(what, stationFailure(err))
}

// playStation plays station id ("ra.…"): a track station fills the queue as
// it plays; a live station or episode plays as one stream. Either replaces the
// queue (with Undo, or a question first, over a long one the user built).
// done, when not nil, gets whether it started, on the main thread. Main
// thread only.
func (r *radio) playStation(id string, done func(started bool)) {
	r.env.Queue.Replace(i18n.T("Couldn't play this station"), func(ctx context.Context) error {
		return r.svc.Play(ctx, id)
	}, done, kit.WithFailure(func(err error) (string, string) {
		return i18n.T("Couldn't play this station"), stationFailure(err)
	}))
}

// Create Station seeds, as RadioService.CreateStation takes them.
const (
	seedSong   = services.SeedSong
	seedAlbum  = services.SeedAlbum
	seedArtist = services.SeedArtist
)

// createStation starts the station seeded by a catalogue song, artist or
// album and says so; a failure names the item it was made from.
func (r *radio) createStation(seed, catalogID, name string) {
	go func() {
		st, err := r.svc.CreateStation(context.Background(), seed, catalogID)
		if err != nil {
			stationFailed(i18n.T("Couldn't start a station from {name}", "name", name), err)
			return
		}
		kit.ShowToast(kit.ToastSpec{Title: i18n.T("Playing {station}", "station", st.Name)})
	}()
}
