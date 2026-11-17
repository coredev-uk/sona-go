package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"
)

// `sona connect` is a long-lived bridge for the person's own programs that
// drive Sona (a Stream Deck plugin, a scripted panel): add-on protocol v2's
// frames on its standard input and output (docs/addons.md), with the
// command line's rights, over the command line's transport. The running
// Sona sees only v1 commands; the session, its topics and its limits live
// here.
//
// It differs from an add-on's channel in what the person may do anyway:
// hello may name any ID, every permission but network is granted, and
// private listening may be switched. It has no settings topic, no secret
// method and no invokes. The queue topic is polled: a change made without
// a status change (songs added) arrives within connectQueueEvery.

// connectQueueEvery is how often the queue is read again while a session
// is subscribed to it.
var connectQueueEvery = 2 * time.Second

// stdio is a session's channel over the process's standard streams.
// Closing it closes the input, which ends the session's read.
type stdio struct {
	io.Reader
	io.Writer
}

func (s stdio) Close() error {
	if c, ok := s.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// connect runs `sona connect` over t until the program hangs up (status 0)
// or Sona quits (status 3, after the shutdown event).
func (env Env) connect(ctx context.Context, t Transport) int {
	if env.Stdin == nil {
		env.write("connect: no standard input")
		return ExitFailed
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	h := newConnectHost(t)
	defer h.hub.Close()
	s := NewSession(AddonConfig{
		Owner: true, Host: h,
		Allowed: func(p string) bool { return p != PermNetwork },
		Log:     slog.New(slog.DiscardHandler),
	})
	quit := make(chan error, 1)
	go func() { quit <- h.follow(ctx) }()
	go h.pollQueue(ctx)
	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, stdio{env.Stdin, env.Stdout}) }()
	select {
	case err := <-served:
		if err != nil {
			env.write(err.Error())
			return ExitFailed
		}
		return ExitOK
	case err := <-quit:
		if ctx.Err() != nil {
			return ExitOK
		}
		// Sona quit (or the stream broke): the program hears shutdown, then
		// connect ends (the session closes the input once it is written, or
		// after its grace; a stdin that can't be closed doesn't hold it).
		s.Close()
		select {
		case serr := <-served:
			if serr != nil {
				env.write(serr.Error())
			}
		case <-time.After(2 * flushGrace):
		}
		if err != nil && !errors.Is(err, ErrNotRunning) && !errors.Is(err, errStopFollow) {
			env.errorf(msgFollowEnded, err)
		}
		return ExitNotRunning
	}
}

// connectHost is the session's AddonHost over the transport: calls go to
// the running Sona as v1 requests; the status stream feeds the status and
// now-playing topics, and the queue is read on each status change and
// every connectQueueEvery while watched.
type connectHost struct {
	t   Transport
	hub *Hub

	mu        sync.Mutex
	queueSubs int
	playing   string // the track ID now-playing was published for
	started   int64  // when that listen started, Unix ms
	kick      chan struct{}
}

func newConnectHost(t Transport) *connectHost {
	return &connectHost{t: t, hub: NewHub(), kick: make(chan struct{}, 1)}
}

// Call implements AddonHost. Answers pass through as Sona sent them, but
// the queue's, which is decoded so that the session windows it.
func (h *connectHost) Call(ctx context.Context, req Request) (any, error) {
	raw, err := h.t.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	if req.Command == "queue" && len(req.Args) == 0 {
		var q Queue
		if err := json.Unmarshal(raw, &q); err != nil {
			return nil, &Error{Code: CodeIncompatible, Message: err.Error()}
		}
		return q, nil
	}
	return raw, nil
}

// Watch implements AddonHost.
func (h *connectHost) Watch(topic string) (<-chan any, func(), error) {
	switch topic {
	case TopicStatus, TopicNowPlaying:
	case TopicQueue:
		h.mu.Lock()
		h.queueSubs++
		h.mu.Unlock()
		h.readQueueSoon()
		ch, cancel := h.hub.Watch(topic)
		var once sync.Once
		return ch, func() {
			once.Do(func() {
				h.mu.Lock()
				h.queueSubs--
				h.mu.Unlock()
				cancel()
			})
		}, nil
	default:
		return nil, nil, &Error{Code: AddonErrUnknownTopic}
	}
	ch, cancel := h.hub.Watch(topic)
	return ch, cancel, nil
}

// follow publishes every status change and what plays, until Sona quits
// (nil) or the stream fails.
func (h *connectHost) follow(ctx context.Context) error {
	return h.t.Follow(ctx, func(st Status) error {
		if !st.Running {
			return errStopFollow
		}
		h.hub.Publish(TopicStatus, st)
		h.hub.Publish(TopicNowPlaying, h.nowPlaying(st, time.Now()))
		h.readQueueSoon()
		return nil
	})
}

// errStopFollow ends follow when the stream says Sona isn't running.
var errStopFollow = errors.New("cli: sona is not running")

// nowPlaying is st as the now-playing topic: the track, and when its listen
// started (the first status that had it, less its position).
func (h *connectHost) nowPlaying(st Status, now time.Time) NowPlaying {
	if st.Track == nil {
		h.playing = ""
		return NowPlaying{}
	}
	if st.Track.ID != h.playing {
		h.playing = st.Track.ID
		h.started = now.Add(-time.Duration(st.Position * float64(time.Second))).UnixMilli()
	}
	return NowPlaying{Track: st.Track, StartedAt: h.started}
}

func (h *connectHost) readQueueSoon() {
	select {
	case h.kick <- struct{}{}:
	default:
	}
}

// pollQueue reads the queue while it is watched: on each status change and
// every connectQueueEvery.
func (h *connectHost) pollQueue(ctx context.Context) {
	t := time.NewTicker(connectQueueEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.kick:
		case <-t.C:
		}
		h.mu.Lock()
		watched := h.queueSubs > 0
		h.mu.Unlock()
		if !watched {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, commandTimeout)
		res, err := h.Call(cctx, Request{Command: "queue"})
		cancel()
		if err == nil {
			h.hub.Publish(TopicQueue, res)
		}
	}
}
