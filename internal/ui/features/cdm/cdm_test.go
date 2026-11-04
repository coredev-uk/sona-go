package cdm

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

type rig struct {
	h  *uitest.Harness
	tt *uitest.Session
}

func newRig(t *testing.T, routes bool, opts ...uitest.Option) *rig {
	t.Helper()
	h := uitest.New(t, opts...)
	h.Env.HasRoute = func(p string) bool { return routes && p == "/settings" }
	m := New(h.Env)
	m.Init()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Row(c).Fill().Padding(8).Children(func() { h.Env.PlayerNotice.Build(c) })
	}, 600, 100)
	tt.Drain()
	return &rig{h, tt}
}

func (r *rig) emit(s services.CDMStatus) {
	r.h.Bus.Emit(services.EventCDMStatus, s)
	r.tt.Drain()
}

func (r *rig) has(t *testing.T, want string) {
	t.Helper()
	if !r.tt.HasText(want) {
		t.Errorf("notice lacks %q: %q", want, r.tt.Texts())
	}
}

func TestNoticeWordsEachState(t *testing.T) {
	for _, routes := range []bool{true, false} { // a link, or plain text until Settings is ported
		r := newRig(t, routes, uitest.Bare())
		// Before Go has reported anything: waiting, with no percent.
		r.has(t, "Waiting for Widevine to download")
		if r.tt.HasText("%") {
			t.Errorf("a percent before any status: %q", r.tt.Texts())
		}
		r.emit(services.CDMStatus{State: "downloading", Received: 42, Total: 100})
		r.has(t, "Waiting for Widevine to download · 42%")
		r.emit(services.CDMStatus{State: "downloading", Received: 42}) // size unknown
		r.has(t, "Waiting for Widevine to download")
		if r.tt.HasText("%") {
			t.Errorf("a percent with no total: %q", r.tt.Texts())
		}
		r.emit(services.CDMStatus{State: "idle"})
		r.has(t, "Waiting for Widevine to download")
		r.emit(services.CDMStatus{State: "failed", Code: "network"})
		r.has(t, "Couldn't get Widevine · Retry in Settings")
		r.emit(services.CDMStatus{State: "failed", Code: "unsupported_platform"})
		r.has(t, "Songs can't play on this computer")
		r.emit(services.CDMStatus{State: "ready", Version: "4.10"})
		r.has(t, "Widevine is ready. Press play to start.")
		if r.tt.HasText("Waiting") || r.tt.HasText("Couldn't") {
			t.Errorf("a ready CDM still waits: %q", r.tt.Texts())
		}
	}
}

func TestOnlyAStateVersionOrCodeChangeRefreshesDiagnostics(t *testing.T) {
	r := newRig(t, true, uitest.Bare())
	var fetches atomic.Int32
	diag := func() {
		store.Use(r.h.Env.Queries, store.Key{"diagnostics"}, func(context.Context) (int, error) {
			return int(fetches.Add(1)), nil
		}, store.Options{Stale: store.Forever})
	}
	waitFetches := func(n int32) {
		t.Helper()
		r.tt.Until("the diagnostics to be fetched", func() bool { diag(); return fetches.Load() >= n })
	}
	waitFetches(1)
	r.emit(services.CDMStatus{State: "downloading", Version: "1", Received: 1, Total: 100})
	waitFetches(2) // the first status
	for i := range 5 {
		r.emit(services.CDMStatus{State: "downloading", Version: "1", Received: int64(10 + i), Total: 100})
	}
	time.Sleep(50 * time.Millisecond)
	diag()
	r.tt.Drain()
	if n := fetches.Load(); n != 2 {
		t.Errorf("progress steps refreshed the diagnostics: %d fetches", n)
	}
	r.emit(services.CDMStatus{State: "ready", Version: "1"})
	waitFetches(3)
	r.emit(services.CDMStatus{State: "ready", Version: "2"})
	waitFetches(4)
	r.emit(services.CDMStatus{State: "ready", Version: "2", Code: "network"})
	waitFetches(5)
}

func TestSnapshotFillsTheMirrorOnlyWhileEmpty(t *testing.T) {
	h := uitest.New(t) // a real CDMService: no CDM engine, it reports nothing ("")
	New(h.Env).Init()
	want := services.CDMStatus{State: "failed", Code: "network"}
	h.Bus.Emit(services.EventCDMStatus, want)
	time.Sleep(50 * time.Millisecond) // the snapshot has long been posted behind it
	h.Main.Drain()
	if got, ok := store.Peek[services.CDMStatus](h.Env.Queries, store.KeyCDM); !ok || got != want {
		t.Errorf("mirror = %+v, %v; want the event's %+v", got, ok, want)
	}

	// With no event, the snapshot fills it: "" means this platform needs no CDM.
	h2 := uitest.New(t)
	New(h2.Env).Init()
	tt := h2.Tester(t, func(*ui.Context) {}, 100, 100)
	tt.Until("the snapshot", func() bool {
		_, ok := store.Peek[services.CDMStatus](h2.Env.Queries, store.KeyCDM)
		return ok
	})
}

// navigator records what the notice asked of the router.
type navigator struct {
	module.Navigator
	pushed []string
}

func (n *navigator) Push(p string) { n.pushed = append(n.pushed, p) }

func TestNoticeLinkOpensSettingsPlaybackThroughTheRouter(t *testing.T) {
	h := uitest.New(t, uitest.Bare())
	h.Env.HasRoute = func(p string) bool { return p == "/settings" }
	nav := &navigator{}
	h.Env.Nav = nav
	New(h.Env).Init()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Row(c).Fill().Padding(8).Children(func() { h.Env.PlayerNotice.Build(c) })
	}, 600, 100)
	h.Bus.Emit(services.EventCDMStatus, services.CDMStatus{State: "failed", Code: "network"})
	tt.Drain()
	if err := tt.Click("Couldn't get Widevine · Retry in Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Drain()
	if len(nav.pushed) != 1 || nav.pushed[0] != "/settings?section=playback" {
		t.Errorf("router got %v, want one push of /settings?section=playback", nav.pushed)
	}
	// Without the page the line is text: nothing to click through to.
	h.Env.HasRoute = func(string) bool { return false }
	tt.Drain()
	_ = tt.Click("Couldn't get Widevine · Retry in Settings")
	if len(nav.pushed) != 1 {
		t.Errorf("plain text pushed %v", nav.pushed)
	}
}
