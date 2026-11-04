package player

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
)

// lyricsPrefsFile is where the lyrics view's choices live (v0.2.0 kept
// them in localStorage under "sona.lyrics"): a small JSON file in the
// config directory. Tests put it elsewhere.
func lyricsPrefsFile() string {
	paths, err := config.ResolvePaths(os.Getenv)
	if err != nil {
		return ""
	}
	return filepath.Join(paths.Config, "lyrics-view.json")
}

// lyricsPrefs are the Translation and Pronunciation choices: kept across
// songs and restarts, both off by default. Main thread only.
type lyricsPrefs struct {
	lyricsync.Prefs
	// file is the path read on first use and written on a change ("" keeps
	// them in memory).
	file   string
	loaded bool
	// failed is called with a write error (logged by the owner).
	failed func(error)
}

// get returns the choices, reading the file once.
func (l *lyricsPrefs) get() lyricsync.Prefs {
	if !l.loaded {
		l.loaded = true
		if l.file != "" {
			var p struct {
				Translation   bool `json:"translation"`
				Pronunciation bool `json:"pronunciation"`
			}
			if b, err := os.ReadFile(l.file); err == nil && json.Unmarshal(b, &p) == nil {
				l.Prefs = lyricsync.Prefs{Translation: p.Translation, Pronunciation: p.Pronunciation}
			}
		}
	}
	return l.Prefs
}

// set changes the choices and writes them.
func (l *lyricsPrefs) set(p lyricsync.Prefs) {
	l.get()
	l.Prefs = p
	if l.file == "" {
		return
	}
	b, _ := json.Marshal(struct {
		Translation   bool `json:"translation"`
		Pronunciation bool `json:"pronunciation"`
	}{p.Translation, p.Pronunciation})
	err := os.MkdirAll(filepath.Dir(l.file), 0o700)
	if err == nil {
		tmp := l.file + ".tmp"
		if err = os.WriteFile(tmp, b, 0o600); err == nil {
			err = os.Rename(tmp, l.file)
		}
	}
	if err != nil && l.failed != nil {
		l.failed(err)
	}
}
