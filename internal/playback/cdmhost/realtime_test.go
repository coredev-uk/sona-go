//go:build linux || (darwin && cgo)

package cdmhost

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// fakeBroker records what it was asked; failRT and failNice refuse those
// calls.
type fakeBroker struct {
	lim              rtLimits
	failRT, failNice error

	mu       sync.Mutex
	realtime []string // "pid/tid@priority"
	nice     []string // "pid/tid@nice"
	closed   int
}

func (b *fakeBroker) name() string     { return "RTKit" }
func (b *fakeBroker) limits() rtLimits { return b.lim }

func (b *fakeBroker) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
}

func (b *fakeBroker) makeRealtime(pid, tid, priority int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failRT != nil {
		return b.failRT
	}
	b.realtime = append(b.realtime, fmt.Sprintf("%d/%d@%d", pid, tid, priority))
	return nil
}

func (b *fakeBroker) makeHighPriority(pid, tid, nice int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failNice != nil {
		return b.failNice
	}
	b.nice = append(b.nice, fmt.Sprintf("%d/%d@%d", pid, tid, nice))
	return nil
}

func (b *fakeBroker) calls() (realtime, nice []string, closed int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.realtime), slices.Clone(b.nice), b.closed
}

// rtkitDefaults are RTKit's default limits.
var rtkitDefaults = rtLimits{maxPriority: 20, minNice: -15, rttimeMax: 200000}

// fakeRTEnv is an rtEnv over b, owning the threads below 1000 of every
// process, that records each RLIMIT_RTTIME it sets ("pid:usec").
type fakeRTEnv struct {
	b                  *fakeBroker
	dialErr, rttimeErr error

	mu     sync.Mutex
	rttime []string
	dials  int
}

func (f *fakeRTEnv) env() rtEnv {
	return rtEnv{
		dial: func() (rtBroker, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.dials++
			if f.dialErr != nil {
				return nil, f.dialErr
			}
			return f.b, nil
		},
		limitRTTime: func(pid int, usec int64) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.rttime = append(f.rttime, fmt.Sprintf("%d:%d", pid, usec))
			return f.rttimeErr
		},
		owns: func(_, tid int) bool { return tid < 1000 },
	}
}

func TestRaise(t *testing.T) {
	refused := errors.New("Access denied")
	for _, tc := range []struct {
		name                string
		lim                 rtLimits
		dialErr, rttimeErr  error
		failRT, failNice    error
		tids                []int
		want                Realtime
		wantRT, wantNice    []string
		wantRTTime          []string
		wantDials, wantShut int
	}{{
		name:       "granted at RTKit's maximum after capping RLIMIT_RTTIME",
		lim:        rtkitDefaults,
		tids:       []int{10, 11},
		want:       Realtime{State: playback.RealtimeGranted, Via: "RTKit", Priority: 20, Threads: 2},
		wantRT:     []string{"7/10@20", "7/11@20"},
		wantRTTime: []string{"7:200000"},
		wantDials:  1, wantShut: 1,
	}, {
		name:       "a broker allowing more gets PipeWire's client priority",
		lim:        rtLimits{maxPriority: 99, minNice: -20, rttimeMax: 50000},
		tids:       []int{10},
		want:       Realtime{State: playback.RealtimeGranted, Via: "RTKit", Priority: clientPriority, Threads: 1},
		wantRT:     []string{"7/10@83"},
		wantRTTime: []string{"7:50000"},
		wantDials:  1, wantShut: 1,
	}, {
		name:       "realtime refused: a raised nice level instead",
		lim:        rtkitDefaults,
		failRT:     refused,
		tids:       []int{10, 11},
		want:       Realtime{State: playback.RealtimeRaised, Via: "RTKit", Priority: -15, Threads: 2, Reason: "RTKit: Access denied"},
		wantNice:   []string{"7/10@-15", "7/11@-15"},
		wantRTTime: []string{"7:200000"},
		wantDials:  1, wantShut: 1,
	}, {
		name:       "both refused",
		lim:        rtkitDefaults,
		failRT:     refused,
		failNice:   errors.New("not allowed"),
		tids:       []int{10},
		want:       Realtime{State: playback.RealtimeDenied, Via: "RTKit", Reason: "RTKit: Access denied; RTKit: not allowed"},
		wantRTTime: []string{"7:200000"},
		wantDials:  1, wantShut: 1,
	}, {
		name:       "the same refusal twice is named once",
		lim:        rtkitDefaults,
		failRT:     refused,
		failNice:   refused,
		tids:       []int{10, 11},
		want:       Realtime{State: playback.RealtimeDenied, Via: "RTKit", Reason: "RTKit: Access denied"},
		wantRTTime: []string{"7:200000"},
		wantDials:  1, wantShut: 1,
	}, {
		name:       "RLIMIT_RTTIME not lowered: realtime is not asked for",
		lim:        rtkitDefaults,
		rttimeErr:  errors.New("operation not permitted"),
		tids:       []int{10},
		want:       Realtime{State: playback.RealtimeRaised, Via: "RTKit", Priority: -15, Threads: 1, Reason: "limit the host's realtime CPU time: operation not permitted"},
		wantNice:   []string{"7/10@-15"},
		wantRTTime: []string{"7:200000"},
		wantDials:  1, wantShut: 1,
	}, {
		name:      "a broker allowing neither",
		lim:       rtLimits{},
		tids:      []int{10},
		want:      Realtime{State: playback.RealtimeDenied, Via: "RTKit", Reason: "RTKit allows no realtime priority"},
		wantDials: 1, wantShut: 1,
	}, {
		name:      "no broker",
		dialErr:   errors.New("RTKit: no system bus; portal: no session bus"),
		tids:      []int{10},
		want:      Realtime{State: playback.RealtimeUnavailable, Reason: "RTKit: no system bus; portal: no session bus"},
		wantDials: 1,
	}, {
		name:   "threads of another process are skipped",
		lim:    rtkitDefaults,
		tids:   []int{1000, 10},
		want:   Realtime{State: playback.RealtimeGranted, Via: "RTKit", Priority: 20, Threads: 1},
		wantRT: []string{"7/10@20"}, wantRTTime: []string{"7:200000"},
		wantDials: 1, wantShut: 1,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			b := &fakeBroker{lim: tc.lim, failRT: tc.failRT, failNice: tc.failNice}
			f := &fakeRTEnv{b: b, dialErr: tc.dialErr, rttimeErr: tc.rttimeErr}
			got, ok := f.env().raise(7, tc.tids)
			if !ok || got != tc.want {
				t.Errorf("raise = %+v, %v; want %+v", got, ok, tc.want)
			}
			rt, nice, closed := b.calls()
			if !slices.Equal(rt, tc.wantRT) || !slices.Equal(nice, tc.wantNice) {
				t.Errorf("realtime %q, nice %q; want %q, %q", rt, nice, tc.wantRT, tc.wantNice)
			}
			if !slices.Equal(f.rttime, tc.wantRTTime) {
				t.Errorf("RLIMIT_RTTIME %q, want %q", f.rttime, tc.wantRTTime)
			}
			if f.dials != tc.wantDials || closed != tc.wantShut {
				t.Errorf("dialled %d, closed %d; want %d, %d", f.dials, closed, tc.wantDials, tc.wantShut)
			}
		})
	}
}

// A report naming no thread of the child asks no one.
func TestRaiseSkipsForeignThreads(t *testing.T) {
	f := &fakeRTEnv{b: &fakeBroker{lim: rtkitDefaults}}
	if rt, ok := f.env().raise(7, []int{1000, 2000}); ok {
		t.Fatalf("raise = %+v, true", rt)
	}
	if f.dials != 0 {
		t.Fatalf("dialled %d times", f.dials)
	}
}

func TestRealtimeMerge(t *testing.T) {
	granted := Realtime{State: playback.RealtimeGranted, Via: "RTKit", Priority: 20, Threads: 1}
	denied := Realtime{State: playback.RealtimeDenied, Via: "RTKit", Reason: "RTKit: Access denied"}
	if got := granted.merge(denied); got != granted {
		t.Errorf("a refusal after a grant: %+v", got)
	}
	if got := granted.merge(granted); got.Threads != 2 || got.State != playback.RealtimeGranted {
		t.Errorf("two grants: %+v", got)
	}
	if got := (Realtime{}).merge(denied); got != denied {
		t.Errorf("a first refusal: %+v", got)
	}
}

// testHost is a Host for process pid with env, without a child.
func testHost(pid int, env rtEnv) *Host {
	return &Host{
		log:     slog.New(slog.DiscardHandler),
		cmd:     &exec.Cmd{Process: &os.Process{Pid: pid}},
		rtEnv:   env,
		rt:      realtimeInitial,
		rtAsked: map[int]bool{},
	}
}

// waitRealtime waits until h's Realtime satisfies done.
func waitRealtime(t *testing.T, h *Host, done func(Realtime) bool) Realtime {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		rt := h.Info().Realtime
		if done(rt) {
			return rt
		}
		if time.Now().After(deadline) {
			t.Fatalf("Realtime stayed %+v", rt)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A child gets at most maxRealtimeThreads threads raised, each once,
// whatever it reports.
func TestAudioThreadsBounded(t *testing.T) {
	b := &fakeBroker{lim: rtkitDefaults}
	f := &fakeRTEnv{b: b}
	h := testHost(7, f.env())
	var tids []int
	for tid := 1; tid <= 3*maxRealtimeThreads; tid++ {
		tids = append(tids, tid)
	}
	h.audioThreads([]int{1, 1, 0, -5})
	h.audioThreads(tids)
	h.audioThreads(tids)
	waitRealtime(t, h, func(rt Realtime) bool { return rt.Threads == maxRealtimeThreads })
	h.rtReq.Lock() // every request has finished
	defer h.rtReq.Unlock()
	rt, _, _ := b.calls()
	if len(rt) != maxRealtimeThreads {
		t.Fatalf("asked for %d threads: %q", len(rt), rt)
	}
	slices.Sort(rt)
	if len(slices.Compact(rt)) != maxRealtimeThreads {
		t.Fatalf("a thread was asked for twice: %q", rt)
	}
}
