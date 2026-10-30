package services

import (
	"context"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/power"
)

// MotionService is motion artwork outside detail pages (whose Header
// carries it): the now-playing cover's, and whether the machine runs on
// battery, which turns motion artwork off in the UI.
type MotionService struct {
	e *Env

	mu     sync.Mutex
	mon    *power.Monitor
	closed bool
}

func newMotionService(e *Env) *MotionService {
	s := &MotionService{e: e}
	// UPower is read off the start-up path; until it answers the machine
	// counts as on mains power.
	e.Go("power monitor", s.watch)
	e.OnClose(s.close)
	return s
}

func (s *MotionService) watch() {
	mon, err := power.Open()
	if err != nil {
		s.e.Logger.Info("power: on-battery state unavailable, motion artwork follows the setting only", "err", err)
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
	emit := func(on bool) { s.e.Emit.Emit(EventPower, PowerState{OnBattery: on}) }
	if mon.OnBattery() {
		emit(true)
	}
	mon.Run(emit)
}

func (s *MotionService) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.mon != nil {
		_ = s.mon.Close()
	}
}

// Power returns whether the machine runs on battery ("power:changed"
// follows it).
func (s *MotionService) Power() PowerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return PowerState{OnBattery: s.mon != nil && s.mon.OnBattery()}
}

// Album returns the motion cover of album id (catalog, or library "l.…"
// through its catalog album) as a Go-served URL, "" when it has none. It
// reads the album page through the catalog cache, so an album whose page
// was opened costs nothing.
func (s *MotionService) Album(ctx context.Context, id string) (string, error) {
	if strings.HasPrefix(id, "l.") {
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
