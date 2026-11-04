// Command devloop is the development loop of the native UI: it builds a
// package with the dev tag, runs it, watches the source tree, and on a
// change builds again while the running app keeps going. A build that
// succeeds quits the app (SIGTERM, so it saves where it was: the dev build
// restores its screen, scroll and window) and starts the new binary with the
// same arguments and environment; a build that fails prints its errors and
// leaves the app alone. Design tokens and catalogues are not built in the
// dev build's run: the app reloads tokens.json and the .po files itself.
//
//	devloop                         # ./cmd/sona
//	devloop -target ./cmd/uigallery # the component gallery
//	devloop -- --some-app-flag      # arguments after -- go to the app
//
// Keys (on a terminal): r builds and restarts at once, q quits the app and
// the loop.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/internal/devwatch"
)

type config struct {
	root   string // module root: the build runs here
	target string // package to build
	out    string // binary
	tags   string
	// ldflags are the linker's flags: -w (no DWARF) saves about a second of
	// every restart; "" keeps debug info for a debugger.
	ldflags string
	ready   string // what the app prints once its window is painted
	grace   time.Duration
	args    []string
}

func main() {
	if real := os.Getenv(extldEnv); real != "" {
		os.Exit(extld(real, os.Args[1:])) // `go build` linking through devloop
	}
	var cfg config
	flag.StringVar(&cfg.root, "root", ".", "module root (the build runs there, its internal/, cmd/ and build/ are watched)")
	flag.StringVar(&cfg.target, "target", "./cmd/sona", "package to build and run")
	flag.StringVar(&cfg.out, "out", "", "binary to build (default bin/<target>-dev under -root)")
	flag.StringVar(&cfg.tags, "tags", "dev,sqlite_fts5", "build tags")
	flag.StringVar(&cfg.ldflags, "ldflags", "-w", "linker flags (-w drops DWARF, which speeds up the link; use \"\" for a debugger)")
	flag.StringVar(&cfg.ready, "ready", "startup: first paint", "text the app prints when its window is painted (times the restart)")
	flag.DurationVar(&cfg.grace, "grace", 5*time.Second, "how long the app has to quit after SIGTERM before it is killed")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: devloop [flags] [-- app arguments]\n\nkeys: r rebuild and restart, q quit\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	cfg.args = flag.Args()

	root, err := filepath.Abs(cfg.root)
	if err != nil {
		fatal(err)
	}
	cfg.root = root
	if cfg.out == "" {
		cfg.out = filepath.Join("bin", filepath.Base(cfg.target)+"-dev")
	}
	if !filepath.IsAbs(cfg.out) {
		cfg.out = filepath.Join(cfg.root, cfg.out)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.out), 0o755); err != nil {
		fatal(err)
	}
	os.Exit(newLoop(cfg).run())
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "devloop:", err)
	os.Exit(1)
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, "devloop: "+format+"\n", a...) }

// loop is the state of the dev loop; run is its only goroutine that
// changes it.
type loop struct {
	cfg config

	// deps are the directories of the packages the target is built from,
	// known once a build has finished: a change anywhere else builds
	// nothing. Nil until then (every change builds).
	deps map[string]bool

	app      *process // nil when none runs
	building bool
	again    bool      // a change arrived during the build: its result is stale
	since    time.Time // when the change the build answers was saved
}

func newLoop(cfg config) *loop { return &loop{cfg: cfg} }

// built is a finished build.
type built struct {
	out  []byte
	err  error
	took time.Duration
}

func (l *loop) run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	changes := make(chan []string, 4)
	go devwatch.Watch(ctx, l.watchOptions(), func(paths []string) { changes <- paths })

	keys := make(chan byte, 4)
	restore := rawKeys()
	defer restore()
	go readKeys(keys)

	results := make(chan built, 1)
	deps := make(chan map[string]bool, 1)
	exited := make(chan *process, 1)

	startBuild := func(since time.Time) {
		l.building, l.again, l.since = true, false, since
		go func() {
			b := l.build()
			results <- b
			if b.err == nil {
				// After the result, off the restart's way.
				if d := l.packages(); d != nil {
					deps <- d
				}
			}
		}()
	}
	startApp := func(t0 time.Time, buildTook, stopTook time.Duration) {
		p, err := startProcess(l.cfg, func(painted time.Time) {
			logf("window painted: %.2fs after the change (build %.2fs, stop %.2fs)",
				painted.Sub(t0).Seconds(), buildTook.Seconds(), stopTook.Seconds())
		})
		if err != nil {
			logf("start: %v", err)
			return
		}
		l.app = p
		go func() { <-p.done; exited <- p }()
	}

	logf("building %s (tags %s); r rebuilds, q quits", l.cfg.target, l.cfg.tags)
	startBuild(time.Now())
	first := true

	for {
		select {
		case sig := <-sigs:
			logf("%v: quitting", sig)
			l.stop()
			return 0

		case k := <-keys:
			switch k {
			case 'q', 'Q':
				logf("quitting")
				l.stop()
				return 0
			case 'r', 'R':
				if l.building {
					l.again = true
					continue
				}
				logf("rebuilding")
				startBuild(time.Now())
			}

		case paths := <-changes:
			if paths = l.relevant(paths); len(paths) == 0 {
				continue
			}
			if !rebuilds(paths) {
				logf("%s changed: the running app reloads it", rel(l.cfg.root, paths[0]))
				continue
			}
			logf("%s changed", summary(l.cfg.root, paths))
			saved := savedAt(paths)
			if l.building {
				l.again = true
				l.since = saved
				continue
			}
			startBuild(saved)

		case d := <-deps:
			l.deps = d

		case b := <-results:
			l.building = false
			if b.err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", bytes.TrimRight(b.out, "\n"))
				if l.app != nil {
					logf("build failed after %.2fs: %v; the app keeps running the last good build", b.took.Seconds(), b.err)
				} else {
					logf("build failed after %.2fs: %v; waiting for a change (r builds again)", b.took.Seconds(), b.err)
				}
				if l.again {
					startBuild(time.Now())
				}
				continue
			}
			if l.again {
				logf("build ok, but the sources changed meanwhile: building again")
				startBuild(l.since)
				continue
			}
			t0 := l.since
			stopTook := l.stop()
			if err := os.Rename(l.cfg.out+".new", l.cfg.out); err != nil {
				logf("install: %v", err)
				continue
			}
			if first {
				logf("build ok in %.2fs", b.took.Seconds())
				first = false
			} else {
				logf("build ok in %.2fs: restarting", b.took.Seconds())
			}
			startApp(t0, b.took, stopTook)

		case p := <-exited:
			if p != l.app {
				continue // one we stopped ourselves
			}
			l.app = nil
			if p.err == nil {
				logf("the app quit")
				return 0
			}
			logf("the app exited: %v; waiting for a change (r rebuilds and restarts)", p.err)
		}
	}
}

// stop quits the running app and says how long it took.
func (l *loop) stop() time.Duration {
	if l.app == nil {
		return 0
	}
	p := l.app
	l.app = nil
	t := time.Now()
	p.terminate(l.cfg.grace)
	return time.Since(t)
}

// build runs go build into out.new, linking through devloop itself (see
// extld.go).
func (l *loop) build() built {
	t := time.Now()
	ldflags := l.cfg.ldflags
	self, err := os.Executable()
	real := os.Getenv("CXX")
	if real == "" {
		real = "g++"
	}
	if err == nil {
		ldflags = strings.TrimSpace(ldflags + " -extld=" + self)
	}
	cmd := exec.Command("go", "build", "-tags", l.cfg.tags, "-ldflags", ldflags, "-o", l.cfg.out+".new", l.cfg.target)
	cmd.Dir = l.cfg.root
	cmd.Env = append(os.Environ(), extldEnv+"="+real)
	out, err := cmd.CombinedOutput()
	return built{out: out, err: err, took: time.Since(t)}
}

// packages are the directories under the module root of the packages the
// target imports, itself included; nil when go list fails.
func (l *loop) packages() map[string]bool {
	cmd := exec.Command("go", "list", "-deps", "-tags", l.cfg.tags, "-f", "{{if not .Standard}}{{.Dir}}{{end}}", l.cfg.target)
	cmd.Dir = l.cfg.root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	deps := map[string]bool{}
	for _, dir := range strings.Fields(string(out)) {
		if dir == l.cfg.root || strings.HasPrefix(dir, l.cfg.root+string(filepath.Separator)) {
			deps[dir] = true
		}
	}
	return deps
}

// relevant keeps the changes that matter to the target: files in the
// directory of one of its packages (or below it: embedded files), and the
// module files. Every change counts until the first build has said which
// packages there are.
func (l *loop) relevant(paths []string) []string {
	if l.deps == nil {
		return paths
	}
	var out []string
	for _, p := range paths {
		if dir := filepath.Dir(p); filepath.Dir(p) == l.cfg.root || l.within(dir) {
			out = append(out, p)
		}
	}
	return out
}

// within reports whether dir is a package's directory or below one.
func (l *loop) within(dir string) bool {
	for ; dir != "" && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if l.deps[dir] {
			return true
		}
	}
	return false
}

// watchOptions are what triggers a build or a reload: the Go sources, the
// files they embed, and the design tokens and catalogues the app reloads.
func (l *loop) watchOptions() devwatch.Options {
	r := l.cfg.root
	return devwatch.Options{
		Roots: []string{filepath.Join(r, "internal"), filepath.Join(r, "cmd"), filepath.Join(r, "build"), filepath.Join(r, "go.mod"), filepath.Join(r, "go.sum")},
		SkipDir: func(name string) bool {
			switch name {
			case ".git", ".devenv", ".plans", "bin", "node_modules", "testdata", "vendor":
				return true
			}
			return false
		},
		Match: func(path string) bool {
			base := filepath.Base(path)
			if strings.HasSuffix(base, "_test.go") {
				return false
			}
			switch filepath.Ext(base) {
			case ".go", ".mod", ".sum", ".svg", ".sql", ".json", ".po", ".ttf", ".otf", ".png", ".c", ".cc", ".cpp", ".h", ".m", ".mm":
				return true
			}
			return false
		},
	}
}

// live is a file the dev build reloads by itself.
func live(path string) bool {
	base := filepath.Base(path)
	return base == "tokens.json" || filepath.Ext(base) == ".po"
}

// rebuilds reports whether any of paths needs a build.
func rebuilds(paths []string) bool {
	for _, p := range paths {
		if !live(p) {
			return true
		}
	}
	return false
}

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}

func summary(root string, paths []string) string {
	s := rel(root, paths[0])
	if len(paths) > 1 {
		s += fmt.Sprintf(" and %d more", len(paths)-1)
	}
	return s
}

// savedAt is when the newest of paths was written: the start of the
// restart's clock. A removed file counts as now.
func savedAt(paths []string) time.Time {
	t := time.Time{}
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
	}
	if t.IsZero() || time.Since(t) > time.Minute {
		return time.Now()
	}
	return t
}
