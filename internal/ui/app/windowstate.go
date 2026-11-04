package app

import (
	"log/slog"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
)

// saveDelay is how long the main window's size must stay put before it is
// written to the settings: a drag-resize reports every step.
const saveDelay = 500 * time.Millisecond

// observeWindow folds one observation of the main window into the
// remembered state. The size is only taken while the window is neither
// maximised nor fullscreen, so un-maximising after the next start returns
// to it; fullscreen is transient and changes nothing.
func observeWindow(prev config.Window, width, height int, maximised, fullscreen bool) config.Window {
	if fullscreen {
		return prev
	}
	next := prev
	next.Maximised = maximised
	if !maximised && width > 0 && height > 0 {
		next.Width, next.Height = width, height
	}
	return next
}

// windowMemory keeps the main window's remembered size and maximised
// state (config.Settings.Window, where v0.2.0 kept it) and writes it to the
// settings a moment after it last changed.
type windowMemory struct {
	settings *config.Store
	log      *slog.Logger

	mu    sync.Mutex
	win   config.Window
	timer *time.Timer
}

func newWindowMemory(s *config.Store, log *slog.Logger) *windowMemory {
	return &windowMemory{settings: s, log: log, win: s.Get().Window}
}

// current is the remembered state.
func (m *windowMemory) current() config.Window {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.win
}

// observe records the window as it is now.
func (m *windowMemory) observe(width, height int, maximised, fullscreen bool) {
	m.mu.Lock()
	next := observeWindow(m.win, width, height, maximised, fullscreen)
	changed := next != m.win
	m.win = next
	m.mu.Unlock()
	if changed {
		m.schedule()
	}
}

// reset returns the remembered state to the default size, now and for the
// next start.
func (m *windowMemory) reset() {
	m.mu.Lock()
	m.win = config.Window{Width: config.DefaultWindowWidth, Height: config.DefaultWindowHeight}
	m.mu.Unlock()
	m.schedule()
}

func (m *windowMemory) schedule() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(saveDelay, m.save)
}

func (m *windowMemory) save() {
	w := m.current()
	if _, err := m.settings.Update(func(st *config.Settings) { st.Window = w }); err != nil {
		m.log.Warn("shell: save window size", "err", err)
	}
}

// flush writes a pending change at once (the app is closing).
func (m *windowMemory) flush() {
	m.mu.Lock()
	pending := m.timer != nil && m.timer.Stop()
	m.timer = nil
	m.mu.Unlock()
	if pending {
		m.save()
	}
}
