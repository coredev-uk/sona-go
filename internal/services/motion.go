package services

import (
	"context"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/power"
)

// MotionService is motion artwork outside detail pages (whose Header
// carries it): the now-playing cover's, the power source, which turns
// motion artwork off on battery, and what Diagnostics shows about it.
type MotionService struct {
	e *Env

	mu         sync.Mutex
	mon        PowerMonitor
	unreadable bool // the power monitor failed to start
	closed     bool
	report     *MotionReport
}

func newMotionService(e *Env) *MotionService {
	s := &MotionService{e: e}
	// The power source (UPower on Linux) is read off the start-up path;
	// until it answers, or where the platform has no monitor, the
	// computer counts as on mains power.
	e.Go("power monitor", s.watch)
	e.OnClose(s.close)
	e.OnDiagnostics("Motion artwork", s.bundle)
	return s
}

func (s *MotionService) watch() {
	if s.e.Platform.Power == nil {
		return
	}
	mon, err := s.e.Platform.Power()
	if err != nil {
		s.e.Logger.Info("power: power source unavailable, motion artwork follows the setting only", "err", err)
		s.mu.Lock()
		s.unreadable = true
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = mon.Close()
		return
	}
	s.mon = mon
	s.mu.Unlock()
	st := mon.State()
	s.e.Logger.Info("power: source", "source", st.Source, "on_battery", st.OnBattery, "upower_on_battery", st.UPower, "ignored", st.Ignored)
	emit := func(st power.State) {
		s.e.Logger.Info("power: source changed", "source", st.Source, "on_battery", st.OnBattery, "upower_on_battery", st.UPower)
		s.e.Emit.Emit(EventPower, followed(st))
	}
	if st.OnBattery {
		emit(st)
	}
	mon.Run(emit)
}

func followed(st power.State) PowerState {
	return PowerState{Followed: true, OnBattery: st.OnBattery, Source: st.Source, UPower: st.UPower, Ignored: st.Ignored}
}

func (s *MotionService) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.mon != nil {
		_ = s.mon.Close()
	}
}

// Power returns the computer's power source ("power:changed" follows it).
func (s *MotionService) Power() PowerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mon == nil {
		return PowerState{Unreadable: s.unreadable}
	}
	return followed(s.mon.State())
}

// Status reports the power source, the motion cache and its newest
// download (Settings → Diagnostics).
func (s *MotionService) Status() MotionStatus {
	out := MotionStatus{Power: s.Power()}
	if s.e.Motion == nil {
		return out
	}
	st := s.e.Motion.Stats()
	out.CacheFiles, out.CacheBytes = st.Files, st.Bytes
	if !st.Last.At.IsZero() {
		out.Last = MotionFetch{
			At: st.Last.At.UnixMilli(), Size: st.Last.Size, Codec: st.Last.Codec,
			Rendition: st.Last.Rendition, Error: CodeOf(st.Last.Err),
		}
	}
	return out
}

// Report keeps the native UI's side of motion artwork for the next
// bug-report bundle (under the key "webView", which bug reports have
// always carried).
func (s *MotionService) Report(r MotionReport) {
	s.mu.Lock()
	s.report = &r
	s.mu.Unlock()
}

// bundle is the bug-report bundle's motion artwork section.
func (s *MotionService) bundle() any {
	out := struct {
		Sona    MotionStatus  `json:"sona"`
		WebView *MotionReport `json:"webView"`
	}{Sona: s.Status()}
	s.mu.Lock()
	out.WebView = s.report
	s.mu.Unlock()
	return out
}

// Album returns the motion cover of album id (catalog, or library "l.…"
// through its catalog album) as a Go-served URL, "" when it has none. It
// reads the album page through the catalog cache, so an album whose page
// was opened costs nothing.
func (s *MotionService) Album(ctx context.Context, id string) (string, error) {
	if applemusic.IsLibraryAlbumID(id) {
		a, _, err := s.e.Library.Album(ctx, id)
		if err != nil {
			return "", err
		}
		id = a.CatalogID
	}
	if id == "" || strings.Contains(id, ".") {
		return "", nil
	}
	a, err := s.e.Catalog.Album(ctx, id)
	if err != nil {
		return "", s.e.Fail(err)
	}
	return s.e.m.motionOf(a.Attributes.EditorialVideo), nil
}
