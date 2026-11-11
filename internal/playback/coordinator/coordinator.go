// Package coordinator owns Sona's play queue and now-playing state on top of
// a playback.Engine, which plays one item at a time.
//
// The coordinator advances the queue when an item ends (honouring shuffle
// and repeat), preloads the next item shortly before the current one ends,
// skips failing items, pauses on credential problems, reports listening
// history and persists the queue so a restart restores it paused.
//
// A radio station (PlayStation) or AutoPlay (SetAutoPlay) feeds the queue
// from a StationSource: its slots are marked Auto, always follow the
// user's upcoming slots, and are refilled in the background when few
// remain. While online (SetOnline), AutoPlay is fetched ahead, as soon as
// the user's queue is set, so its songs show after the user's from the
// start; otherwise it starts when the user's queue is about to end.
package coordinator

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
)

const (
	// HistoryMin is the listening time after which a skipped item counts as
	// played (as does listening to half of it).
	HistoryMin = 30 * time.Second
	// RestartThreshold: Previous restarts the current item when it has
	// played longer than this, and goes back one item otherwise.
	RestartThreshold = 3 * time.Second
	maxHistory       = 100
	// maxPlayedAuto bounds the played station slots kept before the
	// current one (for Previous), so a long station session stays small.
	maxPlayedAuto = 50
)

// Options configures New.
type Options struct {
	Engine playback.Engine // required; the caller closes it
	Logger *slog.Logger    // nil discards
	Store  Store           // queue persistence; nil disables it
	// OnPlayed is called (never under a lock) when the current item ends,
	// or when it is skipped or replaced after HistoryMin or half its
	// duration of actual listening.
	OnPlayed func(item Item, playedFor time.Duration, completed bool)
	// OnActivity receives listening transitions in order, never under the
	// state lock (engine commands may be in progress, so it must not call
	// back into the Coordinator). Close delivers the final EndExited.
	OnActivity  func(Activity)
	Rand        *rand.Rand    // shuffle source; nil uses a random seed
	MaxFailures int           // consecutive failed items before stopping; 0 means 3
	PreloadLead time.Duration // preload the next item this long before the end; 0 means 15 s
	SaveDelay   time.Duration // debounce for Store writes; 0 means 2 s
	// Stations feeds station and AutoPlay slots; nil disables both.
	Stations StationSource
	// RefillAt: a refill starts once this many or fewer items follow the
	// current one while it plays; 0 means 2.
	RefillAt int
	// RefillRetry is the pause after a failed refill before the next
	// background attempt; 0 means 30 s.
	RefillRetry time.Duration
	// FillWait bounds how long running out of items waits for a refill
	// before playback stops; 0 means 20 s.
	FillWait time.Duration
	// AutoPlayDelay debounces fetching AutoPlay ahead after the user's
	// queue changes; 0 means 2 s.
	AutoPlayDelay time.Duration
	// Now is the clock the coordinator reads for positions, the sleep timer
	// and refill retries; nil is time.Now. Tests give a fake one. It does
	// not move timers: those still run on real time.
	Now func() time.Time
}

type entry struct {
	uid   uint64
	item  Item
	auto  bool // from the station, not the user
	added bool // added by hand (PlayNext, Append, Insert)
}

// fillCall is one station refill; done closes when it finished. seed is
// the AutoPlay seed key (seedKey) when the refill creates the AutoPlay
// station.
type fillCall struct {
	gen  uint64
	seed string
	done chan struct{}
}

// Coordinator is the queue and now-playing state machine. It is safe for
// concurrent use. Engine calls are serialised and never made while the
// state lock is held, so Snapshot never waits on the engine. A command
// that changes what plays ends the engine work of the commands before it
// rather than waiting for it (enter).
type Coordinator struct {
	eng       playback.Engine
	log       *slog.Logger
	store     Store
	onPlayed  func(Item, time.Duration, bool)
	onAct     []func(Activity) // OnActivity, then ObserveActivity observers; guarded by mu
	actMu     sync.Mutex       // serialises activity delivery
	rng       *rand.Rand
	maxFail   int
	lead      time.Duration
	saveDelay time.Duration
	stations  StationSource
	refillAt  int
	retry     time.Duration
	now       func() time.Time
	fillWait  time.Duration
	aheadWait time.Duration

	ctx    context.Context // cancelled by Close
	cancel context.CancelFunc
	wg     sync.WaitGroup

	cmd    sync.Mutex // serialises engine commands; held across engine calls
	saveMu sync.Mutex // serialises Store writes
	subMu  sync.Mutex // guards subs; acquired before mu, never under it
	subs   map[chan Snapshot]struct{}

	mu           sync.Mutex         // guards everything below
	tasks        map[*task]struct{} // playback work enter may end
	turn, served uint64             // enter's tickets: the next one handed out, the next one to take cmd
	turnCond     *sync.Cond         // on mu: served moved
	closed       bool
	queue        []entry  // play order
	original     []uint64 // unshuffled order (UIDs) while shuffled
	index        int      // current entry; -1 when none
	nextUID      uint64
	queueVersion uint64
	label        string
	shuffle      bool
	repeat       Repeat
	st           playback.Status // current item status (engine-reported or synthetic)
	loaded       bool            // the engine has the current item loaded
	volume       float64
	volumeDirty  bool // restored volume not yet applied to the engine
	failures     int
	lastErr      error
	auth         bool
	noCDM        bool          // the engine had no CDM ready (playback.ErrNoCDM)
	playedFor    time.Duration // listening time of the current item, excluding the running stretch
	playingSince time.Time     // start of the running stretch; zero unless playing
	preloaded    uint64        // UID of the entry last preloaded
	preloadTimer *time.Timer
	saveTimer    *time.Timer
	savePending  bool
	history      []Item
	acts         []Activity // undelivered OnActivity calls
	actOpen      bool       // an ActivityStart awaits its ActivityEnd
	actItem      Item
	autoplay     bool
	online       bool     // AutoPlay may be fetched ahead (SetOnline)
	station      *Station // feeding the queue; nil when none
	autoSeed     string   // seedKey the AutoPlay station was created from
	aheadTimer   *time.Timer
	aheadKey     string    // seedKey aheadTimer will fetch AutoPlay for
	fill         *fillCall // refill in flight
	fillGen      uint64    // bumped when the queue's station changes; stale refills are dropped
	fillFailed   time.Time // last failed refill (background retries wait RefillRetry)
	restricted   bool      // skip explicit items
	undo         *replaced // the queue the last SetQueue/PlayStation replaced
	sleep        Sleep
	sleepTimer   *time.Timer
	faded        bool          // the sleep fade set the engine below c.volume
	armed        playback.Next // armed in the engine to follow the current item
	armedUID     uint64        // entry of armed
}

// New returns a coordinator over opts.Engine and starts consuming its
// events. Call Restore to load a saved queue, and Close when done.
func New(opts Options) (*Coordinator, error) {
	if opts.Engine == nil {
		return nil, errors.New("coordinator: Engine is required")
	}
	c := &Coordinator{
		eng:       opts.Engine,
		log:       opts.Logger,
		store:     opts.Store,
		onPlayed:  opts.OnPlayed,
		rng:       opts.Rand,
		maxFail:   cmp.Or(opts.MaxFailures, 3),
		lead:      cmp.Or(opts.PreloadLead, 15*time.Second),
		saveDelay: cmp.Or(opts.SaveDelay, 2*time.Second),
		stations:  opts.Stations,
		refillAt:  cmp.Or(opts.RefillAt, 2),
		retry:     cmp.Or(opts.RefillRetry, 30*time.Second),
		now:       opts.Now,
		fillWait:  cmp.Or(opts.FillWait, 20*time.Second),
		aheadWait: cmp.Or(opts.AutoPlayDelay, 2*time.Second),
		subs:      map[chan Snapshot]struct{}{},
		tasks:     map[*task]struct{}{},
		index:     -1,
		repeat:    RepeatOff,
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.log == nil {
		c.log = slog.New(slog.DiscardHandler)
	}
	if opts.OnActivity != nil {
		c.onAct = append(c.onAct, opts.OnActivity)
	}
	if c.rng == nil {
		c.rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	st := c.eng.Status()
	c.volume = st.Volume
	c.st = playback.Status{State: playback.StateIdle, Volume: st.Volume, UpdatedAt: c.now()}
	c.turnCond = sync.NewCond(&c.mu)
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.wg.Add(1)
	go c.loop()
	return c, nil
}

// Close stops consuming engine events, cancels preloads and flushes the
// queue to the Store. It does not stop or close the engine. Subscriber
// channels are closed.
func (c *Coordinator) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.index >= 0 {
		now := c.now()
		c.endActLocked(EndExited, c.positionLocked(now), now)
	}
	if c.preloadTimer != nil {
		c.preloadTimer.Stop()
	}
	if c.saveTimer != nil {
		c.saveTimer.Stop()
	}
	if c.sleepTimer != nil {
		c.sleepTimer.Stop()
	}
	if c.aheadTimer != nil {
		c.aheadTimer.Stop()
	}
	c.mu.Unlock()
	c.cancel()
	c.wg.Wait()
	c.deliverActivity()
	var err error
	if c.store != nil {
		err = c.save()
	}
	c.subMu.Lock()
	for ch := range c.subs {
		close(ch)
	}
	c.subs = nil
	c.subMu.Unlock()
	return err
}

// Subscribe returns a channel carrying the latest Snapshot after every
// change (the current one is delivered immediately). Slow receivers miss
// intermediate snapshots, never the latest. cancel unsubscribes.
func (c *Coordinator) Subscribe() (snapshots <-chan Snapshot, cancel func()) {
	ch := make(chan Snapshot, 1)
	c.subMu.Lock()
	if c.subs == nil {
		close(ch)
	} else {
		c.subs[ch] = struct{}{}
		ch <- c.Snapshot()
	}
	c.subMu.Unlock()
	return ch, func() {
		c.subMu.Lock()
		delete(c.subs, ch)
		c.subMu.Unlock()
	}
}

// Snapshot returns the current state.
func (c *Coordinator) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	s := Snapshot{
		State:          c.st.State,
		Index:          c.index,
		Position:       c.positionLocked(now),
		Duration:       c.st.Duration,
		At:             now,
		Volume:         c.volume,
		Shuffle:        c.shuffle,
		Repeat:         c.repeat,
		Context:        c.label,
		Queue:          make([]Entry, len(c.queue)),
		QueueVersion:   c.queueVersion,
		History:        slices.Clone(c.history),
		Cause:          c.lastErr,
		AuthRequired:   c.auth,
		NoCDM:          c.noCDM,
		Preview:        c.index >= 0 && c.st.Preview,
		OnAir:          c.onAirLocked(),
		Info:           c.infoLocked(),
		AutoPlay:       c.autoplay,
		Restricted:     c.restricted,
		CanUndoReplace: c.undo != nil,
	}
	if c.sleep.Mode != SleepOff {
		sl := c.sleep
		s.Sleep = &sl
	}
	if c.station != nil {
		st := *c.station
		s.Station = &st
	}
	for i, e := range c.queue {
		s.Queue[i] = Entry{Index: i, UID: e.uid, Item: e.item, Auto: e.auto, Added: e.added}
	}
	if c.index >= 0 {
		cur := s.Queue[c.index]
		s.Current = &cur
	}
	if c.lastErr != nil {
		s.Error = c.lastErr.Error()
	} else if c.st.State == playback.StateError {
		s.Error = c.st.Err
	}
	s.Controls = s.controls()
	return s
}

// liveLocked reports whether the current item is a live radio stream.
func (c *Coordinator) liveLocked() bool {
	return c.index >= 0 && c.queue[c.index].item.Live()
}

// onAirLocked is what the current item's stream says is on air.
func (c *Coordinator) onAirLocked() playback.OnAir {
	if c.index < 0 {
		return playback.OnAir{}
	}
	return c.st.OnAir
}

// infoLocked is what is known about the current item's broadcaster.
func (c *Coordinator) infoLocked() playback.StationInfo {
	if c.index < 0 {
		return playback.StationInfo{}
	}
	return c.st.Info
}

// SetQueue replaces the queue with items and plays items[start]. label
// names the source (album or playlist ID) for the UI. With shuffle on,
// items[start] plays first and the rest are shuffled. An empty items is Stop.
func (c *Coordinator) SetQueue(ctx context.Context, items []Item, start int, label string) error {
	if len(items) == 0 {
		return c.Stop(ctx)
	}
	if start < 0 || start >= len(items) {
		return fmt.Errorf("coordinator: start index %d out of range [0,%d)", start, len(items))
	}
	ctx, leave := c.enter(ctx, always)
	defer leave()
	c.mu.Lock()
	fx := c.finalizeLocked(false, EndSelected)
	c.undo = c.replacedLocked()
	q := c.entriesLocked(items, false)
	if c.shuffle {
		c.original = uids(q)
		q[0], q[start] = q[start], q[0]
		c.rng.Shuffle(len(q)-1, func(i, j int) { q[i+1], q[j+1] = q[j+1], q[i+1] })
		start = 0
	} else {
		c.original = nil
	}
	c.queue, c.index, c.label = q, -1, label
	c.failures, c.lastErr = 0, nil
	c.dropStationLocked()
	c.queueChangedLocked()
	c.mu.Unlock()
	run(fx)
	return c.start(ctx, start, 0, true)
}

// PlayNext inserts items after the current one.
func (c *Coordinator) PlayNext(items []Item) {
	if len(items) == 0 {
		return
	}
	c.mu.Lock()
	c.insertLocked(c.index+1, items)
	c.mu.Unlock()
	c.notify()
}

// Insert inserts items at queue position at, clamped to the user's
// upcoming slots (after the current item, before any station slots).
// With shuffle on they follow queue[at-1] in the unshuffled order too.
func (c *Coordinator) Insert(at int, items []Item) {
	if len(items) == 0 {
		return
	}
	c.mu.Lock()
	c.insertLocked(min(max(at, c.index+1), c.userEndLocked()), items)
	c.mu.Unlock()
	c.notify()
}

func (c *Coordinator) insertLocked(at int, items []Item) {
	es := c.entriesLocked(items, false)
	for i := range es {
		es[i].added = true
	}
	if c.shuffle {
		pos := 0
		if at > 0 {
			pos = slices.Index(c.original, c.queue[at-1].uid) + 1
		}
		c.original = slices.Insert(c.original, pos, uids(es)...)
	}
	c.queue = slices.Insert(c.queue, at, es...)
	c.queueChangedLocked()
}

// Append adds items after the user's queue (before any upcoming station
// slots).
func (c *Coordinator) Append(items []Item) {
	if len(items) == 0 {
		return
	}
	c.mu.Lock()
	es := c.entriesLocked(items, false)
	for i := range es {
		es[i].added = true
	}
	at := c.userEndLocked()
	if c.shuffle {
		pos := len(c.original)
		if at < len(c.queue) {
			pos = slices.Index(c.original, c.queue[at].uid)
		}
		c.original = slices.Insert(c.original, pos, uids(es)...)
	}
	c.queue = slices.Insert(c.queue, at, es...)
	c.queueChangedLocked()
	c.mu.Unlock()
	c.notify()
}

// userEndLocked is where the user's upcoming slots end: the first station
// slot after the current one, or the end of the queue.
func (c *Coordinator) userEndLocked() int {
	for i := c.index + 1; i < len(c.queue); i++ {
		if c.queue[i].auto {
			return i
		}
	}
	return len(c.queue)
}

// Remove deletes queue[index]. Removing the current item plays the one
// that takes its place if playback was active, else selects it paused.
func (c *Coordinator) Remove(ctx context.Context, index int) error {
	c.mu.Lock()
	if index < 0 || index >= len(c.queue) {
		c.mu.Unlock()
		return fmt.Errorf("coordinator: index %d out of range", index)
	}
	if index != c.index {
		c.removeLocked(index)
		c.mu.Unlock()
		c.notify()
		return nil
	}
	uid := c.queue[index].uid
	c.mu.Unlock()

	c.cmd.Lock()
	defer c.cmd.Unlock()
	c.mu.Lock()
	index = c.indexOfLocked(uid)
	if index < 0 || index != c.index {
		if index >= 0 {
			c.removeLocked(index)
		}
		c.mu.Unlock()
		c.notify()
		return nil
	}
	active := c.loaded && (c.st.State == playback.StatePlaying || c.st.State == playback.StateLoading)
	loaded := c.loaded
	fx := c.finalizeLocked(false, EndSelected)
	c.removeLocked(index)
	if len(c.queue) == 0 {
		c.resetLocked()
		c.mu.Unlock()
		c.notify()
		run(fx)
		return c.stopEngine(ctx, loaded)
	}
	if index < len(c.queue) && active {
		c.mu.Unlock()
		run(fx)
		return c.start(ctx, index, 0, true)
	}
	c.selectLocked(min(index, len(c.queue)-1), 0, true, playback.StatePaused)
	c.mu.Unlock()
	c.notify()
	run(fx)
	return c.stopEngine(ctx, loaded)
}

// Move moves queue[from] to position to; the current item stays current.
// Station and AutoPlay slots don't move, and the user's slots stay before
// the upcoming ones (to is clamped).
func (c *Coordinator) Move(from, to int) error {
	return c.move(from, func() int { return to })
}

// MoveNext moves queue[from] to play right after the current item (the
// front of the user's upcoming songs). The current item stays where it is.
func (c *Coordinator) MoveNext(from int) error {
	return c.move(from, func() int {
		switch {
		case from == c.index:
			return from
		case from < c.index:
			return c.index // the current item shifts down by one
		}
		return c.index + 1
	})
}

// MoveToEnd moves queue[from] to the end of the user's songs, before any
// station or AutoPlay slots. The current item stays where it is.
func (c *Coordinator) MoveToEnd(from int) error {
	return c.move(from, func() int {
		if from == c.index {
			return from
		}
		return c.userEndLocked() - 1
	})
}

// move moves queue[from] to the position `to` names, read under the lock
// with the queue as it is now. A song already there stays as it is.
func (c *Coordinator) move(from int, to func() int) error {
	c.mu.Lock()
	n := len(c.queue)
	if from < 0 || from >= n {
		c.mu.Unlock()
		return fmt.Errorf("coordinator: move %d out of range [0,%d)", from, n)
	}
	e := c.queue[from]
	if e.auto {
		c.mu.Unlock()
		return fmt.Errorf("coordinator: move %d: station slots keep their place", from)
	}
	dest := to()
	if dest < 0 || dest >= n {
		c.mu.Unlock()
		return fmt.Errorf("coordinator: move %d→%d out of range", from, dest)
	}
	if end := c.userEndLocked(); from < end {
		dest = min(dest, end-1)
	}
	if dest == from {
		c.mu.Unlock()
		return nil
	}
	var cur uint64
	if c.index >= 0 {
		cur = c.queue[c.index].uid
	}
	c.queue = slices.Insert(slices.Delete(c.queue, from, from+1), dest, e)
	if c.index >= 0 {
		c.index = c.indexOfLocked(cur)
	}
	c.queueChangedLocked()
	c.mu.Unlock()
	c.notify()
	return nil
}

// Clear removes every item except the current one, which keeps playing,
// and the upcoming station slots.
func (c *Coordinator) Clear() {
	c.mu.Lock()
	if c.index >= 0 {
		q := []entry{c.queue[c.index]}
		for _, e := range c.queue[c.index+1:] {
			if e.auto {
				q = append(q, e)
			}
		}
		c.queue, c.index = q, 0
		if c.shuffle {
			c.original = uids(c.queue)
		}
	} else {
		c.queue, c.original = nil, nil
		c.dropStationLocked()
	}
	c.queueChangedLocked()
	c.mu.Unlock()
	c.notify()
}

// Stop stops playback and empties the queue.
func (c *Coordinator) Stop(ctx context.Context) error {
	ctx, leave := c.enter(ctx, always)
	defer leave()
	c.mu.Lock()
	fx := c.finalizeLocked(false, EndStopped)
	loaded := c.loaded
	c.queue, c.original, c.label = nil, nil, ""
	c.undo = nil
	c.dropStationLocked()
	c.resetLocked()
	c.queueChangedLocked()
	c.mu.Unlock()
	c.notify()
	run(fx)
	return c.stopEngine(ctx, loaded)
}

// Jump plays queue[index].
func (c *Coordinator) Jump(ctx context.Context, index int) error {
	ctx, leave := c.enter(ctx, func() bool { return index >= 0 && index < len(c.queue) })
	defer leave()
	c.mu.Lock()
	if index < 0 || index >= len(c.queue) {
		c.mu.Unlock()
		return fmt.Errorf("coordinator: index %d out of range", index)
	}
	fx := c.finalizeLocked(false, EndSelected)
	c.failures, c.lastErr = 0, nil
	c.mu.Unlock()
	run(fx)
	return c.start(ctx, index, 0, true)
}

// Next plays the following item, wrapping when repeat is on. At the end of
// the queue with repeat off, playback stops on the last item (Resume
// replays it).
func (c *Coordinator) Next(ctx context.Context) error {
	ctx, leave := c.enter(ctx, func() bool { return !c.liveLocked() })
	defer leave()
	c.mu.Lock()
	if c.liveLocked() {
		c.mu.Unlock()
		return ErrLiveStation
	}
	next, ok := c.nextIndexLocked(false)
	if len(c.queue) == 0 {
		c.mu.Unlock()
		return nil
	}
	if !ok && c.index >= 0 {
		// The station may still have more: wait for its refill.
		next, ok = c.awaitFillLocked(ctx, false)
	}
	fx := c.finalizeLocked(false, EndSkipForward)
	if !ok {
		loaded := c.loaded
		c.setStateLocked(playback.StateEnded)
		c.st.Position = 0
		c.loaded = false
		c.mu.Unlock()
		c.notify()
		run(fx)
		return c.stopEngine(ctx, loaded)
	}
	c.failures, c.lastErr = 0, nil
	c.mu.Unlock()
	run(fx)
	return c.start(ctx, next, 0, true)
}

// Previous restarts the current item if it has played for more than
// RestartThreshold (or there is nothing before it), else plays the previous
// item (wrapping when repeat is on).
func (c *Coordinator) Previous(ctx context.Context) error {
	ctx, leave := c.enter(ctx, func() bool {
		return c.index >= 0 && !c.liveLocked() &&
			(c.station == nil || c.station.AutoPlay || c.positionLocked(c.now()) > RestartThreshold)
	})
	defer leave()
	c.mu.Lock()
	if c.index < 0 {
		c.mu.Unlock()
		return nil
	}
	if c.liveLocked() {
		c.mu.Unlock()
		return ErrLiveStation
	}
	if c.station != nil && !c.station.AutoPlay {
		// A station has nothing before the song: Previous restarts it,
		// and does nothing in its first moments.
		if c.positionLocked(c.now()) <= RestartThreshold {
			c.mu.Unlock()
			return nil
		}
		return c.restartLocked(ctx)
	}
	prev := c.index - 1
	if prev < 0 && c.repeat != RepeatOff {
		prev = len(c.queue) - 1
	}
	if c.positionLocked(c.now()) <= RestartThreshold && prev >= 0 && prev != c.index {
		fx := c.finalizeLocked(false, EndSkipBackward)
		c.failures, c.lastErr = 0, nil
		c.mu.Unlock()
		run(fx)
		return c.start(ctx, prev, 0, true)
	}
	return c.restartLocked(ctx)
}

// restartLocked rewinds the current item; called with cmd and mu held,
// returns with mu released.
func (c *Coordinator) restartLocked(ctx context.Context) error {
	switch {
	case c.loaded && (c.st.State == playback.StatePlaying || c.st.State == playback.StatePaused || c.st.State == playback.StateLoading):
		uid := c.queue[c.index].uid
		c.seekActLocked(0)
		c.mu.Unlock()
		if err := c.eng.Seek(ctx, 0); err != nil && !superseded(ctx) {
			return fmt.Errorf("coordinator: restart: %w", err)
		}
		c.refresh(uid)
		return nil
	case c.st.State == playback.StatePaused:
		c.st.Position, c.st.UpdatedAt = 0, c.now()
		c.mu.Unlock()
		c.notify()
		return nil
	default:
		idx := c.index
		c.failures, c.lastErr = 0, nil
		c.mu.Unlock()
		return c.start(ctx, idx, 0, true)
	}
}

// Pause pauses playback.
func (c *Coordinator) Pause(ctx context.Context) error {
	c.cmd.Lock()
	defer c.cmd.Unlock()
	c.mu.Lock()
	if c.index < 0 || !c.loaded || (c.st.State != playback.StatePlaying && c.st.State != playback.StateLoading) {
		c.mu.Unlock()
		return nil
	}
	uid := c.queue[c.index].uid
	c.mu.Unlock()
	if err := c.eng.Pause(ctx); err != nil {
		return fmt.Errorf("coordinator: pause: %w", err)
	}
	c.refresh(uid)
	return nil
}

// Resume continues playback: it resumes a paused item, loads a restored or
// auth-interrupted item at its saved position, replays an ended item, and
// starts the queue if nothing is selected. It clears AuthRequired.
func (c *Coordinator) Resume(ctx context.Context) error {
	ctx, leave := c.enter(ctx, nil)
	defer leave()
	c.mu.Lock()
	c.auth = false
	if len(c.queue) == 0 {
		c.mu.Unlock()
		return nil
	}
	if c.index < 0 {
		c.failures, c.lastErr = 0, nil
		c.mu.Unlock()
		return c.start(ctx, 0, 0, true)
	}
	idx, uid, st := c.index, c.queue[c.index].uid, c.st.State
	switch {
	case c.loaded && st == playback.StatePaused:
		c.mu.Unlock()
		if err := c.eng.Resume(ctx); err != nil && !superseded(ctx) {
			return fmt.Errorf("coordinator: resume: %w", err)
		}
		c.refresh(uid)
		return nil
	case c.loaded && (st == playback.StatePlaying || st == playback.StateLoading):
		c.mu.Unlock()
		c.notify()
		return nil
	case st == playback.StatePaused || st == playback.StateLoading:
		pos := c.st.Position
		c.mu.Unlock()
		return c.start(ctx, idx, pos, false)
	default: // ended, error, idle
		c.failures, c.lastErr = 0, nil
		c.mu.Unlock()
		return c.start(ctx, idx, 0, true)
	}
}

// Toggle pauses when playing (or loading) and resumes otherwise.
func (c *Coordinator) Toggle(ctx context.Context) error {
	c.mu.Lock()
	st := c.st.State
	c.mu.Unlock()
	if st == playback.StatePlaying || st == playback.StateLoading {
		return c.Pause(ctx)
	}
	return c.Resume(ctx)
}

// Seek moves the current item to pos (clamped to its duration). For an
// item not loaded in the engine (restored, ended) it only sets the position
// Resume starts from.
func (c *Coordinator) Seek(ctx context.Context, pos time.Duration) error {
	c.cmd.Lock()
	defer c.cmd.Unlock()
	c.mu.Lock()
	if c.index < 0 {
		c.mu.Unlock()
		return nil
	}
	if c.liveLocked() {
		c.mu.Unlock()
		return ErrLiveStation
	}
	pos = max(pos, 0)
	if c.st.Duration > 0 {
		pos = min(pos, c.st.Duration)
	}
	st := c.st.State
	if !c.loaded || st == playback.StateEnded || st == playback.StateError || st == playback.StateIdle {
		c.loaded = false
		if st != playback.StatePaused {
			c.setStateLocked(playback.StatePaused)
		}
		c.st.Position, c.st.UpdatedAt = pos, c.now()
		c.mu.Unlock()
		c.notify()
		return nil
	}
	uid := c.queue[c.index].uid
	c.seekActLocked(pos)
	c.mu.Unlock()
	if err := c.eng.Seek(ctx, pos); err != nil {
		return fmt.Errorf("coordinator: seek: %w", err)
	}
	c.refresh(uid)
	return nil
}

// SetVolume sets the volume (clamped to 0..1; NaN is 0).
func (c *Coordinator) SetVolume(ctx context.Context, v float64) error {
	v = playback.ClampVolume(v)
	c.cmd.Lock()
	defer c.cmd.Unlock()
	if err := c.eng.SetVolume(ctx, v); err != nil {
		return fmt.Errorf("coordinator: set volume: %w", err)
	}
	c.mu.Lock()
	c.volume, c.st.Volume, c.volumeDirty = v, v, false
	c.mu.Unlock()
	c.notify()
	return nil
}

// SetShuffle turns shuffle on (the current item moves to the front and
// the rest of the user's items are shuffled; upcoming station slots keep
// their order after them and played ones are dropped) or off (the
// unshuffled order returns, with the current item still current).
func (c *Coordinator) SetShuffle(on bool) {
	c.mu.Lock()
	if on == c.shuffle {
		c.mu.Unlock()
		return
	}
	c.shuffle = on
	if on {
		var head, user, auto []entry
		for i, e := range c.queue {
			switch {
			case i == c.index:
				head = append(head, e)
			case !e.auto:
				user = append(user, e)
			case i > c.index:
				auto = append(auto, e)
			}
		}
		c.rng.Shuffle(len(user), func(i, j int) { user[i], user[j] = user[j], user[i] })
		kept := make(map[uint64]bool, len(c.queue))
		q := slices.Concat(head, user, auto)
		for _, e := range q {
			kept[e.uid] = true
		}
		c.original = slices.DeleteFunc(uids(c.queue), func(u uint64) bool { return !kept[u] })
		c.queue = q
		if c.index >= 0 {
			c.index = 0
		}
	} else {
		var cur uint64
		if c.index >= 0 {
			cur = c.queue[c.index].uid
		}
		byUID := make(map[uint64]entry, len(c.queue))
		for _, e := range c.queue {
			byUID[e.uid] = e
		}
		q := make([]entry, 0, len(c.queue))
		for _, u := range c.original {
			if e, ok := byUID[u]; ok {
				q = append(q, e)
				delete(byUID, u)
			}
		}
		for _, e := range c.queue { // defensive: keep anything missing from original
			if _, ok := byUID[e.uid]; ok {
				q = append(q, e)
			}
		}
		c.queue, c.original = q, nil
		if c.index >= 0 {
			c.index = c.indexOfLocked(cur)
		}
	}
	c.queueChangedLocked()
	c.mu.Unlock()
	c.notify()
}

// SetRepeat sets the repeat mode. AutoPlay runs only with repeat off, so
// turning repeat on removes the upcoming AutoPlay slots (turning it off
// brings AutoPlay back).
func (c *Coordinator) SetRepeat(r Repeat) error {
	if !r.valid() {
		return fmt.Errorf("coordinator: invalid repeat mode %q", r)
	}
	c.mu.Lock()
	c.repeat = r
	if r != RepeatOff {
		c.dropAutoPlayLocked()
	}
	c.aheadLocked()
	c.schedulePreloadLocked()
	c.mu.Unlock()
	c.notify()
	return nil
}

// Restore loads the saved queue from the Store and selects the saved item
// paused at its saved position, without touching the engine; Resume (or
// Toggle) loads it. It is a no-op without a Store or saved queue.
func (c *Coordinator) Restore(ctx context.Context) error {
	if c.store == nil {
		return nil
	}
	s, err := c.store.LoadQueue(ctx)
	if err != nil {
		return fmt.Errorf("coordinator: restore: %w", err)
	}
	if s == nil || len(s.Items) == 0 {
		return nil
	}
	c.cmd.Lock()
	defer c.cmd.Unlock()
	c.mu.Lock()
	q := c.entriesLocked(s.Items, false)
	for _, i := range s.Auto {
		if i >= 0 && i < len(q) {
			q[i].auto = true
		}
	}
	for _, i := range s.Added {
		if i >= 0 && i < len(q) {
			q[i].added = true
		}
	}
	c.queue, c.label = q, s.Context
	c.station = nil
	if s.Station != nil && s.Station.ID != "" {
		st := *s.Station
		c.station = &st
	}
	c.shuffle = s.Shuffle && isPerm(s.Original, len(q))
	c.original = nil
	if c.shuffle {
		for _, i := range s.Original {
			c.original = append(c.original, q[i].uid)
		}
	}
	c.repeat = RepeatOff
	if s.Repeat.valid() {
		c.repeat = s.Repeat
	}
	c.autoSeed = ""
	if c.station != nil && c.station.AutoPlay {
		c.autoSeed = seedKey(c.seedsLocked()) // the saved station follows the saved queue
	}
	c.volume, c.volumeDirty = min(max(s.Volume, 0), 1), true
	c.resetLocked()
	if s.Index >= 0 && s.Index < len(q) {
		c.selectLocked(s.Index, max(s.Position, 0), true, playback.StatePaused)
		if d := c.st.Duration; d > 0 {
			c.st.Position = min(c.st.Position, d)
		}
		c.playedFor = max(s.PlayedFor, 0)
	}
	c.queueChangedLocked()
	c.mu.Unlock()
	c.notify()
	return nil
}

// loop consumes engine events until Close.
func (c *Coordinator) loop() {
	defer c.wg.Done()
	events := c.eng.Events()
	for {
		select {
		case <-c.ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			c.handle(ev)
		}
	}
}

// handle applies one engine event for the current item.
func (c *Coordinator) handle(ev playback.Event) {
	c.cmd.Lock()
	defer c.cmd.Unlock()
	c.mu.Lock()
	if ev.Kind == playback.EventAdvance && c.index >= 0 && c.loaded {
		c.advanceLocked(ev.Status)
		return
	}
	if c.index < 0 || !c.loaded || ev.Status.ItemID != c.queue[c.index].item.ID {
		c.mu.Unlock()
		return
	}
	switch {
	case ev.Kind == playback.EventAuth:
		c.applyLocked(ev.Status)
		c.auth = true
		if c.st.State != playback.StatePaused {
			c.setStateLocked(playback.StatePaused)
		}
		c.mu.Unlock()
		c.notify()
		if err := c.eng.Pause(c.ctx); err != nil {
			c.log.Warn("coordinator: pause after auth failure", "err", err)
		}
	case ev.Kind == playback.EventError || ev.Status.State == playback.StateError:
		next, ok, fx := c.failLocked(errors.New(cmp.Or(ev.Status.Err, "playback failed")))
		ctx, forget := c.taskLocked(c.ctx)
		defer forget()
		c.mu.Unlock()
		c.notify()
		run(fx)
		if ok {
			_ = c.start(ctx, next, 0, true)
		}
	case ev.Status.State == playback.StateEnded:
		c.applyLocked(ev.Status)
		fx := c.finalizeLocked(true, EndNatural)
		if c.sleepEndsHereLocked() {
			c.sleepStopLocked(fx)
			return
		}
		ctx, forget := c.taskLocked(c.ctx)
		defer forget()
		next, ok := c.nextIndexLocked(true)
		if !ok {
			next, ok = c.awaitFillLocked(ctx, true)
		}
		c.mu.Unlock()
		run(fx)
		if ok {
			_ = c.start(ctx, next, 0, true)
		} else {
			c.notify()
		}
	default:
		c.applyLocked(ev.Status)
		c.mu.Unlock()
		c.notify()
	}
}

// advanceLocked adopts the engine's own move to the armed next item: the
// current item ended naturally and st is the new one's status. Called with
// cmd and mu held; returns with mu released.
func (c *Coordinator) advanceLocked(st playback.Status) {
	now := c.now()
	c.endActLocked(EndNatural, cmp.Or(c.st.Duration, c.positionLocked(now)), now)
	fx := c.finalizeLocked(true, EndNatural)
	ai := c.indexOfLocked(c.armedUID)
	if ai < 0 || st.ItemID != c.queue[ai].item.ID {
		// The arm changed while the engine moved on: play the expected next.
		next, ok := c.nextIndexLocked(true)
		ctx, forget := c.taskLocked(c.ctx)
		defer forget()
		c.mu.Unlock()
		run(fx)
		if ok {
			_ = c.start(ctx, next, 0, true)
		} else {
			c.notify()
		}
		return
	}
	c.selectLocked(ai, st.Position, true, playback.StateLoading)
	c.loaded = true
	c.applyLocked(st)
	c.mu.Unlock()
	run(fx)
	c.notify()
}

// armLocked tells the engine which item follows the current one when it
// plays out (none when nothing follows automatically, or the sleep timer
// ends playback with it).
func (c *Coordinator) armLocked() {
	var (
		want playback.Next
		uid  uint64
	)
	if !c.closed && c.index >= 0 && c.loaded && c.st.State != playback.StateEnded && c.st.State != playback.StateError {
		if ni, ok := c.nextIndexLocked(true); ok && !c.sleepEndsHereLocked() {
			next, cur := c.queue[ni], c.queue[c.index]
			want = playback.Next{ID: next.item.ID, Gapless: cur.item.Album != "" && cur.item.Album == next.item.Album}
			uid = next.uid
		}
	}
	if want == c.armed && uid == c.armedUID {
		return
	}
	c.armed, c.armedUID = want, uid
	c.eng.SetNext(want)
}

// errSuperseded is the cause a command's playback work ends with when a
// later command replaces what plays (enter).
var errSuperseded = errors.New("coordinator: superseded by a later command")

// task is one command's (or auto-advance's) playback work, which enter
// may end.
type task struct{ cancel context.CancelCauseFunc }

// enter takes cmd for a command. When replaces (nil: never; called with mu
// held) says the command changes what plays, the playback work of the
// commands before it ends first (errSuperseded): a Play still loading, a
// refill being waited for, and the commands still waiting for cmd, which
// then pass straight through. Otherwise a skip would wait for the very
// load it skips, and the skips pressed meanwhile would all land when it
// ended. Commands take cmd in the order they entered, so the last one's
// choice is what plays. The returned ctx is ctx, also ended by a later
// command; leave releases cmd.
func (c *Coordinator) enter(ctx context.Context, replaces func() bool) (_ context.Context, leave func()) {
	c.mu.Lock()
	if replaces != nil && replaces() {
		for t := range c.tasks {
			t.cancel(errSuperseded)
		}
	}
	ctx, forget := c.taskLocked(ctx)
	turn := c.turn
	c.turn++
	for c.served != turn {
		c.turnCond.Wait()
	}
	c.mu.Unlock()
	c.cmd.Lock()
	c.mu.Lock()
	c.served++
	c.turnCond.Broadcast()
	c.mu.Unlock()
	return ctx, func() {
		forget()
		c.cmd.Unlock()
	}
}

// always is enter's replaces for commands that always change what plays.
func always() bool { return true }

// taskLocked registers playback work done under ctx for enter to end;
// forget (called without mu) unregisters it. With mu held.
func (c *Coordinator) taskLocked(ctx context.Context) (_ context.Context, forget func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	t := &task{cancel}
	c.tasks[t] = struct{}{}
	return ctx, func() {
		c.mu.Lock()
		delete(c.tasks, t)
		c.mu.Unlock()
		cancel(nil)
	}
}

// superseded reports whether ctx ended because a later command replaced
// what plays.
func superseded(ctx context.Context) bool { return errors.Is(context.Cause(ctx), errSuperseded) }

// start plays queue[i] from pos, skipping forward over items whose Play
// fails. fresh resets the item's listening time (false continues a
// restored listen). An engine that cannot play at all
// (playback.ErrUnsupported) leaves the item selected and paused instead.
// Caller holds cmd, not mu.
func (c *Coordinator) start(ctx context.Context, i int, pos time.Duration, fresh bool) error {
	for {
		c.mu.Lock()
		if i < 0 || i >= len(c.queue) {
			c.mu.Unlock()
			return nil
		}
		if c.withheldLocked(c.queue[i].item) {
			// Explicit content is restricted: play the next allowed item.
			next, ok := c.followingLocked(i, false)
			if !ok {
				c.lastErr = ErrRestricted
				c.mu.Unlock()
				c.notify()
				return ErrRestricted
			}
			i, pos, fresh = next, 0, true
			c.mu.Unlock()
			continue
		}
		e := c.queue[i]
		c.selectLocked(i, pos, fresh, playback.StateLoading)
		vol, dirty := c.volume, c.volumeDirty
		c.volumeDirty = false
		c.mu.Unlock()
		c.notify()
		if dirty {
			if err := c.eng.SetVolume(ctx, vol); err != nil {
				c.log.Warn("coordinator: restore volume", "err", err)
			}
		}
		err := c.eng.Play(ctx, e.item.ID, pos)
		st := c.eng.Status()

		c.mu.Lock()
		if c.index < 0 || c.queue[c.index].uid != e.uid {
			c.mu.Unlock()
			return err
		}
		switch {
		case err == nil:
			c.loaded = true
			if st.ItemID == e.item.ID {
				c.applyLocked(st)
			}
			c.mu.Unlock()
			c.notify()
			return nil
		case ctx.Err() != nil:
			c.setStateLocked(playback.StatePaused)
			c.mu.Unlock()
			c.notify()
			if superseded(ctx) {
				return nil // a later command took over: not a failure
			}
			return err
		case errors.Is(err, applemusic.ErrUnauthorized):
			c.auth = true
			c.setStateLocked(playback.StatePaused)
			c.mu.Unlock()
			c.notify()
			return fmt.Errorf("coordinator: play: %w", err)
		case errors.Is(err, playback.ErrUnsupported), errors.Is(err, playback.ErrNoCDM):
			// Nothing can play here (or not yet: the CDM is still on its
			// way): keep the item, say why, count nothing.
			c.lastErr = err
			c.noCDM = errors.Is(err, playback.ErrNoCDM)
			c.setStateLocked(playback.StatePaused)
			c.st.Err = err.Error()
			c.mu.Unlock()
			c.notify()
			return fmt.Errorf("coordinator: play: %w", err)
		}
		next, ok, fx := c.failLocked(err)
		c.mu.Unlock()
		c.notify()
		run(fx)
		if !ok {
			return fmt.Errorf("coordinator: play: %w", err)
		}
		c.log.Warn("coordinator: skipping failed item", "id", e.item.ID, "err", err)
		i, pos, fresh = next, 0, true
	}
}

// failLocked records a failure of the current item and returns the item
// to try next, or ok=false when playback stops.
func (c *Coordinator) failLocked(err error) (next int, ok bool, fx func()) {
	c.failures++
	c.lastErr = err
	fx = c.finalizeLocked(false, EndFailed)
	c.st.Err = err.Error()
	c.setStateLocked(playback.StateError)
	c.loaded = false
	if c.failures >= c.maxFail {
		c.lastErr = fmt.Errorf("playback stopped after %d failed items: %w", c.failures, err)
		return -1, false, fx
	}
	next, ok = c.nextIndexLocked(false)
	return next, ok, fx
}

// selectLocked makes queue[i] current in state s at pos.
func (c *Coordinator) selectLocked(i int, pos time.Duration, fresh bool, s playback.State) {
	e := c.queue[i]
	c.index = i
	if fresh {
		c.playedFor = 0
	}
	c.playingSince = time.Time{}
	// The engine's preview mode carries over until it reports on this item.
	c.st = playback.Status{State: s, ItemID: e.item.ID, Position: pos, Duration: e.item.Duration, Volume: c.volume, UpdatedAt: c.now(), Preview: c.st.Preview}
	c.loaded, c.auth, c.noCDM, c.preloaded = false, false, false, 0
	c.aheadLocked()
	c.schedulePreloadLocked()
}

// resetLocked deselects everything (the queue itself is kept).
func (c *Coordinator) resetLocked() {
	c.index = -1
	c.st = playback.Status{State: playback.StateIdle, Volume: c.volume, UpdatedAt: c.now()}
	c.loaded, c.auth, c.noCDM, c.preloaded = false, false, false, 0
	c.playedFor, c.playingSince = 0, time.Time{}
	c.failures, c.lastErr = 0, nil
	c.aheadLocked()
	c.schedulePreloadLocked()
}

// applyLocked adopts an engine status for the current item.
func (c *Coordinator) applyLocked(s playback.Status) {
	now := c.now()
	was := c.st.State == playback.StatePlaying
	if s.Duration <= 0 {
		s.Duration = c.st.Duration
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	c.st = s
	if c.faded {
		// The sleep fade lowered the engine volume; the user's stays.
		c.st.Volume = c.volume
	} else {
		c.volume = s.Volume
	}
	switch playing := s.State == playback.StatePlaying; {
	case was && !playing:
		c.playedFor += now.Sub(c.playingSince)
		c.playingSince = time.Time{}
	case !was && playing:
		c.playingSince = now
	}
	if s.State == playback.StatePlaying {
		c.failures, c.lastErr = 0, nil
	}
	// Loading (buffering, seeking) keeps a listen open; only a real stop
	// ends it. A preview is not a listen: no observer hears of it.
	switch {
	case s.State == playback.StatePlaying && !c.actOpen && c.index >= 0 && !s.Preview:
		c.actOpen, c.actItem = true, c.queue[c.index].item
		c.addActLocked(Activity{Kind: ActivityStart, Item: c.actItem, Position: s.Position, Duration: s.Duration, At: now})
	case s.State == playback.StatePaused:
		c.endActLocked(EndPaused, s.Position, now)
	case s.State == playback.StateEnded:
		c.endActLocked(EndNatural, cmp.Or(s.Duration, s.Position), now)
	case s.State == playback.StateError:
		c.endActLocked(EndFailed, s.Position, now)
	case s.State == playback.StateIdle:
		c.endActLocked(EndStopped, s.Position, now)
	}
	c.schedulePreloadLocked()
}

// addActLocked queues a, with the queue's modes, for the activity
// observers.
func (c *Coordinator) addActLocked(a Activity) {
	if len(c.onAct) > 0 {
		a.Mode = c.playModeLocked()
		c.acts = append(c.acts, a)
	}
}

func (c *Coordinator) playModeLocked() PlayMode {
	m := PlayMode{Shuffle: c.shuffle, Repeat: c.repeat, AutoPlay: AutoPlayOff}
	switch {
	case !c.autoplay:
	case c.station != nil && !c.station.AutoPlay:
		m.AutoPlay = AutoPlayUnsupported
	default:
		m.AutoPlay = AutoPlayOn
	}
	return m
}

// endActLocked closes the open listen, if any, at pos.
func (c *Coordinator) endActLocked(reason EndReason, pos time.Duration, now time.Time) {
	if !c.actOpen {
		return
	}
	c.actOpen = false
	c.addActLocked(Activity{Kind: ActivityEnd, Item: c.actItem, Position: pos, Duration: c.st.Duration, Reason: reason, At: now})
}

// seekActLocked records a seek of the open listen to pos.
func (c *Coordinator) seekActLocked(pos time.Duration) {
	if !c.actOpen {
		return
	}
	now := c.now()
	c.addActLocked(Activity{Kind: ActivitySeek, Item: c.actItem, From: c.positionLocked(now), Position: pos, Duration: c.st.Duration, At: now})
}

// ObserveActivity adds an observer that receives the same listening
// transitions as Options.OnActivity, after it, with the same rules: in
// order, never under the state lock, and it must not call back into the
// Coordinator. Transitions before the call are not replayed.
func (c *Coordinator) ObserveActivity(fn func(Activity)) {
	c.mu.Lock()
	c.onAct = append(c.onAct, fn)
	c.mu.Unlock()
}

// deliverActivity hands queued activities to the observers in order.
func (c *Coordinator) deliverActivity() {
	c.actMu.Lock()
	defer c.actMu.Unlock()
	for {
		c.mu.Lock()
		acts, obs := c.acts, c.onAct
		c.acts = nil
		c.mu.Unlock()
		if len(acts) == 0 {
			return
		}
		for _, a := range acts {
			for _, fn := range obs {
				fn(a)
			}
		}
	}
}

// setStateLocked moves the current status to s at the extrapolated position.
func (c *Coordinator) setStateLocked(s playback.State) {
	st := c.st
	now := c.now()
	st.Position, st.UpdatedAt, st.State = c.positionLocked(now), now, s
	c.applyLocked(st)
}

func (c *Coordinator) positionLocked(now time.Time) time.Duration {
	p := c.st.Position
	if c.st.State == playback.StatePlaying {
		p += now.Sub(c.st.UpdatedAt)
	}
	if c.st.Duration > 0 {
		p = min(p, c.st.Duration)
	}
	return max(p, 0)
}

// listenedLocked is the current item's listening time so far.
func (c *Coordinator) listenedLocked(now time.Time) time.Duration {
	if !c.playingSince.IsZero() {
		return c.playedFor + now.Sub(c.playingSince)
	}
	return c.playedFor
}

// finalizeLocked ends the current item's listen (closing an open activity
// with reason) and, if it counts, records it in the session history and
// returns the OnPlayed call to make after unlocking. A preview never
// counts.
func (c *Coordinator) finalizeLocked(completed bool, reason EndReason) func() {
	if c.index < 0 {
		return nil
	}
	now := c.now()
	c.endActLocked(reason, c.positionLocked(now), now)
	played := c.listenedLocked(now)
	c.playedFor = 0
	if !c.playingSince.IsZero() {
		c.playingSince = now
	}
	item := c.queue[c.index].item
	dur := cmp.Or(c.st.Duration, item.Duration)
	if c.st.Preview || (!completed && played < HistoryMin && (dur <= 0 || played < dur/2)) {
		return nil
	}
	c.history = slices.Insert(c.history, 0, item)
	if len(c.history) > maxHistory {
		c.history = c.history[:maxHistory]
	}
	if c.onPlayed == nil {
		return nil
	}
	fn := c.onPlayed
	return func() { fn(item, played, completed) }
}

// nextIndexLocked returns the item after the current one. auto (an item
// ended) honours repeat one; repeat all wraps at the end unless a station
// feeds the queue (it continues instead). Withheld items are skipped.
func (c *Coordinator) nextIndexLocked(auto bool) (int, bool) {
	return c.followingLocked(c.index, auto)
}

// followingLocked is nextIndexLocked as if queue[from] were current.
func (c *Coordinator) followingLocked(from int, auto bool) (int, bool) {
	n := len(c.queue)
	if n == 0 {
		return -1, false
	}
	if auto && c.repeat == RepeatOne && from >= 0 && !c.withheldLocked(c.queue[from].item) {
		return from, true
	}
	for i, steps := from+1, 0; steps < n; i, steps = i+1, steps+1 {
		if i >= n {
			if c.repeat == RepeatOff || c.station != nil {
				return -1, false
			}
			i = 0
		}
		if !c.withheldLocked(c.queue[i].item) {
			return i, true
		}
	}
	return -1, false
}

// withheldLocked reports whether it cannot play on this account.
func (c *Coordinator) withheldLocked(it Item) bool { return c.restricted && it.Explicit }

// PlayStation replaces the queue with station st and plays its first
// track; more tracks are fetched as the queue drains. It fetches the first
// batch before touching the queue, so a failure leaves playback as it was.
func (c *Coordinator) PlayStation(ctx context.Context, st Station) error {
	c.mu.Lock()
	src := c.stations
	c.mu.Unlock()
	if src == nil {
		return errors.New("coordinator: no station source")
	}
	items, err := src.NextTracks(ctx, st.ID)
	if err != nil {
		return fmt.Errorf("coordinator: station %s: %w", st.ID, err)
	}
	if len(items) == 0 {
		return fmt.Errorf("coordinator: station %s returned no tracks", st.ID)
	}
	ctx, leave := c.enter(ctx, always)
	defer leave()
	c.mu.Lock()
	fx := c.finalizeLocked(false, EndSelected)
	c.undo = c.replacedLocked()
	q := c.entriesLocked(fromStation(items, st.ID), true)
	c.original = nil
	if c.shuffle {
		c.original = uids(q)
	}
	c.queue, c.index, c.label = q, -1, st.ID
	c.failures, c.lastErr = 0, nil
	c.dropStationLocked()
	st.AutoPlay = false
	c.station = &st
	c.queueChangedLocked()
	c.mu.Unlock()
	run(fx)
	return c.start(ctx, 0, 0, true)
}

// fromStation sets station id as the container of items that have none.
func fromStation(items []Item, id string) []Item {
	for i := range items {
		if items[i].Container == (Container{}) {
			items[i].Container = Container{Kind: ContainerStation, ID: id}
		}
	}
	return items
}

// SetStationSource sets (or, with nil, removes) the source of station and
// AutoPlay tracks after New, for callers built after the coordinator.
func (c *Coordinator) SetStationSource(src StationSource) {
	c.mu.Lock()
	c.stations = src
	c.mu.Unlock()
}

// SetAutoPlay turns AutoPlay on (similar music continues after the user's
// queue, unless repeat is on) or off (upcoming AutoPlay slots are removed).
func (c *Coordinator) SetAutoPlay(on bool) {
	c.mu.Lock()
	if c.autoplay == on {
		c.mu.Unlock()
		return
	}
	c.autoplay = on
	if !on {
		c.dropAutoPlayLocked()
	}
	c.aheadLocked()
	c.schedulePreloadLocked()
	c.mu.Unlock()
	c.notify()
}

// SetOnline says whether the network is open and Apple reachable. While
// online, AutoPlay is fetched ahead (AutoPlayDelay after the user's queue
// last changed), so its songs follow the user's before they are needed;
// offline (the default) it is fetched only when the user's queue is about
// to end.
func (c *Coordinator) SetOnline(on bool) {
	c.mu.Lock()
	c.online = on
	c.aheadLocked()
	c.mu.Unlock()
}

// dropStationLocked detaches the queue from its station; a refill in
// flight is discarded when it lands.
func (c *Coordinator) dropStationLocked() {
	c.station, c.fill, c.fillFailed, c.autoSeed = nil, nil, time.Time{}, ""
	c.fillGen++
}

// dropAutoPlayLocked detaches the AutoPlay station (discarding an AutoPlay
// fetch in flight) and removes its upcoming slots; a station the user
// started is kept.
func (c *Coordinator) dropAutoPlayLocked() {
	if c.station != nil && !c.station.AutoPlay {
		return
	}
	c.dropStationLocked()
	if c.dropUpcomingAutoLocked() {
		c.queueChangedLocked()
	}
}

// dropUpcomingAutoLocked removes the station slots after the current one
// and reports whether there were any. The caller reports the queue change.
func (c *Coordinator) dropUpcomingAutoLocked() bool {
	from := c.index + 1
	gone := make(map[uint64]bool)
	keep := c.queue[:from]
	for _, e := range c.queue[from:] {
		if e.auto {
			gone[e.uid] = true
		} else {
			keep = append(keep, e)
		}
	}
	if len(gone) == 0 {
		return false
	}
	clear(c.queue[len(keep):])
	c.queue = keep
	if c.shuffle {
		c.original = slices.DeleteFunc(c.original, func(u uint64) bool { return gone[u] })
	}
	return true
}

// wantsAutoPlayLocked reports whether a new AutoPlay station for seed key
// seed is still what the queue needs.
func (c *Coordinator) wantsAutoPlayLocked(seed string) bool {
	if c.index < 0 || !c.autoPlayLocked() || (c.station != nil && !c.staleAutoPlayLocked()) {
		return false
	}
	return seedKey(c.seedsLocked()) == seed
}

// autoPlayLocked reports whether AutoPlay continues the queue: it is on,
// repeat is off and no station the user started feeds the queue.
func (c *Coordinator) autoPlayLocked() bool {
	return c.autoplay && c.repeat == RepeatOff && (c.station == nil || c.station.AutoPlay)
}

// staleAutoPlayLocked reports whether the AutoPlay station no longer
// follows the user's queue: their last song now comes from another
// container (one station per container, so edits within an album or
// playlist keep it). Once an AutoPlay song plays the station stays.
func (c *Coordinator) staleAutoPlayLocked() bool {
	if c.station == nil || !c.station.AutoPlay || c.index < 0 || c.queue[c.index].auto {
		return false
	}
	key := seedKey(c.seedsLocked())
	return key != "" && key != c.autoSeed
}

// aheadSeedLocked returns the seed key AutoPlay should be fetched ahead
// for now, or "" when nothing needs fetching (offline, AutoPlay off or
// already following the user's queue, a refill in flight or backing off).
func (c *Coordinator) aheadSeedLocked() string {
	if !c.online || c.closed || c.stations == nil || c.fill != nil || c.index < 0 || !c.autoPlayLocked() {
		return ""
	}
	if c.station != nil && !c.staleAutoPlayLocked() {
		return ""
	}
	if !c.fillFailed.IsZero() && c.now().Sub(c.fillFailed) < c.retry {
		return ""
	}
	return seedKey(c.seedsLocked())
}

// aheadLocked (re)schedules fetching AutoPlay ahead. It runs whenever the
// queue, the current item or the AutoPlay conditions change; the fetch
// waits AutoPlayDelay after the last change of the seed, so a burst of
// edits makes one request.
func (c *Coordinator) aheadLocked() {
	want := c.aheadSeedLocked()
	if want == c.aheadKey {
		return
	}
	if c.aheadTimer != nil {
		c.aheadTimer.Stop()
		c.aheadTimer = nil
	}
	c.aheadKey = want
	if want != "" {
		c.aheadTimer = time.AfterFunc(c.aheadWait, c.aheadTick)
	}
}

// aheadTick fetches AutoPlay ahead once the queue settled.
func (c *Coordinator) aheadTick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aheadTimer, c.aheadKey = nil, ""
	if c.aheadSeedLocked() != "" {
		c.startFillLocked()
	}
}

// refillLocked starts a background refill when the playing item is
// followed by RefillAt or fewer items and a station (or AutoPlay) can
// continue the queue.
func (c *Coordinator) refillLocked() {
	if c.st.State != playback.StatePlaying && c.st.State != playback.StateLoading {
		return
	}
	if c.index < 0 || len(c.queue)-1-c.index > c.refillAt {
		return
	}
	if !c.fillFailed.IsZero() && c.now().Sub(c.fillFailed) < c.retry {
		return
	}
	c.startFillLocked()
}

// startFillLocked starts (or joins) a refill from the queue's station, or
// from a new AutoPlay station seeded by the user's last item (also when
// the AutoPlay station no longer follows the user's queue). It returns
// nil when nothing can continue the queue.
func (c *Coordinator) startFillLocked() *fillCall {
	if c.fill != nil {
		return c.fill
	}
	if c.stations == nil || c.closed || c.index < 0 {
		return nil
	}
	var (
		seeds []Item
		st    *Station
	)
	if c.station == nil || c.staleAutoPlayLocked() {
		if !c.autoPlayLocked() {
			return nil
		}
		if seeds = c.seedsLocked(); len(seeds) == 0 {
			return nil
		}
	} else {
		s := *c.station
		st = &s
	}
	f := &fillCall{gen: c.fillGen, seed: seedKey(seeds), done: make(chan struct{})}
	c.fill = f
	c.aheadLocked() // a refill in flight: nothing to fetch ahead
	c.wg.Add(1)
	go c.runFill(c.stations, f, st, seeds)
	return f
}

// seedsLocked returns the AutoPlay seed: the user's last song in the queue,
// in the unshuffled order while shuffled (stations and radio items, "ra.…",
// cannot seed).
func (c *Coordinator) seedsLocked() []Item {
	// Stations and music videos seed no AutoPlay station (Apple makes one
	// from a song).
	seed := func(e entry) bool {
		return !e.auto && !strings.HasPrefix(e.item.ID, "ra.") && !playback.IsVideo(e.item.ID)
	}
	if c.shuffle {
		for i := len(c.original) - 1; i >= 0; i-- {
			if j := c.indexOfLocked(c.original[i]); j >= 0 && seed(c.queue[j]) {
				return []Item{c.queue[j].item}
			}
		}
		return nil
	}
	for i := len(c.queue) - 1; i >= 0; i-- {
		if seed(c.queue[i]) {
			return []Item{c.queue[i].item}
		}
	}
	return nil
}

// seedKey identifies the AutoPlay station seeds lead to: the container the
// last seed was queued from, else the song itself. "" without seeds.
func seedKey(seeds []Item) string {
	if len(seeds) == 0 {
		return ""
	}
	it := seeds[len(seeds)-1]
	if it.Container.ID != "" {
		return string(it.Container.Kind) + ":" + it.Container.ID
	}
	return "song:" + it.ID
}

// runFill fetches one batch for f (creating the AutoPlay station first
// when st is nil) and appends it as station slots, unless the queue moved
// to another station meanwhile. A new AutoPlay station replaces the
// upcoming slots of the one it follows on from.
func (c *Coordinator) runFill(src StationSource, f *fillCall, st *Station, seeds []Item) {
	defer c.wg.Done()
	defer close(f.done)
	var (
		station Station
		items   []Item
		err     error
	)
	if st != nil {
		station = *st
	} else {
		station, err = src.Continue(c.ctx, seeds)
		station.AutoPlay = true
	}
	if err == nil {
		items, err = src.NextTracks(c.ctx, station.ID)
	}
	if err == nil && len(items) == 0 {
		err = fmt.Errorf("station %s returned no tracks", station.ID)
	}
	c.mu.Lock()
	if c.fill == f {
		c.fill = nil
	}
	if c.closed || f.gen != c.fillGen {
		c.mu.Unlock()
		return
	}
	if st == nil && !c.wantsAutoPlayLocked(f.seed) {
		// The user's queue moved on meanwhile: fetch again for it.
		c.aheadLocked()
		c.mu.Unlock()
		return
	}
	if err != nil {
		c.fillFailed = c.now()
		c.mu.Unlock()
		if c.ctx.Err() == nil {
			c.log.Warn("coordinator: station refill", "station", station.ID, "err", err)
		}
		return
	}
	c.fillFailed = time.Time{}
	if st == nil {
		c.dropUpcomingAutoLocked()
		c.station, c.autoSeed = &station, f.seed
	}
	es := c.entriesLocked(fromStation(items, station.ID), true)
	c.queue = append(c.queue, es...)
	if c.shuffle {
		c.original = append(c.original, uids(es)...)
	}
	c.trimPlayedLocked()
	c.queueChangedLocked()
	c.mu.Unlock()
	c.notify()
}

// trimPlayedLocked drops the oldest played station slots beyond
// maxPlayedAuto.
func (c *Coordinator) trimPlayedLocked() {
	played := 0
	for i := range c.index {
		if c.queue[i].auto {
			played++
		}
	}
	for i := 0; played > maxPlayedAuto && i < c.index; {
		if c.queue[i].auto {
			c.removeLocked(i)
			played--
			continue
		}
		i++
	}
}

// awaitFillLocked is called with cmd and mu held when nothing follows the
// current item: it waits (mu released) up to FillWait for a station
// refill and returns, with mu held, the item to play next.
func (c *Coordinator) awaitFillLocked(ctx context.Context, auto bool) (int, bool) {
	f := c.startFillLocked()
	if f == nil {
		return -1, false
	}
	uid := c.queue[c.index].uid
	c.mu.Unlock()
	c.notify()
	t := time.NewTimer(c.fillWait)
	select {
	case <-f.done:
	case <-ctx.Done():
	case <-t.C:
	}
	t.Stop()
	c.mu.Lock()
	if c.index < 0 || c.queue[c.index].uid != uid {
		return -1, false
	}
	return c.nextIndexLocked(auto)
}

// schedulePreloadLocked arranges for the next item to be preloaded
// PreloadLead before the current one ends, extrapolating the position
// from the last status rather than relying on periodic events. It runs
// whenever the current item, its status or the queue changes, so it also
// reschedules the sleep timer.
func (c *Coordinator) schedulePreloadLocked() {
	if c.preloadTimer != nil {
		c.preloadTimer.Stop()
		c.preloadTimer = nil
	}
	c.scheduleSleepLocked()
	c.armLocked()
	if !c.closed {
		c.refillLocked()
	}
	if c.closed || c.index < 0 || !c.loaded || c.st.State != playback.StatePlaying || c.st.Duration <= 0 {
		return
	}
	ni, ok := c.nextIndexLocked(true)
	if !ok || c.sleepEndsHereLocked() {
		return
	}
	next, cur := c.queue[ni], c.queue[c.index]
	if next.uid == c.preloaded || next.item.ID == cur.item.ID {
		return
	}
	if delay := c.st.Duration - c.positionLocked(c.now()) - c.lead; delay > 0 {
		c.preloadTimer = time.AfterFunc(delay, func() {
			c.mu.Lock()
			c.schedulePreloadLocked()
			c.mu.Unlock()
		})
		return
	}
	c.preloaded = next.uid
	id := next.item.ID
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.eng.Preload(c.ctx, id); err != nil && c.ctx.Err() == nil {
			if errors.Is(err, playback.ErrUnsupported) {
				c.log.Debug("coordinator: preload", "id", id, "err", err)
			} else {
				c.log.Warn("coordinator: preload", "id", id, "err", err)
			}
		}
	}()
}

// refresh adopts the engine's status after a command on entry uid.
func (c *Coordinator) refresh(uid uint64) {
	st := c.eng.Status()
	c.mu.Lock()
	if c.index >= 0 && c.loaded && c.queue[c.index].uid == uid && st.ItemID == c.queue[c.index].item.ID {
		c.applyLocked(st)
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Coordinator) stopEngine(ctx context.Context, loaded bool) error {
	if !loaded {
		return nil
	}
	if err := c.eng.Stop(ctx); err != nil && !superseded(ctx) {
		return fmt.Errorf("coordinator: stop: %w", err)
	}
	return nil
}

func (c *Coordinator) entriesLocked(items []Item, auto bool) []entry {
	es := make([]entry, len(items))
	for i, it := range items {
		c.nextUID++
		es[i] = entry{uid: c.nextUID, item: it, auto: auto}
	}
	return es
}

func (c *Coordinator) indexOfLocked(uid uint64) int {
	return slices.IndexFunc(c.queue, func(e entry) bool { return e.uid == uid })
}

// removeLocked deletes queue[i], keeping index on the same entry (or, when
// the current entry is removed, on the one that takes its place).
func (c *Coordinator) removeLocked(i int) {
	uid := c.queue[i].uid
	c.queue = slices.Delete(c.queue, i, i+1)
	if c.shuffle {
		c.original = slices.DeleteFunc(c.original, func(u uint64) bool { return u == uid })
	}
	if i < c.index {
		c.index--
	}
	c.queueChangedLocked()
}

func (c *Coordinator) queueChangedLocked() {
	c.queueVersion++
	c.aheadLocked()
	c.schedulePreloadLocked()
}

// notify publishes the latest snapshot and schedules a save.
func (c *Coordinator) notify() {
	c.subMu.Lock()
	if len(c.subs) > 0 {
		s := c.Snapshot()
		for ch := range c.subs {
			select {
			case ch <- s:
			default:
				select {
				case <-ch:
				default:
				}
				ch <- s // buffer is free: only notify sends, under subMu
			}
		}
	}
	c.subMu.Unlock()
	c.mu.Lock()
	c.scheduleSaveLocked()
	c.mu.Unlock()
	c.deliverActivity()
}

func (c *Coordinator) scheduleSaveLocked() {
	if c.store == nil || c.closed || c.savePending {
		return
	}
	c.savePending = true
	if c.saveTimer == nil {
		c.saveTimer = time.AfterFunc(c.saveDelay, c.saveTick)
		return
	}
	c.saveTimer.Reset(c.saveDelay)
}

func (c *Coordinator) saveTick() {
	c.mu.Lock()
	c.savePending = false
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}
	if err := c.save(); err != nil {
		c.log.Warn("coordinator: save queue", "err", err)
	}
}

func (c *Coordinator) save() error {
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	c.mu.Lock()
	s := c.savedLocked()
	c.mu.Unlock()
	return c.store.SaveQueue(context.Background(), s)
}

func (c *Coordinator) savedLocked() Saved {
	now := c.now()
	s := Saved{
		Items:     make([]Item, len(c.queue)),
		Index:     c.index,
		Position:  c.positionLocked(now),
		PlayedFor: c.listenedLocked(now),
		Shuffle:   c.shuffle,
		Repeat:    c.repeat,
		Context:   c.label,
		Volume:    c.volume,
	}
	pos := make(map[uint64]int, len(c.queue))
	for i, e := range c.queue {
		s.Items[i] = e.item
		pos[e.uid] = i
		if e.auto {
			s.Auto = append(s.Auto, i)
		}
		if e.added {
			s.Added = append(s.Added, i)
		}
	}
	if c.station != nil {
		st := *c.station
		s.Station = &st
	}
	if c.shuffle {
		for _, u := range c.original {
			s.Original = append(s.Original, pos[u])
		}
	}
	if c.index < 0 {
		s.Position, s.PlayedFor = 0, 0
	}
	return s
}

func uids(es []entry) []uint64 {
	out := make([]uint64, len(es))
	for i, e := range es {
		out[i] = e.uid
	}
	return out
}

// isPerm reports whether p is a permutation of [0, n).
func isPerm(p []int, n int) bool {
	if len(p) != n {
		return false
	}
	seen := make([]bool, n)
	for _, i := range p {
		if i < 0 || i >= n || seen[i] {
			return false
		}
		seen[i] = true
	}
	return true
}

func run(fx func()) {
	if fx != nil {
		fx()
	}
}
