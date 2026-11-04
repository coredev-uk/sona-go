//go:build dev

package devstate

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/devinstance"
	"github.com/coredev-uk/sona-go/internal/devwatch"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// scrollWait is how long a restored offset waits for its page to be tall
// enough (the data it shows arrives after the first frame).
const scrollWait = 10 * time.Second

type pendingScroll struct {
	y        float32
	deadline time.Time
	// rested counts the frames the page has stood at its end, not tall
	// enough for y: after settleFrames it is as tall as it gets.
	rested int
}

// settleFrames is how many frames at the end of a page too short for the
// saved offset end the wait for it to grow.
const settleFrames = 30

// reg is what pages and the shell share; the main thread, but guarded as
// Save may run from a quit listener.
var reg struct {
	mu      sync.Mutex
	loc     func() string
	offsets map[string]float32       // by location, as the last frames showed
	pending map[string]pendingScroll // restored, not yet reached
	flags   map[string]*bool
	waiting map[string]bool // restored flags whose page has not registered yet
}

// Instance is SONA_DEV_INSTANCE, which names a development run so that two
// worktrees run side by side: it suffixes the state directory's name (and
// the D-Bus names, internal/devinstance).
func Instance() string { return devinstance.Name() }

// programName is the program whose state this is ("" for the app).
var programName string

// SetName gives the program its own state file, dev-restore-<name>.json, so
// that two programs built with the dev tag (the app and the gallery) do not
// overwrite each other's. Call it before Load.
func SetName(name string) {
	reg.mu.Lock()
	programName = unsafeProgram.ReplaceAllString(name, "-")
	reg.mu.Unlock()
}

var unsafeProgram = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// file is the state file: dev-restore.json in Sona's state directory
// (sona-<instance> with SONA_DEV_INSTANCE).
func file() (string, error) {
	p, err := config.ResolvePaths(os.Getenv)
	if err != nil {
		return "", err
	}
	dir := p.State
	if n := Instance(); n != "" {
		dir = filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-"+n)
	}
	reg.mu.Lock()
	name := "dev-restore.json"
	if programName != "" {
		name = "dev-restore-" + programName + ".json"
	}
	reg.mu.Unlock()
	return filepath.Join(dir, name), nil
}

// Load reads the state the last run left. SONA_DEV_FRESH=1 ignores it: a
// run from the start page.
func Load() (State, bool) {
	if os.Getenv("SONA_DEV_FRESH") == "1" {
		return State{}, false
	}
	path, err := file()
	if err != nil {
		return State{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, false
	}
	var st State
	if json.Unmarshal(data, &st) != nil {
		return State{}, false
	}
	return st, true
}

// Restore arms the scroll offsets and flags of st: an offset is applied by
// Scroll when its page is built, and kept until the page is tall enough.
func Restore(st State) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.pending == nil {
		reg.pending = map[string]pendingScroll{}
		reg.waiting = map[string]bool{}
	}
	if reg.offsets == nil {
		reg.offsets = map[string]float32{}
	}
	for loc, y := range st.Scroll {
		if y > 0 {
			reg.pending[loc] = pendingScroll{y: y, deadline: time.Now().Add(scrollWait)}
			reg.offsets[loc] = y
		}
	}
	for name, v := range st.Flags {
		if p := reg.flags[name]; p != nil {
			*p = v
		} else {
			reg.waiting[name] = v
		}
	}
}

// Save writes the state for the next run: route is the router's location.
func Save(route string, w *Window) {
	path, err := file()
	if err != nil {
		return
	}
	reg.mu.Lock()
	st := State{Route: route, Window: w, Scroll: map[string]float32{}, Flags: map[string]bool{}}
	for loc, y := range reg.offsets {
		if y > 0 {
			st.Scroll[loc] = y
		}
	}
	for name, p := range reg.flags {
		st.Flags[name] = *p
	}
	reg.mu.Unlock()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil || os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// SetLocation tells Scroll how to name the page: the router's location.
func SetLocation(fn func() string) {
	reg.mu.Lock()
	reg.loc = fn
	reg.mu.Unlock()
}

// Scroll keeps the scroll offset of the page being built across restarts.
// Call it in the page's view, before the container that tracks s.
func Scroll(c *ui.Context, s *ui.ScrollState) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.loc == nil {
		return
	}
	if reg.offsets == nil {
		reg.offsets = map[string]float32{}
	}
	loc := reg.loc()
	if p, ok := reg.pending[loc]; ok {
		if s.MaxY > 0 && s.Y >= s.MaxY-1 && s.Y < p.y-1 {
			p.rested++
		} else {
			p.rested = 0
		}
		reg.pending[loc] = p
		if s.Y >= p.y-1 || p.rested >= settleFrames || time.Now().After(p.deadline) {
			delete(reg.pending, loc)
		} else {
			// The container keeps Y within its content: ask again until the
			// page has grown to it.
			s.Y = p.y
			c.Invalidate()
			return
		}
	}
	reg.offsets[loc] = s.Y
}

// Flag keeps *p (an open overlay, an aside) across restarts under name.
// Call it when the module initialises.
func Flag(name string, p *bool) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.flags == nil {
		reg.flags = map[string]*bool{}
	}
	reg.flags[name] = p
	if v, ok := reg.waiting[name]; ok {
		*p = v
		delete(reg.waiting, name)
	}
}

// sourceRoot is the source tree this binary was built from: SONA_SOURCE
// (devloop sets it), else where the compiler found this file (not with
// -trimpath). "" when neither holds Sona's tokens.
func sourceRoot() string {
	root := os.Getenv("SONA_SOURCE")
	if root == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			return ""
		}
		root = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file)))) // internal/ui/devstate/dev.go
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(theme.SourceFile))); err != nil {
		return ""
	}
	return root
}

// Live reads tokens.json and the .po catalogues from the source tree now
// and again whenever they change, so that an edit shows on the next frame
// without a build. post runs a func on the main thread and redraws (the
// window's Update).
func Live(post func(func()), log *slog.Logger) {
	root := sourceRoot()
	if root == "" {
		log.Warn("dev: no source tree for live tokens (set SONA_SOURCE to the repository)")
		return
	}
	tokens := filepath.Join(root, filepath.FromSlash(theme.SourceFile))
	locales := filepath.Join(root, "internal", "ui", "i18n", "locales")
	if err := theme.Reload(tokens); err != nil {
		log.Error("dev: tokens.json", "err", err)
	}
	go devwatch.Watch(context.Background(), devwatch.Options{
		Roots: []string{tokens, locales},
		Match: func(path string) bool {
			return filepath.Base(path) == "tokens.json" || filepath.Ext(path) == ".po"
		},
	}, func(paths []string) {
		post(func() {
			for _, p := range paths {
				reload(p, log)
			}
		})
	})
}

// reload applies one changed file.
func reload(path string, log *slog.Logger) {
	if filepath.Base(path) == "tokens.json" {
		if err := theme.Reload(path); err != nil {
			log.Error("dev: tokens.json not reloaded", "err", err)
			return
		}
		log.Info("dev: tokens.json reloaded")
		return
	}
	tag := strings.TrimSuffix(filepath.Base(path), ".po")
	if tag == i18n.SourceLocale {
		return // the call sites' own text is the source
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // removed
	}
	if err := i18n.LoadCommunity(tag, data); err != nil {
		log.Error("dev: catalogue not reloaded", "tag", tag, "err", err)
		return
	}
	log.Info("dev: catalogue reloaded", "tag", tag)
}
