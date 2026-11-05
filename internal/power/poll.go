package power

import (
	"sync"
	"time"
)

// DefaultPollEvery is how often a Poller reads the power source when
// PollOptions.Every is zero. Reading is a cheap local call, and the answer
// only has to be as fresh as unplugging a laptop is noticed: motion artwork
// pausing a few seconds late is invisible.
const DefaultPollEvery = 10 * time.Second

// PollOptions configure a Poller.
type PollOptions struct {
	// OnBattery reads the power source now: true when the computer runs on
	// its battery. It may be slow-ish (macOS asks the main thread) but never
	// blocks for long, and it is called from Run's goroutine and once from
	// NewPoller.
	OnBattery func() bool
	// Every is the time between reads (DefaultPollEvery when zero).
	Every time.Duration
	// Ticks replaces the timer: each value is one scheduled read. Tests send
	// on it; production leaves it nil.
	Ticks <-chan time.Time
}

// Poller follows a power source that can be read but not subscribed to
// (macOS: MyGo reports whether the Mac is on battery, not when that changes)
// by reading it on a timer and when told to (Refresh, after the Mac wakes).
// It is a services.PowerMonitor.
type Poller struct {
	read  func() bool
	ticks <-chan time.Time
	stop  func() // stops the timer; nil when Ticks was given
	wake  chan struct{}
	done  chan struct{}
	once  sync.Once

	mu    sync.Mutex
	state State
}

// NewPoller reads the power source once and returns a Poller that holds it.
// Run follows it from there.
func NewPoller(o PollOptions) *Poller {
	p := &Poller{
		read:  o.OnBattery,
		ticks: o.Ticks,
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
	}
	if p.ticks == nil {
		every := o.Every
		if every <= 0 {
			every = DefaultPollEvery
		}
		t := time.NewTicker(every)
		p.ticks, p.stop = t.C, t.Stop
	}
	p.state = pollState(o.OnBattery())
	return p
}

// pollState is the State of a computer a poller found on or off battery.
// A poller has no UPower: it repeats OnBattery there so Diagnostics, which
// notes where UPower and Sona's decision disagree, has nothing to note.
func pollState(onBattery bool) State {
	if onBattery {
		return State{OnBattery: true, Source: SourceBattery, UPower: true}
	}
	return State{Source: SourceAC}
}

// State returns the power source as last read.
func (p *Poller) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Refresh asks Run to read the source now, without waiting for the next
// tick. It never blocks, and several calls before Run reads count as one.
func (p *Poller) Refresh() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Run reads the source on every tick and Refresh, and calls fn with the new
// State whenever it changed, until Close.
func (p *Poller) Run(fn func(State)) {
	for {
		select {
		case <-p.done:
			return
		case <-p.ticks:
		case <-p.wake:
		}
		st := pollState(p.read())
		p.mu.Lock()
		changed := st != p.state
		p.state = st
		p.mu.Unlock()
		if changed {
			select {
			case <-p.done: // closed while reading: say nothing more
				return
			default:
			}
			fn(st)
		}
	}
}

// Close stops the timer and makes Run return. It is safe to call twice.
func (p *Poller) Close() error {
	p.once.Do(func() {
		close(p.done)
		if p.stop != nil {
			p.stop()
		}
	})
	return nil
}
