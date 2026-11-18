package services

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// addonHost is what an add-on's session (cli.Session) sees of the running
// Sona: the v1 commands and three topics, as the command server answers
// them, with one rule on top: an add-on never learns of a listen the one
// private filter hides (Mode.Hidden: a preview, or private at any point
// since the listen started, so private mode switched on and off again
// during it still keeps it hidden). For it there is nothing playing: no
// event, and status, queue, lyrics and like answer as if the song weren't
// there.
type addonHost struct {
	s    *commandServer
	hub  *cli.Hub
	feed *cli.Feed
	now  func() time.Time

	pub   sync.Mutex // orders observe; taken before mu
	mu    sync.Mutex
	uid   uint64    // the queue slot of the listen in progress
	start time.Time // when Sona first saw it
	// preview: the listen in progress has been a preview at some point; it
	// stays one, whatever later snapshots say.
	preview bool
	// hiddenUIDs are the queue slots judged hidden (private or preview):
	// a queue shown to an add-on leaves them out.
	hiddenUIDs map[uint64]bool
	// seen is the last status published to add-ons. While a listen is
	// hidden it stays as it is (or goes idle once): nothing the user does
	// meanwhile becomes an event.
	seen cli.Status
}

func newAddonHost(s *commandServer) *addonHost {
	h := &addonHost{s: s, hub: cli.NewHub(), now: s.e.Now, hiddenUIDs: map[uint64]bool{}}
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
	h.observe()
}

// listenLocked tracks the listen of snap and reports whether an add-on may
// not know of it, with when it started. A new queue slot is a new listen.
func (h *addonHost) listenLocked(snap coordinator.Snapshot) (start time.Time, hidden bool) {
	if snap.Current == nil {
		h.uid, h.start, h.preview = 0, time.Time{}, false
		return h.start, false
	}
	if snap.Current.UID != h.uid {
		h.uid, h.start, h.preview = snap.Current.UID, h.now(), false
	}
	h.preview = h.preview || snap.Preview
	hidden = h.s.e.Private.Hidden(h.start, h.preview)
	if hidden {
		h.hiddenUIDs[h.uid] = true
	}
	return h.start, hidden
}

// queueLocked is snap's queue without the slots judged hidden, re-indexed.
// Slots that left the queue are forgotten.
func (h *addonHost) queueLocked(snap coordinator.Snapshot) cli.Queue {
	q := cli.Queue{Index: -1, Tracks: make([]cli.QueueTrack, 0, len(snap.Queue))}
	live := make(map[uint64]bool, len(snap.Queue))
	for i, e := range snap.Queue {
		live[e.UID] = true
		if h.hiddenUIDs[e.UID] {
			continue
		}
		if i == snap.Index && snap.Current != nil {
			q.Index = len(q.Tracks)
		}
		q.Tracks = append(q.Tracks, cli.QueueTrack{Track: track(e.Item), AutoPlay: e.Auto})
	}
	for uid := range h.hiddenUIDs {
		if !live[uid] {
			delete(h.hiddenUIDs, uid)
		}
	}
	return q
}

// verdict judges the listen of a fresh snapshot: its queue slot and whether
// an add-on may not know of it.
func (h *addonHost) verdict() (uid uint64, hidden bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, hidden = h.listenLocked(h.s.e.Coord.Snapshot())
	return h.uid, hidden
}

// observe publishes what an add-on may see of the player now. It runs on
// every coordinator snapshot and every private mode change. It takes its
// own snapshot under mu rather than the one that woke it: callers hold
// snapshots of different ages, and judging an old one after a newer one
// would forget which slots were hidden and restart the listen.
func (h *addonHost) observe() {
	h.pub.Lock() // publishes one at a time, in order
	defer h.pub.Unlock()
	h.mu.Lock()
	snap := h.s.e.Coord.Snapshot()
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
		q = h.queueLocked(snap)
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
	st.Liked, st.Disliked, st.InLibrary = false, false, false
	return st
}

// visible is snap's status with the flags that would say why a listen is
// hidden and the error text cleared: an add-on that is shown a song is shown a public one.
func (h *addonHost) visible(snap coordinator.Snapshot) cli.Status {
	st := h.s.status(snap)
	st.Private, st.Preview = false, false
	// The error text can hold a URL, a path or a song ID of any listen,
	// hidden ones included: an add-on gets none of it.
	st.Error = ""
	return st
}

// statusNow is the status an add-on's call gets.
func (h *addonHost) statusNow() cli.Status {
	h.mu.Lock()
	snap := h.s.e.Coord.Snapshot()
	_, hidden := h.listenLocked(snap)
	h.mu.Unlock()
	if hidden {
		return h.idle(snap)
	}
	return h.visible(snap)
}

// Call implements cli.AddonHost. The commands about the playing song
// (lyrics, like, dislike, library but its lists) fail with nothing_playing
// for a hidden listen; `queue jump`'s place is one in the queue the add-on
// sees.
func (h *addonHost) Call(ctx context.Context, req cli.Request) (any, error) {
	uid, hidden := h.verdict()
	judged := slices.Contains([]string{"lyrics", "like", "dislike"}, req.Command) ||
		(req.Command == "library" && (len(req.Args) == 0 || !slices.Contains([]string{"playlists", "albums"}, req.Args[0])))
	if hidden && judged {
		return nil, &cli.Error{Code: cli.CodeNothingPlaying}
	}
	if req.Command == "queue" && len(req.Args) == 2 && req.Args[0] == "jump" {
		i, err := cli.ParseQueuePlace(req.Args[1])
		if err != nil {
			return nil, err
		}
		slot, ok := h.queueSlot(i)
		if !ok {
			return nil, &cli.Error{Code: cli.CodeUsage}
		}
		req.Args = []string{"jump", strconv.Itoa(slot + 1)}
	}
	res, err := h.s.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	if judged {
		// The command took time, and private mode or the player may have
		// moved on meanwhile: judge again, against the same queue slot.
		if now, hidden := h.verdict(); hidden || now != uid {
			return nil, &cli.Error{Code: cli.CodeNothingPlaying}
		}
		return res, nil
	}
	switch res.(type) {
	case cli.Status:
		return h.statusNow(), nil
	case cli.Queue:
		return h.queueNow(), nil
	}
	return res, nil
}

// queueSlot is the index in the queue of place i (from 0) of the queue an
// add-on sees: without the hidden slots, windowed around the playing song.
func (h *addonHost) queueSlot(i int) (int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	snap := h.s.e.Coord.Snapshot()
	var visible []int
	playing := -1
	for j, e := range snap.Queue {
		if h.hiddenUIDs[e.UID] {
			continue
		}
		if j == snap.Index && snap.Current != nil {
			playing = len(visible)
		}
		visible = append(visible, j)
	}
	lo, hi := cli.AddonQueueWindow(len(visible), playing)
	if lo+i >= hi {
		return 0, false
	}
	return visible[lo+i], true
}

// queueNow is the queue an add-on's call gets.
func (h *addonHost) queueNow() cli.Queue {
	h.mu.Lock()
	defer h.mu.Unlock()
	snap := h.s.e.Coord.Snapshot()
	if _, hidden := h.listenLocked(snap); hidden {
		return cli.Queue{Index: -1, Tracks: []cli.QueueTrack{}}
	}
	return h.queueLocked(snap)
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
