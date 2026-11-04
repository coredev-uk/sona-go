package player

import (
	"sync"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// lyricsPrefs are the Translation and Pronunciation choices: kept across
// songs and restarts (v0.2.0 kept them in localStorage under "sona.lyrics";
// here they are config.Settings.LyricsView), both off by default. read and
// write are where they live; with neither (a test) they stay in memory. Main
// thread only.
type lyricsPrefs struct {
	read  func() lyricsync.Prefs
	write func(lyricsync.Prefs)
	mem   lyricsync.Prefs
}

// get returns the choices.
func (l *lyricsPrefs) get() lyricsync.Prefs {
	if l.read != nil {
		return l.read()
	}
	return l.mem
}

// set changes the choices and saves them.
func (l *lyricsPrefs) set(p lyricsync.Prefs) {
	if l.write != nil {
		l.write(p)
		return
	}
	l.mem = p
}

// settingsPrefs is the choices of the app: read from the settings cache
// (store.UseSettings: no file is read on the main thread), saved through
// SystemService.SetLyricsView off the main thread, one save at a time and the
// newest choices last. A choice shows at once: the cached settings are
// updated before the save returns.
func (p *player) settingsPrefs() (read func() lyricsync.Prefs, write func(lyricsync.Prefs)) {
	read = func() lyricsync.Prefs {
		v := store.UseSettings(p.env.Queries, p.env.Core).Data.LyricsView
		return lyricsync.Prefs{Translation: v.Translation, Pronunciation: v.Pronunciation}
	}
	var (
		mu      sync.Mutex
		pending *config.LyricsView
		running bool
	)
	save := func(v config.LyricsView) {
		mu.Lock()
		pending = &v
		if running {
			mu.Unlock()
			return
		}
		running = true
		mu.Unlock()
		sys := services.Service[services.SystemService](p.env.Core)
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
				if _, err := sys.SetLyricsView(*next); err != nil {
					p.env.Log.Warn("lyrics view: saving the choices", "err", services.CodeOf(err))
				}
			}
		}()
	}
	write = func(v lyricsync.Prefs) {
		view := config.LyricsView{Translation: v.Translation, Pronunciation: v.Pronunciation}
		if r := store.UseSettings(p.env.Queries, p.env.Core); r.HasData {
			s := r.Data
			s.LyricsView = view
			store.Set(p.env.Queries, store.KeySettings, s)
		}
		if p.env.Core != nil {
			save(view)
		}
	}
	return read, write
}
