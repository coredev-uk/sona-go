package tunein

import (
	"context"
	"errors"
	"time"
)

// Polling policy. A station that feeds TuneIn its songs is read often
// enough to follow them; one that does not has a profile that hardly
// changes. Failures back off exponentially so a TuneIn outage costs a
// handful of requests, not one every half minute.
const (
	SongInterval    = 30 * time.Second
	ProfileInterval = 10 * time.Minute
	FailureBase     = 30 * time.Second
	FailureMax      = 10 * time.Minute
)

// Interval is how long to wait before reading s again.
func Interval(s Station) time.Duration {
	if s.HasSong {
		return SongInterval
	}
	return ProfileInterval
}

// Backoff is how long to wait after the failures-th failure in a row.
func Backoff(failures int) time.Duration {
	d := FailureBase
	for i := 1; i < failures && d < FailureMax; i++ {
		d *= 2
	}
	return min(d, FailureMax)
}

// Watcher follows one station's profile.
type Watcher struct {
	Client *Client
	ID     string
	// Emit receives the profile the first time and whenever it changes.
	Emit func(Station)
	// Want reports whether to keep reading (nil: always). The first read
	// is made regardless, for the profile; Watch ends the first time Want
	// is false afterwards, so a stream that carries the song itself costs
	// one request.
	Want func() bool
	// Sleep waits d or until ctx ends (nil: a timer).
	Sleep func(ctx context.Context, d time.Duration) error
}

// Watch reads the station until ctx ends, Want is false, or TuneIn does
// not know the station. It returns nil in the last two cases.
func (w *Watcher) Watch(ctx context.Context) error {
	sleep := w.Sleep
	if sleep == nil {
		sleep = sleepFor
	}
	var (
		last     Station
		seen     bool
		failures int
	)
	for {
		s, err := w.Client.Describe(ctx, w.ID)
		wait := time.Duration(0)
		switch {
		case err == nil:
			failures = 0
			if !seen || s != last {
				last, seen = s, true
				w.Emit(s)
			}
			wait = Interval(s)
		case errors.Is(err, ErrNotFound):
			return nil
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			failures++
			wait = Backoff(failures)
		}
		if w.Want != nil && !w.Want() {
			return nil
		}
		if err := sleep(ctx, wait); err != nil {
			return err
		}
		if w.Want != nil && !w.Want() {
			return nil
		}
	}
}

func sleepFor(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
