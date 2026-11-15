package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
)

// AddonsService supervises add-ons (docs/addons.md): it lists the folders of
// the user's add-ons directory with their manifests, grants permissions when
// the user enables one, and runs the enabled ones after the first paint,
// restarting a crashed one with a growing wait and switching it off after
// too many crashes. It enforces invocation timeouts, keeps a log per
// add-on and stops them all, group and children, when Sona quits.
//
// An add-on is never in the UI's way: every call here returns without
// waiting for an add-on, except Invoke, which waits at most its deadline,
// so the UI calls it off its thread.
//
// Until the OS sandbox (#304) permissions are advisory: an add-on runs with
// the user's rights.
type AddonsService struct {
	e    *Env
	host cli.AddonHost
	root string
	logs string
	opts addonOptions

	mu      sync.Mutex
	runners map[string]*addonRunner // by add-on ID
	started bool                    // the network is open: enabled add-ons run
	closed  bool
}

// addonOptions are the supervisor's timings, replaced in tests.
type addonOptions struct {
	// backoff is the wait before restart n (the last repeats): the ceiling.
	backoff []time.Duration
	// maxCrashes in a row switch the add-on off; a run of at least stable
	// starts the count again.
	maxCrashes int
	stable     time.Duration
	// helloTimeout is how long a started add-on has to say hello.
	helloTimeout time.Duration
	// grace is how long an add-on has to leave when asked, twice (end of
	// input, then the terminate signal) before it is killed.
	grace time.Duration
	// environ is Sona's environment, os.Environ when nil.
	environ func() []string
}

func defaultAddonOptions() addonOptions {
	return addonOptions{
		backoff:      []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second},
		maxCrashes:   5,
		stable:       time.Minute,
		helloTimeout: 10 * time.Second,
		grace:        time.Second,
		environ:      os.Environ,
	}
}

// ErrAddonUnavailable is an add-on that is not running (or didn't answer a
// call in the time it had).
var ErrAddonUnavailable = errors.New("the add-on is not running")

func newAddonsService(e *Env) *AddonsService {
	s := &AddonsService{
		e: e, root: e.Paths.AddonsDir(), logs: e.Paths.AddonLogDir(),
		opts: defaultAddonOptions(), runners: map[string]*addonRunner{},
	}
	e.OnNetwork(s.start)
	e.OnClose(s.close)
	e.OnDiagnostics("Add-ons", s.bundle)
	return s
}

// attach gives the service the host its add-ons are served from, once the
// command server that holds it exists.
func (s *AddonsService) attach(h cli.AddonHost) {
	s.mu.Lock()
	s.host = h
	s.mu.Unlock()
}

// start runs the enabled add-ons, after the first paint.
func (s *AddonsService) start() {
	s.mu.Lock()
	if s.started || s.closed || !addons.Supported() {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	for _, en := range s.scan() {
		if en.Problem == nil && s.state(en.Manifest).Enabled {
			s.begin(en)
		}
	}
}

func (s *AddonsService) scan() []addons.Entry {
	es, err := addons.Scan(s.root, addons.ScanOptions{})
	if err != nil {
		s.e.Logger.Warn("addons: scan", "err", err)
	}
	return es
}

// state is the saved decision about an add-on.
func (s *AddonsService) state(m addons.Manifest) config.AddonSetting {
	return s.e.Settings.Get().Addons[m.ID]
}

// covers reports whether granted holds every permission of asked.
func covers(granted, asked []string) bool {
	for _, p := range asked {
		if !slices.Contains(granted, p) {
			return false
		}
	}
	return true
}

// List returns the add-ons of the user's folder, valid and refused, by ID.
func (s *AddonsService) List() []AddonInfo {
	set := s.e.Settings.Get().Addons
	s.mu.Lock()
	runners := make(map[string]*addonRunner, len(s.runners))
	for id, r := range s.runners {
		runners[id] = r
	}
	s.mu.Unlock()
	out := []AddonInfo{}
	for _, en := range s.scan() {
		info := AddonInfo{
			ID: en.Name(), Name: en.Manifest.Name, Version: en.Manifest.Version, Description: en.Manifest.Description,
			Permissions: stringsOrEmpty(en.Manifest.Permissions), Granted: []string{}, State: AddonOff,
		}
		if en.Problem != nil {
			info.State, info.Problem = AddonInvalid, en.Problem.Code
			info.Name = en.Name()
			out = append(out, info)
			continue
		}
		st := set[en.ID]
		info.Enabled, info.Granted = st.Enabled, stringsOrEmpty(st.Granted)
		switch r := runners[en.ID]; {
		case r != nil && !st.Enabled && r.crashed():
			info.State, info.Restarts = r.snapshot()
		case !st.Enabled:
			info.State = AddonOff
		case !covers(st.Granted, en.Manifest.Permissions):
			info.State = AddonNeedsApproval
		case r != nil:
			info.State, info.Restarts = r.snapshot()
		default:
			info.State = AddonStarting // enabled, the network not open yet
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b AddonInfo) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func stringsOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return slices.Clone(s)
}

func (s *AddonsService) find(id string) (addons.Entry, error) {
	for _, en := range s.scan() {
		if en.ID == id || (en.ID == "" && en.Dir == id) {
			if en.Problem != nil {
				return en, fmt.Errorf("%w: add-on %q is refused: %s", ErrInvalidArgument, id, en.Problem.Code)
			}
			return en, nil
		}
	}
	return addons.Entry{}, fmt.Errorf("%w: no add-on %q", ErrNotFoundAddon, id)
}

// ErrNotFoundAddon is an add-on that isn't in the folder; it classifies as
// not found.
var ErrNotFoundAddon = errors.New("no such add-on")

// Enable approves permissions and switches the add-on on. approve must be
// the permissions the manifest asks for now, as the user was shown them: a
// manifest that changed since is refused, so the user never approves what
// they didn't see. The add-on starts at once when the network is open (the
// first paint has happened), otherwise with the rest.
func (s *AddonsService) Enable(id string, approve []string) error {
	if !addons.Supported() {
		return errors.ErrUnsupported
	}
	en, err := s.find(id)
	if err != nil {
		return err
	}
	asked := slices.Clone(en.Manifest.Permissions)
	got := slices.Clone(approve)
	slices.Sort(asked)
	slices.Sort(got)
	if !slices.Equal(asked, got) {
		return fmt.Errorf("%w: the approved permissions are not the ones the manifest asks for", ErrInvalidArgument)
	}
	if _, err := s.e.updateSettings(func(st *config.Settings) {
		if st.Addons == nil {
			st.Addons = config.Addons{}
		}
		st.Addons[en.ID] = config.AddonSetting{Enabled: true, Granted: asked}
	}); err != nil {
		return err
	}
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if started {
		s.begin(en)
	}
	s.changed()
	return nil
}

// Disable switches the add-on off and stops it without waiting for it: it
// is given its shutdown event and a short grace, then killed, in the
// background. Its permissions stay granted.
func (s *AddonsService) Disable(id string) error {
	if _, err := s.find(id); err != nil && !errors.Is(err, ErrInvalidArgument) {
		return err
	}
	if _, err := s.e.updateSettings(func(st *config.Settings) {
		if a, ok := st.Addons[id]; ok {
			a.Enabled = false
			st.Addons[id] = a
		}
	}); err != nil {
		return err
	}
	s.mu.Lock()
	r := s.runners[id]
	delete(s.runners, id)
	s.mu.Unlock()
	if r != nil {
		r.stop()
	}
	s.changed()
	return nil
}

// Revoke takes one permission back. The add-on's very next call or event
// needing it is refused (the grant is read each time), and without it the add-on
// needs approval before it runs again.
func (s *AddonsService) Revoke(id, permission string) error {
	if !cli.KnownPermission(permission) {
		return fmt.Errorf("%w: unknown permission %q", ErrInvalidArgument, permission)
	}
	if _, ok := s.e.Settings.Get().Addons[id]; !ok {
		return fmt.Errorf("%w: no add-on %q", ErrNotFoundAddon, id)
	}
	if _, err := s.e.updateSettings(func(st *config.Settings) {
		a := st.Addons[id]
		a.Granted = slices.DeleteFunc(slices.Clone(a.Granted), func(p string) bool { return p == permission })
		st.Addons[id] = a
	}); err != nil {
		return err
	}
	s.changed()
	return nil
}

// Log returns the last lines of the add-on's log, at most 200.
func (s *AddonsService) Log(id string) ([]string, error) {
	if _, err := s.find(id); err != nil && !errors.Is(err, ErrInvalidArgument) {
		return nil, err
	}
	lines, err := addons.ReadLog(s.logs, id, 200, 64<<10)
	if lines == nil {
		lines = []string{}
	}
	return lines, err
}

// Invoke asks a running add-on to run one of its contributions (action is
// addon.<id>.<name>) and waits for its answer: at most the deadline ctx
// carries, or ten seconds, and never more than thirty. A hung add-on costs
// the caller that wait and nothing else.
func (s *AddonsService) Invoke(ctx context.Context, id, action string, target json.RawMessage) (AddonResult, error) {
	if !strings.HasPrefix(action, "addon."+id+".") || len(action) > 200 {
		return AddonResult{}, fmt.Errorf("%w: %q is not an action of add-on %q", ErrInvalidArgument, action, id)
	}
	s.mu.Lock()
	r := s.runners[id]
	s.mu.Unlock()
	sess := r.current()
	if sess == nil {
		return AddonResult{}, ErrAddonUnavailable
	}
	res, err := sess.Invoke(ctx, action, target)
	if err != nil {
		var ce *cli.Error
		if errors.As(err, &ce) && ce.Code == cli.AddonErrTimeout {
			return AddonResult{}, fmt.Errorf("add-on %s: %w", id, context.DeadlineExceeded)
		}
		return AddonResult{}, fmt.Errorf("add-on %s: %w", id, ErrAddonUnavailable)
	}
	return AddonResult{Toast: res.Toast, URL: res.URL}, nil
}

func (s *AddonsService) changed() {
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventAddonsChanged, s.List())
	}
}

// bundle is the diagnostics section: which add-ons exist and whether they
// run, never their logs (an add-on's text is not Sona's to put in a report).
func (s *AddonsService) bundle() any {
	type row struct {
		ID      string     `json:"id"`
		Version string     `json:"version"`
		Enabled bool       `json:"enabled"`
		State   AddonState `json:"state"`
	}
	rows := []row{}
	for _, a := range s.List() {
		rows = append(rows, row{a.ID, a.Version, a.Enabled, a.State})
	}
	return rows
}

// begin starts the supervision of one add-on (replacing a stopping one).
func (s *AddonsService) begin(en addons.Entry) {
	s.mu.Lock()
	if s.closed || s.host == nil {
		s.mu.Unlock()
		return
	}
	prev := s.runners[en.ID]
	ctx, cancel := context.WithCancel(context.Background())
	r := &addonRunner{s: s, dir: en.Dir, id: en.ID, cancel: cancel, done: make(chan struct{}), state: AddonStarting}
	s.runners[en.ID] = r
	s.mu.Unlock()
	s.e.Go("addon "+en.ID, func() { r.run(ctx, prev) })
}

// close stops every add-on at quit and waits (a bounded time: each is given
// its grace, then killed) until they are gone.
func (s *AddonsService) close() {
	s.mu.Lock()
	s.closed = true
	rs := make([]*addonRunner, 0, len(s.runners))
	for _, r := range s.runners {
		rs = append(rs, r)
	}
	s.runners = map[string]*addonRunner{}
	s.mu.Unlock()
	for _, r := range rs {
		r.stop()
	}
	for _, r := range rs {
		<-r.done
	}
}

// allowed reports whether the user's grant for id holds permission now: it
// is read from the settings at every call, never kept.
func (s *AddonsService) allowed(id, permission string) bool {
	return slices.Contains(s.e.Settings.Get().Addons[id].Granted, permission)
}

// addonRunner supervises one add-on: start, serve, restart.
type addonRunner struct {
	s      *AddonsService
	dir    string // the folder's name
	id     string
	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	state    AddonState
	restarts int
	session  *cli.Session
}

func (r *addonRunner) snapshot() (AddonState, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state, r.restarts
}

// crashed reports that the supervisor switched the add-on off.
func (r *addonRunner) crashed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == AddonCrashed
}

func (r *addonRunner) current() *cli.Session {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != AddonRunning {
		return nil
	}
	return r.session
}

func (r *addonRunner) set(state AddonState, sess *cli.Session) {
	r.mu.Lock()
	r.state, r.session = state, sess
	r.mu.Unlock()
	r.s.changed()
}

// stop asks the runner to end; its goroutine tells the add-on and reaps it.
func (r *addonRunner) stop() { r.cancel() }

// run is the add-on's supervisor loop. It stays on one OS thread for as long
// as the add-on runs: on Linux the kernel kills the add-on when the thread
// that started it ends, which is how it dies with Sona.
func (r *addonRunner) run(ctx context.Context, prev *addonRunner) {
	runtime.LockOSThread()
	defer close(r.done)
	if prev != nil {
		prev.stop()
		<-prev.done
	}
	s, o := r.s, r.s.opts
	log, err := addons.OpenLog(s.logs, r.id)
	if err != nil {
		s.e.Logger.Warn("addons: log", "addon", r.id, "err", err)
		r.set(AddonInvalid, nil)
		return
	}
	defer func() { _ = log.Close() }()
	crashes := 0
	for ctx.Err() == nil {
		en := addons.ScanOne(filepath.Join(s.root, r.dir), addons.ScanOptions{})
		if en.Problem != nil {
			log.Notef("refused: %s", en.Problem.Code)
			r.set(AddonInvalid, nil)
			return
		}
		if !covers(s.e.Settings.Get().Addons[r.id].Granted, en.Manifest.Permissions) {
			log.Notef("not started: the manifest asks for a permission that isn't granted")
			r.set(AddonNeedsApproval, nil)
			return
		}
		began := time.Now()
		r.runOnce(ctx, en, log)
		if ctx.Err() != nil {
			r.set(AddonOff, nil)
			return
		}
		if time.Since(began) >= o.stable {
			crashes = 0
		}
		crashes++
		r.mu.Lock()
		r.restarts++
		r.mu.Unlock()
		if crashes >= o.maxCrashes {
			log.Notef("stopped on its own %d times in a row: switched off", crashes)
			r.giveUp(en.Manifest.Name)
			return
		}
		wait := o.backoff[min(crashes-1, len(o.backoff)-1)]
		log.Notef("stopped on its own; starting again in %s", wait)
		r.set(AddonRestarting, nil)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
		}
	}
	r.set(AddonOff, nil)
}

// giveUp switches the add-on off after too many crashes and says so.
func (r *addonRunner) giveUp(name string) {
	s := r.s
	if _, err := s.e.updateSettings(func(st *config.Settings) {
		if a, ok := st.Addons[r.id]; ok {
			a.Enabled = false
			st.Addons[r.id] = a
		}
	}); err != nil {
		s.e.Logger.Warn("addons: switch off", "addon", r.id, "err", err)
	}
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventAddonsNotice, AddonNotice{ID: r.id, Name: name, Kind: "crashed"})
	}
	r.set(AddonCrashed, nil)
}

// runOnce starts the add-on and serves it until its channel ends, then
// makes sure it is gone: stopped, its group killed, reaped.
func (r *addonRunner) runOnce(ctx context.Context, en addons.Entry, log *addons.Log) {
	s, o := r.s, r.s.opts
	declared := en.Manifest.Permissions
	proc, err := addons.Start(addons.Spec{
		Exe: en.Exe, Dir: filepath.Join(s.root, r.dir), Env: addons.Environment(o.environ(), r.id), Stderr: log,
	})
	if err != nil {
		s.e.Logger.Warn("addons: start", "addon", r.id, "err", err)
		log.Notef("could not start")
		return
	}
	log.Notef("started (pid %d, version %s)", proc.Pid(), en.Manifest.Version)
	sess := cli.NewSession(cli.AddonConfig{
		ID: r.id, Host: s.host,
		// The grant is asked at every call; the manifest's list bounds it.
		Allowed: func(p string) bool { return slices.Contains(declared, p) && s.allowed(r.id, p) },
		Log:     slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
	r.set(AddonStarting, sess)
	served := make(chan struct{})
	s.e.Go("addon "+r.id+" watch", func() {
		defer close(served)
		timer := time.NewTimer(o.helloTimeout)
		defer timer.Stop()
		select {
		case <-sess.Helloed():
			r.mu.Lock()
			if r.session == sess {
				r.state = AddonRunning
			}
			r.mu.Unlock()
			r.s.changed()
		case <-timer.C:
			log.Notef("no hello in %s: killed", o.helloTimeout)
			proc.Kill()
		case <-sess.Done():
		}
		select {
		case <-ctx.Done():
			sess.Close() // the shutdown event, then the channel closes
		case <-sess.Done():
		}
	})
	if err := sess.Serve(context.WithoutCancel(ctx), proc.Conn()); err != nil {
		log.Notef("session ended: %v", err)
	}
	<-served
	proc.Stop(o.grace)
	err = proc.Err()
	log.Notef("exited: %v", err)
	r.set(AddonRestarting, nil)
}
