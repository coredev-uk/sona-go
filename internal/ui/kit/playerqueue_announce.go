package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// TrackAnnouncer is the track announcer (v0.2.0's TrackAnnouncer): it tells
// screen readers, politely and once, that a new track plays. The player bar
// and the mini player each hold one, as only the window that is shown is read.
// The zero value is ready. Main thread only.
type TrackAnnouncer struct {
	// seen is the ID of the last track announced.
	seen string
}

// AnnouncedTrack is the track a track change should announce, or nil. seen is
// the ID of the last track announced. Only a different track that is playing
// counts: not while it loads, because what the snapshot says about the track
// (that it plays as a preview) is only sure once the engine has reported on
// it. The track restored paused at launch stays quiet until it plays, and
// play/pause, seeking and repeats of the same track stay quiet.
func AnnouncedTrack(seen string, s services.PlaybackState) *services.QueueItem {
	cur := s.Current
	if cur == nil || cur.ID == seen || s.State != store.StatePlaying {
		return nil
	}
	return cur
}

// Announce says that a new track plays: track changes only, never play/pause
// or seeking, and it says when the song plays as its 30-second preview. Call
// it every frame the window builds.
func (a *TrackAnnouncer) Announce(c *ui.Context, s services.PlaybackState) {
	t := AnnouncedTrack(a.seen, s)
	if t == nil {
		return
	}
	a.seen = t.ID
	c.Announce(TrackAnnouncement(t.Name, t.Artist, s.Preview))
}

// TrackAnnouncement is what is said when a song starts.
func TrackAnnouncement(name, artist string, preview bool) string {
	switch {
	case preview && artist != "":
		return i18n.T("Now playing a preview: {name} by {artist}", "name", name, "artist", artist)
	case preview:
		return i18n.T("Now playing a preview: {name}", "name", name)
	case artist != "":
		return i18n.T("Now playing: {name} by {artist}", "name", name, "artist", artist)
	}
	return i18n.T("Now playing: {name}", "name", name)
}
