// Package scrobble submits Sona's listening to Last.fm and ListenBrainz.
//
// A Tracker turns the coordinator's listening transitions into now-playing
// announcements and completed listens (the scrobble rules); a Scrobbler
// sends both to every connected provider, keeping undelivered listens in a
// persisted per-provider queue that it retries with backoff. The queue holds
// listen data only; credentials (the Last.fm session key, the ListenBrainz
// user token) are handed in by the caller, which keeps them in the keyring.
//
// Rules (Last.fm's, which ListenBrainz shares): a listen counts once it has
// been played for half the track or four minutes, whichever comes first,
// and only for tracks longer than 30 seconds. Played time is listening time:
// seeking forward skips the jumped-over part, seeking back replays it, and a
// pause followed by a resume of the same item continues the same listen.
// Live radio ("ra." items) and items without an artist or title are never
// announced or scrobbled.
package scrobble

import (
	"errors"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// Errors providers return; the Scrobbler acts on them.
var (
	// ErrAuth: the service refused the credentials. Delivery to that
	// provider stops (the queue is kept) until new credentials are set.
	ErrAuth = errors.New("scrobble: credentials rejected")
	// ErrRejected: the service refused the submission itself; it is
	// dropped so it cannot block the queue.
	ErrRejected = errors.New("scrobble: submission rejected")
	// ErrPending: the user has not approved the Last.fm token yet.
	ErrPending = errors.New("scrobble: authorisation pending")
	// ErrExpired: the Last.fm token expired before it was approved.
	ErrExpired = errors.New("scrobble: authorisation expired")
	// ErrNotConfigured: this build carries no Last.fm API account.
	ErrNotConfigured = errors.New("scrobble: Last.fm is not configured in this build")
)

// Rule limits.
const (
	MinDuration  = 30 * time.Second // tracks this short or shorter never scrobble
	MaxThreshold = 4 * time.Minute  // played time that always counts
)

// Threshold is the played time after which a track of duration d counts.
func Threshold(d time.Duration) time.Duration { return min(d/2, MaxThreshold) }

// Track is the metadata scrobblers take.
type Track struct {
	Artist   string        `json:"artist"`
	Title    string        `json:"title"`
	Album    string        `json:"album,omitempty"`
	Duration time.Duration `json:"duration"`
}

// Listen is a completed listen: the track and when it started playing.
type Listen struct {
	Track
	ListenedAt time.Time `json:"listenedAt"`
}

// trackOf returns the scrobbleable track of a, or nil for live radio and
// items without an artist or title.
func trackOf(a coordinator.Activity) *Track {
	it := a.Item
	if strings.HasPrefix(it.ID, "ra.") {
		return nil
	}
	t := Track{
		Artist: strings.TrimSpace(it.Artist),
		Title:  strings.TrimSpace(it.Title),
		Album:  strings.TrimSpace(it.Album),
	}
	if t.Artist == "" || t.Title == "" {
		return nil
	}
	t.Duration = a.Duration
	if t.Duration <= 0 {
		t.Duration = it.Duration
	}
	return &t
}

// Tracker applies the scrobble rules to coordinator activity. It is not
// safe for concurrent use.
type Tracker struct {
	cur *listening
	now func() time.Time // for activity without a timestamp; nil means time.Now
}

// listening is the open (playing or paused) listen.
type listening struct {
	id      string
	track   *Track // nil: not scrobbleable
	start   time.Time
	played  time.Duration
	from    time.Duration // position the current stretch of playing began at
	playing bool
	paused  bool // ended by a pause: a Start of the same item resumes it
	counted bool // already returned as a Listen
}

// Observe takes one transition. It returns the track to announce as now
// playing (on every start and resume) and the listen that has just counted
// (at most once per listen, when a stretch of playing ends).
func (t *Tracker) Observe(a coordinator.Activity) (now *Track, done *Listen) {
	c := t.cur
	switch a.Kind {
	case coordinator.ActivityStart:
		if c != nil && c.paused && c.id == a.Item.ID {
			c.playing, c.paused, c.from = true, false, a.Position
		} else {
			at := a.At
			if at.IsZero() {
				at = t.clock()
			}
			c = &listening{id: a.Item.ID, track: trackOf(a), start: at, from: a.Position, playing: true}
			t.cur = c
		}
		if c.track != nil {
			tr := *c.track
			now = &tr
		}
	case coordinator.ActivitySeek:
		if c == nil || !c.playing || c.id != a.Item.ID {
			return nil, nil
		}
		c.played += max(0, a.From-c.from)
		c.from = a.Position
	case coordinator.ActivityEnd:
		if c == nil || !c.playing || c.id != a.Item.ID {
			return nil, nil
		}
		c.played += max(0, a.Position-c.from)
		c.playing = false
		c.paused = a.Reason == coordinator.EndPaused
		if !c.paused {
			t.cur = nil
		}
		if c.track == nil || c.counted {
			return nil, nil
		}
		if c.track.Duration <= 0 && a.Duration > 0 {
			c.track.Duration = a.Duration
		}
		if d := c.track.Duration; d > MinDuration && c.played >= Threshold(d) {
			c.counted = true
			done = &Listen{Track: *c.track, ListenedAt: c.start}
		}
	}
	return now, done
}

func (t *Tracker) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}
