package playlist

import (
	"sync"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
)

// prefs is the sidebar's remembered state: which folders are open and which
// sections are folded (v0.2.0 kept them in localStorage under
// sona.playlist-folders.open and sona.sidebar.collapsed; here they are
// config.Settings.Sidebar). The in-memory state is what the views read; each
// change is handed to save, which writes it to the settings.
type prefs struct {
	save func(config.Sidebar)

	mu        sync.Mutex
	open      map[string]bool
	collapsed map[string]bool
}

// newPrefs starts from what the settings hold; save (nil: none) is told the
// new state after every change.
func newPrefs(from config.Sidebar, save func(config.Sidebar)) *prefs {
	p := &prefs{save: save, open: map[string]bool{}, collapsed: map[string]bool{}}
	for _, id := range from.OpenFolders {
		p.open[id] = true
	}
	for _, id := range from.FoldedSections {
		p.collapsed[id] = true
	}
	return p
}

// settingsPrefs is the prefs of the app: read from the settings now, saved
// through SystemService.SetSidebar off the main thread, one save at a time and
// the newest state last (a failed save keeps the state for this session).
func settingsPrefs(sys *services.SystemService) *prefs {
	var (
		mu      sync.Mutex
		pending *config.Sidebar
		running bool
	)
	save := func(sb config.Sidebar) {
		mu.Lock()
		pending = &sb
		if running {
			mu.Unlock()
			return
		}
		running = true
		mu.Unlock()
		go func() {
			for {
				mu.Lock()
				next := pending
				pending = nil
				if next == nil {
					running = false
					mu.Unlock()
					return
				}
				mu.Unlock()
				_, _ = sys.SetSidebar(*next)
			}
		}()
	}
	return newPrefs(sys.Settings().Sidebar, save)
}

// FolderOpen reports whether a folder is open (folders start closed).
func (p *prefs) FolderOpen(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open[id]
}

// SetFolderOpen opens or closes a folder and remembers it.
func (p *prefs) SetFolderOpen(id string, open bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open[id] == open {
		return
	}
	setKey(p.open, id, open)
	p.saveLocked()
}

// Collapsed reports whether a section is folded (sections start open).
func (p *prefs) Collapsed(section string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.collapsed[section]
}

// SetCollapsed folds or unfolds a section and remembers it.
func (p *prefs) SetCollapsed(section string, folded bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.collapsed[section] == folded {
		return
	}
	setKey(p.collapsed, section, folded)
	p.saveLocked()
}

// snapshot is the state as settings hold it.
func (p *prefs) snapshot() config.Sidebar {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotLocked()
}

func (p *prefs) snapshotLocked() config.Sidebar {
	return config.Sidebar{OpenFolders: keys(p.open), FoldedSections: keys(p.collapsed)}
}

func (p *prefs) saveLocked() {
	if p.save != nil {
		p.save(p.snapshotLocked())
	}
}

func setKey(m map[string]bool, k string, on bool) {
	if on {
		m[k] = true
	} else {
		delete(m, k)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
