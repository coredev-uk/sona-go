package playlist

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// editTracks is an optimistic edit of a library playlist's tracks: shown
// at once, sent in order with the playlist's other edits, then replaced by
// Go's answer (or rolled back to what SQLite holds when it fails). Main
// thread only.
func (p *playlists) editTracks(id string, apply func(services.PlaylistDetail) services.PlaylistDetail, call func(ctx context.Context) (services.PlaylistDetail, error), what string) {
	q := p.env.Queries
	key := keyDetail(id)
	if prev := p.detail(id); prev.HasData {
		store.Set(q, key, apply(prev.Data))
	}
	p.edits.serial(id, func() (services.PlaylistDetail, error) { return call(context.Background()) },
		func(d services.PlaylistDetail, err error, _ bool) {
			p.env.Post(func() {
				if err == nil {
					// Later edits are still on their way: their optimistic
					// state stays. (Asked here, on the main thread, where
					// the edits made since the answer arrived are counted.)
					if !p.edits.editing(id) {
						store.Set(q, key, d)
					}
					return
				}
				if services.CodeOf(err) == services.CodeConflict {
					kit.ShowToast(kit.ToastSpec{
						Title:       i18n.T("This playlist changed on another device"),
						Description: i18n.T("Here is its latest version. Make the change again."),
					})
				} else {
					kit.ToastErrorf(what, failText(err))
				}
				// SQLite holds Apple's current order either way.
				q.Invalidate(key)
			})
		})
}

// moveTrack moves the track at from (the song the user saw there) to to.
func (p *playlists) moveTrack(id string, from, to int, song services.Song) {
	p.editTracks(id,
		func(d services.PlaylistDetail) services.PlaylistDetail { return withMove(d, from, to) },
		func(ctx context.Context) (services.PlaylistDetail, error) {
			return p.api.MoveTrack(ctx, id, from, to, song.ID)
		},
		i18n.T("Couldn't move the song"))
}

// removeTracks removes the tracks at indices (those copies only).
func (p *playlists) removeTracks(id string, indices []int, songs []services.Song) {
	ids := make([]string, len(songs))
	for i, s := range songs {
		ids[i] = s.ID
	}
	what := i18n.T("Couldn't remove the songs")
	if len(songs) == 1 {
		what = i18n.T("Couldn't remove “{name}”", "name", songs[0].Name)
	}
	p.editTracks(id,
		func(d services.PlaylistDetail) services.PlaylistDetail { return withRemoved(d, indices) },
		func(ctx context.Context) (services.PlaylistDetail, error) {
			return p.api.RemoveTracks(ctx, id, append([]int(nil), indices...), ids)
		},
		what)
}
