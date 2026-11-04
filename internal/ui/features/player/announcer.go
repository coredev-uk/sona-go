package player

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// announcer is the track announcer's state (v0.2.0's TrackAnnouncer).
type announcer struct {
	// seen is the ID of the last track announced.
	seen string
}

// announcedTrack is the track a track change should announce, or nil. seen
// is the ID of the last track announced. Only a different track that is
// playing counts: not while it loads, because what the snapshot says about
// the track (that it plays as a preview) is only sure once the engine has
// reported on it. The track restored paused at launch stays quiet until it
// plays, and play/pause, seeking and repeats of the same track stay quiet.
func announcedTrack(seen string, s services.PlaybackState) *services.QueueItem {
	cur := s.Current
	if cur == nil || cur.ID == seen || s.State != store.StatePlaying {
		return nil
	}
	return cur
}

// announceTrack tells screen readers, politely and once, that a new track
// plays: track changes only, never play/pause or seeking, and it says when
// the song plays as its 30-second preview. Call it every frame.
func (p *player) announceTrack(c *ui.Context, s services.PlaybackState) {
	t := announcedTrack(p.announce.seen, s)
	if t == nil {
		return
	}
	p.announce.seen = t.ID
	c.Announce(trackAnnouncement(t.Name, t.Artist, s.Preview))
}

// trackAnnouncement is what is said when a song starts.
func trackAnnouncement(name, artist string, preview bool) string {
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
