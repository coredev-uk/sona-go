package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
)

// `sona addons dev <folder>` (docs/addons.md, "Developing an add-on") runs an
// add-on from a folder without installing it. The rules, each with a test:
//
//   - the folder is checked as an installed one is (the user's, not writable by
//     others, not a link), and the add-on's ID may not be an installed one;
//   - the person confirms in Sona's own window, which names the folder and the
//     permissions; nothing answers for them, and a command from an add-on's
//     channel never reaches this code (cli.RequestPermission refuses it);
//   - the permissions are the manifest's, held in memory for the run: nothing
//     is written to the settings;
//   - the add-on gets the defaults of its settings rows, none of the user's
//     values or secrets;
//   - the program is opened once, checked as the open file and run from that
//     file (addons.OpenVerified), so a path swapped meanwhile changes nothing;
//   - the run ends when its client stops asking for the log, or at quit.

// devAsk is the one question waiting for the person.
type devAsk struct {
	token string
	ch    chan bool
}

// devRun is one run of `addons dev`.
type devRun struct {
	token  string
	id     string
	dir    string // the folder, absolute
	runner *addonRunner
	buf    *devBuffer

	mu   sync.Mutex
	last time.Time // when the client last asked for the log
}

func (d *devRun) touch(now time.Time) {
	d.mu.Lock()
	d.last = now
	d.mu.Unlock()
}

func (d *devRun) idle(now time.Time) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	return now.Sub(d.last)
}

// maxDevLines bounds the lines a run keeps for its client.
const maxDevLines = 2000

// devBuffer is a run's log lines, numbered from 0, for the client to read
// since the line it has.
type devBuffer struct {
	mu    sync.Mutex
	lines []string
	base  int64 // the number of lines dropped from the front
	ended bool
	exit  string
}

func (b *devBuffer) add(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if drop := len(b.lines) - maxDevLines; drop > 0 {
		b.lines = slices.Clone(b.lines[drop:])
		b.base += int64(drop)
	}
}

func (b *devBuffer) end(exit string) {
	b.mu.Lock()
	b.ended, b.exit = true, exit
	b.mu.Unlock()
}

// since returns at most cli.DevLogBatch lines from line next on, and where
// the next read starts. Lines that were dropped are skipped.
func (b *devBuffer) since(next int64) cli.AddonDevLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	next = max(next, b.base)
	from := min(int(next-b.base), len(b.lines))
	lines := slices.Clone(b.lines[from:min(from+cli.DevLogBatch, len(b.lines))])
	if lines == nil {
		lines = []string{}
	}
	end := next + int64(len(lines))
	return cli.AddonDevLog{
		Lines: lines, Next: end, Ended: b.ended && end >= b.base+int64(len(b.lines)), Exit: b.exit,
	}
}

func newToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// AnswerDev is the person's answer to the question EventAddonsDevAsk carried.
// A token that isn't the one waiting is ignored.
func (s *AddonsService) AnswerDev(token string, run bool) {
	s.devMu.Lock()
	a := s.ask
	s.devMu.Unlock()
	if a != nil && a.token == token {
		select {
		case a.ch <- run:
		default:
		}
	}
}

// askDev puts the question to the person and waits for the answer: yes only
// when they say so, in time. One question at a time: another `addons dev`
// meanwhile is refused, so nothing can bury the window in questions.
func (s *AddonsService) askDev(ctx context.Context, q AddonDevAsk) (bool, error) {
	a := &devAsk{token: q.Token, ch: make(chan bool, 1)}
	s.devMu.Lock()
	if s.ask != nil {
		s.devMu.Unlock()
		return false, &cli.Error{Code: "busy", Message: "another add-on is waiting for your confirmation in Sona's window"}
	}
	s.ask = a
	s.devMu.Unlock()
	defer func() {
		s.devMu.Lock()
		s.ask = nil
		s.devMu.Unlock()
		if s.e.Emit != nil {
			s.e.Emit.Emit(EventAddonsDevAskEnd, q.Token)
		}
	}()
	if s.e.Emit == nil {
		return false, nil // no window to ask in
	}
	s.e.Emit.Emit(EventAddonsDevAsk, q)
	if s.e.Windows != nil {
		s.e.Windows.ShowMain() // the question is in the main window: bring it up
	}
	timer := time.NewTimer(s.opts.devAsk)
	defer timer.Stop()
	select {
	case ok := <-a.ch:
		return ok, nil
	case <-timer.C:
		return false, nil
	case <-ctx.Done():
		return false, nil
	}
}

// installedID reports whether an add-on of the user's folder (valid or
// refused) has this ID or a folder of this name, or a runner holds the ID.
func (s *AddonsService) installedID(id string) bool {
	for _, en := range s.scan() {
		if strings.EqualFold(en.Name(), id) || strings.EqualFold(en.ID, id) || strings.EqualFold(en.Dir, id) {
			return true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, running := s.runners[id]
	return running
}

// DevStart is `addons dev <folder>`: it checks the folder, asks the person in
// Sona's window and, once they confirm, runs the add-on. It waits for the
// person, so it is not called on the UI's thread.
func (s *AddonsService) DevStart(ctx context.Context, dir string) (cli.AddonDev, error) {
	if !addons.Supported() {
		return cli.AddonDev{}, fmt.Errorf("add-ons are not supported on this system: %w", errors.ErrUnsupported)
	}
	s.mu.Lock()
	ready := s.host != nil && !s.closed
	s.mu.Unlock()
	if !ready {
		return cli.AddonDev{}, ErrAddonUnavailable
	}
	if !filepath.IsAbs(dir) {
		return cli.AddonDev{}, fmt.Errorf("%w: the folder must be an absolute path", ErrInvalidArgument)
	}
	dir = filepath.Clean(dir)
	en, err := s.devEntry(dir)
	if err != nil {
		return cli.AddonDev{}, err
	}
	m := en.Manifest
	// A native program is run from the file that was checked, which only
	// Linux can do (/proc/self/fd). A module needs no such thing: its bytes
	// are read once and those bytes run, on every system wazero runs on.
	if !m.Wasm() && !s.opts.verified {
		return cli.AddonDev{}, fmt.Errorf("add-ons dev runs a native add-on on Linux only; a wasm add-on runs anywhere: %w", errors.ErrUnsupported)
	}
	ok, err := s.askDev(ctx, AddonDevAsk{
		Token: newToken(), ID: m.ID, Name: m.Name, Version: m.Version, Dir: dir, Permissions: slices.Clone(m.Permissions),
	})
	if err != nil {
		return cli.AddonDev{}, err
	}
	if !ok {
		return cli.AddonDev{}, &cli.Error{Code: cli.CodeDenied}
	}
	// What the person confirmed is what runs: the folder is read again, and
	// one whose add-on, program or permissions changed meanwhile is refused.
	again, err := s.devEntry(dir)
	if err != nil {
		return cli.AddonDev{}, err
	}
	if again.Exe != en.Exe || again.ID != en.ID || !slices.Equal(again.Manifest.Permissions, m.Permissions) || again.Manifest.Version != m.Version {
		return cli.AddonDev{}, fmt.Errorf("%w: the folder changed while you were asked", ErrInvalidArgument)
	}
	prog, err := openDevProgram(again, dir)
	if err != nil {
		return cli.AddonDev{}, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	run, err := s.beginDev(again, dir, prog)
	if err != nil {
		prog.close()
		return cli.AddonDev{}, err
	}
	return cli.AddonDev{ID: m.ID, Name: m.Name, Version: m.Version, Permissions: slices.Clone(m.Permissions), Token: run.token}, nil
}

// devEntry checks the folder as an installed add-on is checked, and that its
// ID is no installed add-on's nor a run's.
func (s *AddonsService) devEntry(dir string) (addons.Entry, error) {
	en := addons.ScanOne(dir, addons.ScanOptions{})
	if en.Problem != nil {
		return en, fmt.Errorf("%w: %s (%s)", ErrInvalidArgument, en.Problem.Code, en.Problem.Error())
	}
	// A folder inside the add-ons folder is an installed add-on, whatever it is called.
	if rel, err := filepath.Rel(s.root, dir); err == nil && !strings.HasPrefix(rel, "..") {
		return en, fmt.Errorf("%w: %s is in the add-ons folder: it is installed", ErrInvalidArgument, en.ID)
	}
	if s.installedID(en.ID) {
		return en, fmt.Errorf("%w: %s is an installed add-on's ID", ErrInvalidArgument, en.ID)
	}
	return en, nil
}

// devProgram is what a development run executes, opened once and checked:
// a native add-on's open file, which runs through /proc/self/fd, or a
// module's bytes. What was checked is what runs, whatever happens to the
// path meanwhile.
type devProgram struct {
	file   *os.File
	module []byte
}

func openDevProgram(en addons.Entry, dir string) (devProgram, error) {
	if en.Manifest.Wasm() {
		b, err := addons.ReadModule(en.Exe)
		return devProgram{module: b}, err
	}
	f, err := addons.OpenVerified(dir, en.Exe)
	return devProgram{file: f}, err
}

func (p devProgram) close() {
	if p.file != nil {
		_ = p.file.Close()
	}
}

// beginDev starts the run: a runner of its own (not the installed kind: it
// never restarts), registered under the add-on's ID so its actions work, and a
// lease the client keeps alive by asking for the log.
func (s *AddonsService) beginDev(en addons.Entry, dir string, prog devProgram) (*devRun, error) {
	s.mu.Lock()
	if s.closed || s.host == nil {
		s.mu.Unlock()
		return nil, ErrAddonUnavailable
	}
	if _, taken := s.runners[en.ID]; taken {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: %s is already running", ErrInvalidArgument, en.ID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &devRun{token: newToken(), id: en.ID, dir: dir, buf: &devBuffer{}, last: time.Now()}
	r := &addonRunner{s: s, dir: en.Dir, id: en.ID, cancel: cancel, done: make(chan struct{}), state: AddonStarting, dev: run}
	run.runner = r
	s.runners[en.ID] = r
	s.mu.Unlock()
	s.devMu.Lock()
	s.devs[run.token] = run
	s.devMu.Unlock()
	s.e.Go("addon dev "+en.ID, func() { r.runDev(ctx, en, prog, run) })
	s.e.Go("addon dev lease "+en.ID, func() { s.lease(ctx, run) })
	return run, nil
}

// lease ends a run whose client stopped asking for the log.
func (s *AddonsService) lease(ctx context.Context, run *devRun) {
	tick := time.NewTicker(max(s.opts.devLease/10, 5*time.Millisecond))
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-run.runner.done:
			return
		case now := <-tick.C:
			if run.idle(now) > s.opts.devLease {
				run.runner.stop()
				return
			}
		}
	}
}

// runDev is the run's supervisor: one start, no restart.
func (r *addonRunner) runDev(ctx context.Context, en addons.Entry, prog devProgram, run *devRun) {
	runtime.LockOSThread()
	s, o := r.s, r.s.opts
	defer close(r.done)
	defer func() {
		prog.close()
		s.mu.Lock()
		if s.runners[r.id] == r {
			delete(s.runners, r.id)
		}
		s.mu.Unlock()
		r.set(AddonOff, nil)
		exit := "exited cleanly"
		switch {
		case ctx.Err() != nil:
			exit = "stopped by Sona"
		case r.exit != nil:
			exit = r.exit.Error()
		}
		run.buf.end(exit)
	}()
	// A log file of its own: no installed add-on's ID has a dot.
	log, err := addons.OpenLog(s.logs, en.ID+".dev")
	if err != nil {
		s.e.Logger.Warn("addons: dev log", "addon", r.id, "err", err)
		return
	}
	defer func() { _ = log.Close() }()
	log.Tap(run.buf.add)
	log.Notef("development run from %s: its permissions are held for this run only", run.dir)
	declared := en.Manifest.Permissions
	var start startFunc
	if en.Manifest.Wasm() {
		// A development run of a module gets no data folder: it gets none of your data.
		start = func() (addons.Running, error) { return s.startWasm(en.ID, prog.module, "", log) }
	} else {
		data := s.e.Paths.AddonDataDir(r.id)
		if err := addons.EnsureData(data); err != nil {
			s.e.Logger.Warn("addons: dev data folder", "addon", r.id, "err", err)
			log.Notef("could not make its data folder")
			return
		}
		// The same sandbox as an installed add-on's, with the permissions the
		// person confirmed for this run.
		spec := addons.Spec{
			ID: r.id, Exe: en.Exe, ExeFile: prog.file, Dir: run.dir, Data: data, Network: slices.Contains(declared, cli.PermNetwork),
			Env: addons.Environment(o.environ(), r.id, data), Stderr: log,
		}
		start = func() (addons.Running, error) { return r.startProc(spec, false, log) }
	}
	_ = r.serve(ctx, en, log, start, newAddonOwn(s, en.Manifest, true), func(p string) bool { return slices.Contains(declared, p) })
}

// DevLog is a run's lines since line next (the client's poll). Asking keeps
// the run alive. A run that ended and whose last lines were read is forgotten.
func (s *AddonsService) DevLog(token string, next int64) (cli.AddonDevLog, error) {
	run := s.devRun(token)
	if run == nil {
		return cli.AddonDevLog{}, ErrNotFoundAddon
	}
	run.touch(time.Now())
	l := run.buf.since(next)
	if l.Ended {
		s.forget(run)
	}
	return l, nil
}

// DevStop ends a run at its client's request and waits for it to be gone.
func (s *AddonsService) DevStop(token string) error {
	run := s.devRun(token)
	if run == nil {
		return ErrNotFoundAddon
	}
	run.runner.stop()
	select {
	case <-run.runner.done:
	case <-time.After(4*s.opts.grace + 2*time.Second):
	}
	s.forget(run)
	return nil
}

func (s *AddonsService) devRun(token string) *devRun {
	s.devMu.Lock()
	defer s.devMu.Unlock()
	return s.devs[token]
}

func (s *AddonsService) forget(run *devRun) {
	s.devMu.Lock()
	delete(s.devs, run.token)
	s.devMu.Unlock()
}

// devCall answers the `addons` request the command server received.
func (s *AddonsService) devCall(ctx context.Context, req cli.Request) (any, error) {
	r, err := cli.ParseDevRequest(req)
	if err != nil {
		return nil, err
	}
	switch r.Verb {
	case cli.DevStart:
		return s.DevStart(ctx, r.Dir)
	case cli.DevLog:
		return s.DevLog(r.Token, r.Next)
	default:
		return struct{}{}, s.DevStop(r.Token)
	}
}
