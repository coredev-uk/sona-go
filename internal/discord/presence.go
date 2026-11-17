package discord

import (
	"context"
	"log/slog"
	"time"
)

const (
	// callTimeout bounds each Discord call.
	callTimeout = 5 * time.Second
	// presenceInterval spaces SET_ACTIVITY calls (Discord allows 5 per 20 s).
	presenceInterval = 4 * time.Second
	// presenceRetry is how often an unreachable Discord is retried while
	// something is shown.
	presenceRetry = 30 * time.Second
)

// Want is what the user's Discord should show: the activity, or nothing
// (Activity nil) while Enabled; a disabled presence is cleared and its
// connection closed.
type Want struct {
	Enabled  bool
	Activity *Activity
}

// Presence sends the wanted activity to Discord, coalescing changes to
// Discord's rate limit and retrying while Discord is unreachable. Its
// owner calls Run once, on a goroutine of its own, Set for each change and
// Stop at the end.
type Presence struct {
	client *Client
	// onConnected is told whether the connection is open after each call it
	// made (nil: nobody is).
	onConnected func(connected bool)
	log         *slog.Logger

	want chan Want // latest only
	quit chan struct{}
	done chan struct{}
}

// NewPresence returns the worker of client. onConnected, when set, is
// called from the worker's goroutine with whether Discord is connected,
// after each attempt to reach it.
func NewPresence(client *Client, onConnected func(bool), log *slog.Logger) *Presence {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Presence{
		client: client, onConnected: onConnected, log: log,
		want: make(chan Want, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Set replaces the wanted presence. It never blocks.
func (p *Presence) Set(want Want) {
	for {
		select {
		case p.want <- want:
			return
		default:
			select {
			case <-p.want:
			default:
			}
		}
	}
}

// Stop clears the presence, disconnects and waits for Run to return.
func (p *Presence) Stop() {
	close(p.quit)
	<-p.done
}

func (p *Presence) connected(c bool) {
	if p.onConnected != nil {
		p.onConnected(c)
	}
}

// Run serves Set until Stop.
func (p *Presence) Run() {
	defer close(p.done)
	var (
		want    Want
		sent    *Activity
		synced  = true // Discord shows want (nothing wanted at start)
		next    time.Time
		timer   = time.NewTimer(0)
		pending bool // timer armed
	)
	<-timer.C
	defer func() {
		timer.Stop()
		if p.client.Connected() {
			ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
			_ = p.client.SetActivity(ctx, nil)
			cancel()
		}
		_ = p.client.Close()
	}()
	arm := func(at time.Time) {
		if pending {
			timer.Stop()
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(time.Until(at))
		pending = true
	}
	for {
		select {
		case <-p.quit:
			return
		case nw := <-p.want:
			if nw.Enabled == want.Enabled && SameActivity(nw.Activity, sent) && synced {
				want = nw
				continue
			}
			want, synced = nw, false
			if time.Now().Before(next) {
				arm(next)
				continue
			}
		case <-timer.C:
			pending = false
		}
		if synced {
			continue
		}
		if !want.Enabled && !p.client.Connected() {
			sent, synced = nil, true
			p.connected(false)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		err := p.client.SetActivity(ctx, want.Activity)
		cancel()
		next = time.Now().Add(presenceInterval)
		if err == nil {
			sent, synced = want.Activity, true
			if !want.Enabled {
				_ = p.client.Close()
			}
		} else if want.Activity != nil {
			arm(time.Now().Add(presenceRetry))
		} else {
			// Nothing to show and Discord gone: nothing left to clear.
			sent, synced = nil, true
		}
		if err != nil {
			p.log.Debug("discord presence", "err", err)
		}
		p.connected(p.client.Connected())
	}
}

// SameActivity reports whether a and b show the same thing; the start and
// end may drift by the extrapolation of the position they come from.
func SameActivity(a, b *Activity) bool {
	if a == nil || b == nil {
		return a == b
	}
	near := func(x, y time.Time) bool {
		d := x.Sub(y)
		return d < 2*time.Second && d > -2*time.Second
	}
	return a.Title == b.Title && a.Artist == b.Artist && a.Album == b.Album &&
		a.ArtworkURL == b.ArtworkURL && near(a.Start, b.Start) && near(a.End, b.End)
}
