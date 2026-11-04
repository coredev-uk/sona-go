package playlist

import (
	"slices"
	"sync"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// prefs is the sidebar's remembered state: which folders are open and which
// sections are folded (v0.2.0 kept them in localStorage under
// sona.playlist-folders.open and sona.sidebar.collapsed; here they are
// config.Settings.Sidebar). The views read the settings live (what another
// surface or an import changed shows), and each change is written back.
type prefs struct {
	// read is the current state; main thread.
	read func() config.Sidebar
	// write shows a change at once and saves it.
	write func(config.Sidebar)
}

// newPrefs keeps the state in memory, starting from what the settings hold;
// save (nil: none) is told the new state after every change.
func newPrefs(from config.Sidebar, save func(config.Sidebar)) *prefs {
	cur := from
	return &prefs{
		read: func() config.Sidebar { return cur },
		write: func(sb config.Sidebar) {
			cur = sb
			if save != nil {
				save(sb)
			}
		},
	}
}

// settingsPrefs is the prefs of the app: read live from the settings cache
// (store.UseSettings; Go's own copy until the cache holds one), saved
// through SystemService.SetSidebar off the main thread, one save at a time and
// the newest state last (a failed save keeps the state for this session).
// A change shows at once: it is written into the cache before the save, and
// "settings:changed" confirms it.
func settingsPrefs(cl *store.Client, core *services.Core) *prefs {
	sys := services.Service[services.SystemService](core)
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
	return &prefs{
		read: func() config.Sidebar {
			if r := store.UseSettings(cl, core); r.HasData {
				return r.Data.Sidebar
			}
			return sys.Settings().Sidebar
		},
		write: func(sb config.Sidebar) {
			cur, ok := store.Peek[config.Settings](cl, store.KeySettings)
			if !ok {
				cur = sys.Settings()
			}
			cur.Sidebar = sb
			store.Set(cl, store.KeySettings, cur)
			save(sb)
		},
	}
}

// FolderOpen reports whether a folder is open (folders start closed).
func (p *prefs) FolderOpen(id string) bool { return slices.Contains(p.read().OpenFolders, id) }

// SetFolderOpen opens or closes a folder and remembers it.
func (p *prefs) SetFolderOpen(id string, open bool) {
	sb := p.read()
	if on := slices.Contains(sb.OpenFolders, id); on == open {
		return
	}
	sb.OpenFolders = with(sb.OpenFolders, id, open)
	p.write(sb)
}

// Collapsed reports whether a section is folded (sections start open).
func (p *prefs) Collapsed(section string) bool {
	return slices.Contains(p.read().FoldedSections, section)
}

// SetCollapsed folds or unfolds a section and remembers it.
func (p *prefs) SetCollapsed(section string, folded bool) {
	sb := p.read()
	if on := slices.Contains(sb.FoldedSections, section); on == folded {
		return
	}
	sb.FoldedSections = with(sb.FoldedSections, section, folded)
	p.write(sb)
}

// snapshot is the state as settings hold it.
func (p *prefs) snapshot() config.Sidebar { return p.read() }

// with is list with id in it (on) or out of it, as a new sorted list.
func with(list []string, id string, on bool) []string {
	out := make([]string, 0, len(list)+1)
	for _, v := range list {
		if v != id {
			out = append(out, v)
		}
	}
	if on {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
