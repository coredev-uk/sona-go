package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`
	// ReduceEffects turns off backdrops and animation for solid surfaces.
	ReduceEffects bool `json:"reduceEffects"`
	// Volume is the playback volume in [0, 1].
	Volume float64 `json:"volume"`
	// AudioQuality is 256 or 64 (kbps).
	AudioQuality int `json:"audioQuality"`
	// CDMPath overrides the libwidevinecdm.so location; empty probes the
	// usual install paths.
	CDMPath string `json:"cdmPath"`
	// EngineIdleMinutes is how long the playback host stays alive with no
	// queue before it is shut down.
	EngineIdleMinutes int `json:"engineIdleMinutes"`
	// AutoPlay continues with similar music when the queue ends.
	AutoPlay bool `json:"autoPlay"`
	// Diagnostics holds rendering workarounds; all off by default.
	Diagnostics Diagnostics `json:"diagnostics"`
}

// Diagnostics are WebKitGTK rendering workarounds exposed as toggles. They
// take effect on the next start because they must be in the environment
// before the web view is created.
type Diagnostics struct {
	// DisableDMABufRenderer sets WEBKIT_DISABLE_DMABUF_RENDERER=1.
	DisableDMABufRenderer bool `json:"disableDmabufRenderer"`
	// DisableCompositingMode sets WEBKIT_DISABLE_COMPOSITING_MODE=1.
	DisableCompositingMode bool `json:"disableCompositingMode"`
}

// Default idle timeout for the playback host, in minutes.
const defaultEngineIdleMinutes = 10

// Defaults returns the settings used when no file exists.
func Defaults() Settings {
	return Settings{
		Theme:             ThemeSystem,
		Volume:            0.8,
		AudioQuality:      Quality256,
		EngineIdleMinutes: defaultEngineIdleMinutes,
		AutoPlay:          true,
	}
}

// Normalized returns s with out-of-range values replaced: unknown themes
// become "system", volume is clamped to [0, 1], unknown qualities become 256
// and a non-positive idle timeout becomes the default.
func (s Settings) Normalized() Settings {
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = ThemeSystem
	}
	s.Volume = min(max(s.Volume, 0), 1)
	if s.AudioQuality != Quality256 && s.AudioQuality != Quality64 {
		s.AudioQuality = Quality256
	}
	if s.EngineIdleMinutes <= 0 {
		s.EngineIdleMinutes = defaultEngineIdleMinutes
	}
	return s
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

// ApplyEnv exports the enabled WebKit workarounds into the process
// environment. It must run before the Wails application is created. A
// variable the user already set in the environment is left untouched, so an
// explicit WEBKIT_*=0 wins over the toggle.
func (d Diagnostics) ApplyEnv() error {
	set := func(key string, on bool) error {
		if !on {
			return nil
		}
		if _, ok := os.LookupEnv(key); ok {
			return nil
		}
		return os.Setenv(key, "1")
	}
	return errors.Join(
		set("WEBKIT_DISABLE_DMABUF_RENDERER", d.DisableDMABufRenderer),
		set("WEBKIT_DISABLE_COMPOSITING_MODE", d.DisableCompositingMode),
	)
}

// Store is a concurrency-safe handle on the settings file: it keeps the
// current settings in memory and persists every update.
type Store struct {
	path string
	mu   sync.Mutex
	cur  Settings
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
	return st.cur
}

// Update applies fn to a copy of the current settings, normalizes and saves
// the result, and returns it. On a save error the in-memory settings are
// unchanged.
func (st *Store) Update(fn func(*Settings)) (Settings, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	next := st.cur
	fn(&next)
	next = next.Normalized()
	if err := SaveSettings(st.path, next); err != nil {
		return st.cur, err
	}
	st.cur = next
	return next, nil
}
