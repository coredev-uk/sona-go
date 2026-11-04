package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Theme values for [Settings.Theme].
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// Audio quality values for [Settings.AudioQuality], in kbps. They select the
// 256 kbps (ctrp256) or 64 kbps (ctrp64) Widevine AAC flavor.
const (
	Quality256 = 256
	Quality64  = 64
)

// Settings is the user-editable configuration stored in settings.json. The
// JSON field names are the frontend's names, so the struct can be bound to
// Wails as-is. It never holds tokens.
type Settings struct {
	// Language is the interface language: "" (Automatic) or a BCP 47 tag
	// (language.go). Go's own text (tray, notifications, the command line,
	// dialog titles) and Apple's content language follow it too.
	Language string `json:"language"`
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`
	// ReduceEffects turns off backdrops and animation for solid surfaces.
	ReduceEffects bool `json:"reduceEffects"`
	// Volume is the playback volume in [0, 1].
	Volume float64 `json:"volume"`
	// AudioQuality is 256 or 64 (kbps).
	AudioQuality int `json:"audioQuality"`
	// EngineIdleMinutes is how long the playback host stays alive with no
	// queue before it is shut down.
	EngineIdleMinutes int `json:"engineIdleMinutes"`
	// AutoPlay continues with similar music when the queue ends.
	AutoPlay bool `json:"autoPlay"`
	// Scrobbling is the Last.fm and ListenBrainz connection state shown in
	// Settings; their session key and token live in the keyring.
	Scrobbling Scrobbling `json:"scrobbling"`
	// Desktop is the window, tray and startup behaviour (desktop.go).
	Desktop Desktop `json:"desktop"`
	// Window is the main window's remembered size (desktop.go).
	Window Window `json:"window"`
	// Shortcuts rebinds keyboard shortcuts (shortcuts.go).
	Shortcuts Shortcuts `json:"shortcuts"`
	// Updates is the release check (updates.go).
	Updates Updates `json:"updates"`
	// Integrations are the desktop and Discord integrations (integrations.go).
	Integrations Integrations `json:"integrations"`
	// Audio is the equaliser and output device (audio.go).
	Audio Audio `json:"audio"`
	// MotionArtwork plays Apple's motion covers (motion.go).
	MotionArtwork MotionArtwork `json:"motionArtwork"`
	// Queue is the play queue's behaviour (queue.go).
	Queue Queue `json:"queue"`
	// Storage is the local cache configuration (storage.go).
	Storage Storage `json:"storage"`
	// Crossfade overlaps tracks of different albums (crossfade.go).
	Crossfade Crossfade `json:"crossfade"`
	// Private is private listening (private.go).
	Private Private `json:"private"`
	// TitleBar is who draws the main window's title bar (titlebar.go).
	TitleBar TitleBar `json:"titleBar"`
	// Onboarding is the first-run setup's state (onboarding.go).
	Onboarding Onboarding `json:"onboarding"`
	// Lyrics is where lyrics come from (lyrics.go).
	Lyrics Lyrics `json:"lyrics"`
	// Concerts is where the Concerts page looks (concerts.go).
	Concerts Concerts `json:"concerts"`
	// LyricsView is the lyrics view's Translation and Pronunciation choices
	// (lyricsview.go).
	LyricsView LyricsView `json:"lyricsView"`
}

// Scrobbling holds the non-secret part of the scrobbler connections.
type Scrobbling struct {
	LastFM       ScrobbleAccount `json:"lastfm"`
	ListenBrainz ScrobbleAccount `json:"listenbrainz"`
}

// ScrobbleAccount is one scrobbler connection. Username is set when the
// account is connected and only displayed; Enabled pauses submitting
// without disconnecting.
type ScrobbleAccount struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
}

// normalized trims usernames. Enabled stays independent of Username: the
// connection itself is the stored secret, not this display name.
func (s Scrobbling) normalized() Scrobbling {
	s.LastFM.Username = strings.TrimSpace(s.LastFM.Username)
	s.ListenBrainz.Username = strings.TrimSpace(s.ListenBrainz.Username)
	return s
}

// Idle timeout for the playback host, in minutes: the default and the
// longest accepted (a day), which keeps the duration far from overflowing.
const (
	defaultEngineIdleMinutes = 10
	maxEngineIdleMinutes     = 24 * 60
)

// Defaults returns the settings used when no file exists.
func Defaults() Settings {
	return Settings{
		Theme:             ThemeSystem,
		Volume:            0.8,
		AudioQuality:      Quality256,
		EngineIdleMinutes: defaultEngineIdleMinutes,
		AutoPlay:          true,
		Desktop:           defaultDesktop(),
		Window:            defaultWindow(),
		Shortcuts:         Shortcuts{},
		Updates:           Updates{Check: true},
		Integrations:      defaultIntegrations(),
		Audio:             defaultAudio(),
		MotionArtwork:     MotionArtwork{Enabled: true},
		Queue:             Queue{ReplaceConfirm: ReplaceUndo},
		Storage:           Storage{ArtCacheMiB: DefaultArtCacheMiB},
		Crossfade:         Crossfade{Seconds: defaultCrossfadeSeconds},
		TitleBar:          TitleBar{Style: TitleBarAuto},
		Lyrics:            defaultLyrics(),
	}
}

// Normalized returns s with out-of-range values replaced: unknown themes
// become "system", volume is clamped to [0, 1], unknown qualities become 256,
// and a non-positive idle timeout becomes the default and a longer one than
// a day is cut to a day.
func (s Settings) Normalized() Settings {
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = ThemeSystem
	}
	s.Volume = clampVolume(s.Volume)
	if s.AudioQuality != Quality256 && s.AudioQuality != Quality64 {
		s.AudioQuality = Quality256
	}
	if s.EngineIdleMinutes <= 0 {
		s.EngineIdleMinutes = defaultEngineIdleMinutes
	}
	s.EngineIdleMinutes = min(s.EngineIdleMinutes, maxEngineIdleMinutes)
	s.Scrobbling = s.Scrobbling.normalized()
	s.Desktop = s.Desktop.normalized()
	s.Window = s.Window.normalized()
	s.Shortcuts = s.Shortcuts.normalized()
	s.Audio = s.Audio.normalized()
	s.Queue = s.Queue.normalized()
	s.Storage = s.Storage.normalized()
	s.Crossfade = s.Crossfade.normalized()
	s.TitleBar = s.TitleBar.normalized()
	s.Lyrics = s.Lyrics.normalized()
	s.Concerts = s.Concerts.normalized()
	s.Language = normalizedLanguage(s.Language)
	return s
}

// clampVolume clamps v to [0, 1]; NaN (which min and max pass through)
// becomes 0.
func clampVolume(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return min(max(v, 0), 1)
}

// LoadSettings reads the settings file at path. A missing file yields
// [Defaults] and no error. Keys absent from the file keep their defaults, and
// out-of-range values are normalized. A file that cannot be parsed yields the
// defaults together with the error, so callers can log it and keep running.
func LoadSettings(path string) (Settings, error) {
	s := Defaults()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("config: read settings: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Defaults(), fmt.Errorf("config: parse %s: %w", path, err)
	}
	return s.Normalized(), nil
}

// SaveSettings writes s (normalized) to path atomically: it writes a 0600
// temporary file in the same directory, syncs it and renames it over path, so
// a crash leaves either the old or the new file, never a partial one.
func SaveSettings(path string, s Settings) error {
	b, err := json.MarshalIndent(s.Normalized(), "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode settings: %w", err)
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp file: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if err = f.Chmod(perm); err == nil {
		if _, err = f.Write(data); err == nil {
			err = f.Sync()
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("config: write %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config: replace %s: %w", path, err)
	}
	// Persist the rename itself; best effort, not every filesystem supports it.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Store is a concurrency-safe handle on the settings file: it keeps the
// current settings in memory and persists every update.
type Store struct {
	path string
	mu   sync.Mutex
	cur  Settings

	obsMu sync.Mutex // serialises observer delivery, in Update order
	obs   []func(prev, next Settings)
}

// OpenStore loads the settings at path. Like [LoadSettings], an unparsable
// file yields a usable Store holding the defaults together with the error;
// the next Update overwrites the bad file.
func OpenStore(path string) (*Store, error) {
	s, err := LoadSettings(path)
	return &Store{path: path, cur: s}, err
}

// Get returns the current settings.
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cur.clone()
}

// clone returns s with its own copy of every map and slice (the shortcut
// map, the equaliser curve), so a caller's edits, or an Update function
// writing into them, never reach the Store's or a previous copy's values.
func (s Settings) clone() Settings {
	s.Shortcuts = maps.Clone(s.Shortcuts)
	s.Audio.Equalizer.Custom = slices.Clone(s.Audio.Equalizer.Custom)
	s.Lyrics.Providers = slices.Clone(s.Lyrics.Providers)
	return s
}

// OnChange registers fn to run after every successful Update with the
// settings before and after it. Observers run in registration order, one
// Update at a time, outside the store lock (they may call Get, not Update).
// A feature applies its own settings here instead of in UpdateSettings.
func (st *Store) OnChange(fn func(prev, next Settings)) {
	st.obsMu.Lock()
	st.obs = append(st.obs, fn)
	st.obsMu.Unlock()
}

// Update applies fn to a copy of the current settings, normalizes and saves
// the result, notifies the OnChange observers and returns it. On a save
// error the in-memory settings are unchanged and no observer runs.
func (st *Store) Update(fn func(*Settings)) (Settings, error) {
	st.obsMu.Lock()
	defer st.obsMu.Unlock()
	st.mu.Lock()
	prev := st.cur
	next := prev.clone()
	fn(&next)
	next = next.Normalized()
	if err := SaveSettings(st.path, next); err != nil {
		st.mu.Unlock()
		return prev, err
	}
	st.cur = next
	st.mu.Unlock()
	for _, o := range st.obs {
		o(prev, next)
	}
	return next, nil
}
