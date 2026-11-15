package services

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// addonHost is what an add-on's session (cli.Session) sees of the running
// Sona: the v1 commands and three topics, as the command server answers
// them, with one rule on top. The single private filter: an add-on never
// learns of a listen that is private (Mode.Since with the start of the
// listen, so private mode switched on and off again during it still keeps
// it hidden) or a preview. For it there is nothing playing: no event, and
// status, queue, lyrics and like answer as if the song weren't there.
type addonHost struct {
	s    *commandServer
	hub  *cli.Hub
	feed *cli.Feed
	now  func() time.Time

	pub   sync.Mutex // orders observe; taken before mu
	mu    sync.Mutex
	uid   uint64    // the queue slot of the listen in progress
	start time.Time // when Sona first saw it
	// seen is the last status published to add-ons. While a listen is
	// hidden it stays as it is (or goes idle once): nothing the user does
	// meanwhile becomes an event.
	seen cli.Status
}

func newAddonHost(s *commandServer) *addonHost {
	h := &addonHost{s: s, hub: cli.NewHub(), now: s.e.Now}
	if h.now == nil {
		h.now = time.Now
	}
	h.feed = cli.NewFeed(func() cli.Status {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.seen
	})
	return h
}

// run starts the goroutine that moves statuses from the feed (which drops
// a position that only moves on) to the hub.
func (h *addonHost) run() {
	updates, cancel := h.feed.Watch()
	done := make(chan struct{})
	h.s.e.Go("addon status", func() {
		defer close(done)
		for st := range updates {
			h.hub.Publish(cli.TopicStatus, st)
		}
	})
	h.s.e.OnClose(func() {
		h.feed.Close()
		<-done
		cancel()
		h.hub.Close()
	})
	h.observe(h.s.e.Coord.Snapshot())
}

// listenLocked tracks the listen of snap and reports whether an add-on may
// not know of it, with when it started. A new queue slot is a new listen.
func (h *addonHost) listenLocked(snap coordinator.Snapshot) (start time.Time, hidden bool) {
	if snap.Current == nil {
		h.uid, h.start = 0, time.Time{}
		return h.start, false
	}
	if snap.Current.UID != h.uid {
		h.uid, h.start = snap.Current.UID, h.now()
	}
	return h.start, snap.Preview || h.s.e.Private.Since(h.start)
}

// observe publishes what an add-on may see of snap. It runs on every
// coordinator snapshot and every private mode change.
func (h *addonHost) observe(snap coordinator.Snapshot) {
	h.pub.Lock() // publishes one at a time, in order
	defer h.pub.Unlock()
	h.mu.Lock()
	start, hidden := h.listenLocked(snap)
	var (
		st cli.Status
		np cli.NowPlaying
		q  cli.Queue
	)
	if hidden {
		st = h.seen
		if st.Track != nil || !st.Running {
			st = h.idle(snap)
		}
		q = cli.Queue{Index: -1, Tracks: []cli.QueueTrack{}}
	} else {
		st = h.visible(snap)
		q = h.s.queue(snap)
		if st.Track != nil {
			np = cli.NowPlaying{Track: st.Track, StartedAt: start.UnixMilli()}
		}
	}
	h.seen = st
	h.mu.Unlock()
	h.feed.Publish(st)
	h.hub.Publish(cli.TopicNowPlaying, np)
	h.hub.Publish(cli.TopicQueue, q)
}

// idle is the status of a Sona that plays nothing an add-on may see.
func (h *addonHost) idle(snap coordinator.Snapshot) cli.Status {
	st := h.s.status(snap)
	st.State, st.Track, st.Position, st.Error = cli.StateIdle, nil, 0, ""
	st.Private, st.Preview = false, false
	return st
}

// visible is snap's status with the flags that would say why a listen is
// hidden cleared: an add-on that is shown a song is shown a public one.
func (h *addonHost) visible(snap coordinator.Snapshot) cli.Status {
	st := h.s.status(snap)
	st.Private, st.Preview = false, false
	return st
}

// statusNow is the status an add-on's call gets.
func (h *addonHost) statusNow() cli.Status {
	snap := h.s.e.Coord.Snapshot()
	h.mu.Lock()
	_, hidden := h.listenLocked(snap)
	h.mu.Unlock()
	if hidden {
		return h.idle(snap)
	}
	return h.visible(snap)
}

// Call implements cli.AddonHost.
func (h *addonHost) Call(ctx context.Context, req cli.Request) (any, error) {
	snap := h.s.e.Coord.Snapshot()
	h.mu.Lock()
	_, hidden := h.listenLocked(snap)
	h.mu.Unlock()
	if hidden && slices.Contains([]string{"lyrics", "like"}, req.Command) {
		return nil, &cli.Error{Code: cli.CodeNothingPlaying}
	}
	res, err := h.s.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	switch res.(type) {
	case cli.Status:
		return h.statusNow(), nil
	case cli.Queue:
		if hidden {
			return cli.Queue{Index: -1, Tracks: []cli.QueueTrack{}}, nil
		}
	}
	return res, nil
}

// Watch implements cli.AddonHost.
func (h *addonHost) Watch(topic string) (<-chan any, func(), error) {
	if _, ok := cli.TopicPermission(topic); !ok {
		return nil, nil, &cli.Error{Code: cli.AddonErrUnknownTopic}
	}
	ch, cancel := h.hub.Watch(topic)
	return ch, cancel, nil
}

var _ cli.AddonHost = (*addonHost)(nil)
