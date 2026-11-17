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
	"github.com/coredev-uk/sona-go/internal/addons/sandbox"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/discord"
	"github.com/coredev-uk/sona-go/internal/scrobble"
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
// Every add-on runs in the OS sandbox (internal/addons/sandbox): it reaches its
// own folder, a private data folder and, with the `network` permission, the
// network, and nothing else of the user's system. Where the system can't set
// the sandbox up the add-on does not run (AddonNoSandbox), except Sona's own
// bundled add-ons, which then run as plain child processes with Sona's rights
// (AddonInfo.Unsandboxed), as the same code did in Sona's process before #302.
type AddonsService struct {
	e    *Env
	host cli.AddonHost
	root string
	logs string
	opts addonOptions

	// noted holds the folders already reported as claiming one of Sona's own
	// IDs, so the log says it once.
	noted sync.Map

	// recMu serialises reconcile.
	recMu sync.Mutex

	mu      sync.Mutex
	runners map[string]*addonRunner // by add-on ID
	started bool                    // the network is open: enabled add-ons run
	// leases counts what holds one of Sona's own add-ons up besides its
	// integration being in use (Lease), by add-on ID.
	leases map[string]int
	// ticked is closed, and replaced, at every change of an add-on's state:
	// a caller waiting for one to come up (Request) wakes on it.
	ticked chan struct{}
	closed bool
	// discordConnected is told what the `discord` add-on says of its
	// connection to the Discord app (nil: nobody is).
	discordConnected func(bool)

	// devMu guards `addons dev`: the one question waiting for the person and
	// the runs, by token (addons_dev.go).
	devMu sync.Mutex
	ask   *devAsk
	devs  map[string]*devRun

	// wasmMu guards wasm, the host of the WebAssembly add-ons. It is made
	// when the first one starts, so Sona pays nothing for a runtime nobody
	// uses, and closed at quit once every add-on is gone.
	wasmMu sync.Mutex
	wasm   *addons.WasmHost
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
	// wasm are the limits of a WebAssembly add-on's memory and CPU.
	wasm addons.WasmLimits
	// verified is whether this system can run a native add-on from the file
	// that was checked (addons.CanRunVerified), which `addons dev` needs.
	verified bool
	// environ is Sona's environment, os.Environ when nil.
	environ func() []string
	// exe is Sona's own executable, with links resolved: the program of its
	// bundled add-ons ("" when it can't be told).
	exe string
	// start runs an add-on; addons.Start, replaced in tests (to fail as a
	// system without the sandbox does).
	start func(addons.Spec) (*addons.Proc, error)
	// devAsk is how long `addons dev` waits for the person to confirm in
	// Sona's window; devLease how long a run lives without its client asking
	// for the log (a killed terminal).
	devAsk, devLease time.Duration
}

func defaultAddonOptions() addonOptions {
	return addonOptions{
		backoff:      []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second},
		maxCrashes:   5,
		stable:       time.Minute,
		helloTimeout: 10 * time.Second,
		grace:        time.Second,
		environ:      os.Environ,
		wasm:         addons.DefaultWasmLimits(),
		verified:     addons.CanRunVerified,
		exe:          selfExecutable(),
		start:        addons.Start,
		devAsk:       60 * time.Second,
		devLease:     15 * time.Second,
	}
}

// selfExecutable is the path of the running Sona with every link resolved,
// "" when it can't be told. A binary replaced by an update while it runs
// reads "… (deleted)" on Linux: the path is the installed one's.
func selfExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe = strings.TrimSuffix(exe, " (deleted)")
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return exe
}

// ErrAddonUnavailable is an add-on that is not running (or didn't answer a
// call in the time it had).
var ErrAddonUnavailable = errors.New("the add-on is not running")

func newAddonsService(e *Env) *AddonsService {
	s := &AddonsService{
		e: e, root: e.Paths.AddonsDir(), logs: e.Paths.AddonLogDir(),
		opts: defaultAddonOptions(), runners: map[string]*addonRunner{}, devs: map[string]*devRun{}, leases: map[string]int{},
	}
	e.Settings.OnChange(func(prev, next config.Settings) { s.settingsChanged() })
	e.OnNetwork(s.start)
	e.OnClose(s.close)
	e.OnDiagnostics("Add-ons", s.bundle)
	return s
}

// settingsChanged tells every running add-on its rows' new values.
func (s *AddonsService) settingsChanged() {
	s.mu.Lock()
	rs := make([]*addonRunner, 0, len(s.runners))
	for _, r := range s.runners {
		rs = append(rs, r)
	}
	s.mu.Unlock()
	for _, r := range rs {
		if own := r.ownData(); own != nil {
			own.publish()
		}
	}
	s.reconcile()
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
	// Held until every add-on is begun: reconcile, woken by a settings change
	// meanwhile, must not begin one of them a second time.
	s.recMu.Lock()
	defer s.recMu.Unlock()
	s.mu.Lock()
	if s.started || s.closed || !addons.Supported() {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	for _, en := range s.scan() {
		if en.Problem == nil && s.wanted(en) {
			s.begin(en)
		}
	}
}

// wanted reports whether the supervisor should run en now: it is on, and one
// of Sona's own has something to do (bundledWanted).
func (s *AddonsService) wanted(en addons.Entry) bool {
	if !s.setting(en).Enabled {
		return false
	}
	return !en.Bundled || s.inUse(en.ID, s.e.Settings.Get())
}

// bundledWanted reports whether Sona's own add-on id has anything to do
// under st. Each is a process of Sona's size, so none runs for an
// integration nobody uses: Last.fm and ListenBrainz while an account is
// connected (also switched off, as listens already queued are still
// delivered), Discord while its status is on.
func bundledWanted(id string, st config.Settings) bool {
	switch id {
	case "lastfm":
		return st.Scrobbling.LastFM.Enabled || st.Scrobbling.LastFM.Username != ""
	case "listenbrainz":
		return st.Scrobbling.ListenBrainz.Enabled || st.Scrobbling.ListenBrainz.Username != ""
	case "discord":
		return st.Integrations.DiscordPresence && discord.ClientID() != ""
	}
	return true
}

// reconcile starts Sona's own add-ons that have something to do and stops
// those that no longer do, after a change of the settings. An add-on the
// supervisor switched off for crashing stays as it is.
func (s *AddonsService) reconcile() {
	s.recMu.Lock()
	defer s.recMu.Unlock()
	s.mu.Lock()
	started, closed := s.started, s.closed
	s.mu.Unlock()
	if !started || closed {
		return
	}
	moved := false
	for _, en := range s.bundled() {
		if en.Problem != nil {
			continue
		}
		s.mu.Lock()
		r := s.runners[en.ID]
		s.mu.Unlock()
		switch want := s.wanted(en); {
		case want && r == nil:
			s.begin(en)
			moved = true
		case !want && r != nil && !r.crashed():
			s.mu.Lock()
			delete(s.runners, en.ID)
			s.mu.Unlock()
			r.stop()
			moved = true
		}
	}
	if moved {
		s.changed()
	}
}

// scan lists the add-ons: Sona's own, registered in code, then the folders
// of the user's add-ons directory. A folder that claims one of Sona's IDs
// is left out (and noted once in the log): the real one is already listed
// and nothing from the folder ever runs under that name.
func (s *AddonsService) scan() []addons.Entry {
	es, err := addons.Scan(s.root, addons.ScanOptions{})
	if err != nil {
		s.e.Logger.Warn("addons: scan", "err", err)
	}
	out := s.bundled()
	for _, en := range es {
		if addons.IsBundled(strings.ToLower(en.ID)) || addons.IsBundled(strings.ToLower(en.Dir)) {
			if _, noted := s.noted.LoadOrStore(en.Dir, true); !noted {
				s.e.Logger.Warn("addons: ignoring a folder that claims one of Sona's own add-on IDs", "dir", en.Dir)
			}
			continue
		}
		out = append(out, en)
	}
	return out
}

// entryOf reads the add-on a runner supervises again: one of Sona's own from
// the registry, an installed one from its folder.
func (s *AddonsService) entryOf(id, dir string) addons.Entry {
	if addons.IsBundled(id) {
		for _, en := range s.bundled() {
			if en.ID == id {
				return en
			}
		}
	}
	return addons.ScanOne(filepath.Join(s.root, dir), addons.ScanOptions{})
}

// setting is the saved decision about en. Sona's own add-ons are on unless
// the user turned one off, and always hold the permissions their manifest
// asks for: they are the signed release itself, there is nothing to approve.
func (s *AddonsService) setting(en addons.Entry) config.AddonSetting {
	st, saved := s.e.Settings.Get().Addons[en.ID]
	if !en.Bundled {
		return st
	}
	if !saved {
		st.Enabled = true
	}
	st.Granted = slices.Clone(en.Manifest.Permissions)
	return st
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
	s.mu.Lock()
	runners := make(map[string]*addonRunner, len(s.runners))
	for id, r := range s.runners {
		runners[id] = r
	}
	s.mu.Unlock()
	out := []AddonInfo{}
	cur := s.e.Settings.Get()
	for _, en := range s.scan() {
		info := AddonInfo{
			ID: en.Name(), Name: en.Manifest.Name, Version: en.Manifest.Version, Description: en.Manifest.Description,
			Runtime:     en.Manifest.RuntimeName(),
			Permissions: stringsOrEmpty(en.Manifest.Permissions), Granted: []string{}, State: AddonOff,
		}
		if en.Problem != nil {
			info.State, info.Problem = AddonInvalid, en.Problem.Code
			info.Name = en.Name()
			out = append(out, info)
			continue
		}
		st := s.setting(en)
		info.Bundled = en.Bundled
		info.Enabled, info.Granted = st.Enabled, stringsOrEmpty(st.Granted)
		switch r := runners[en.ID]; {
		case r != nil && !st.Enabled && r.crashed():
			info.State, info.Restarts = r.snapshot()
		case !st.Enabled:
			info.State = AddonOff
		case en.Bundled && r == nil && !s.inUse(en.ID, cur):
			info.State = AddonIdle
		case !covers(st.Granted, en.Manifest.Permissions):
			info.State = AddonNeedsApproval
		case r != nil:
			info.State, info.Restarts = r.snapshot()
			info.Sandbox = r.sandboxReason()
			info.Unsandboxed = r.unsandboxedReason()
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
	switch {
	case !started:
	case en.Bundled:
		// Turned on again after crashing: forget the crashed runner, then run
		// it if it has something to do.
		s.mu.Lock()
		if r := s.runners[en.ID]; r != nil && r.crashed() {
			delete(s.runners, en.ID)
		}
		s.mu.Unlock()
		s.reconcile()
	default:
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
	if _, err := s.e.updateSettings(func(st *config.Settings) { setEnabled(st, id, false) }); err != nil {
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
// needs approval before it runs again. Revoking `network` also stops the add-on:
// its network is the sandbox's, fixed when the process starts.
func (s *AddonsService) Revoke(id, permission string) error {
	if !cli.KnownPermission(permission) {
		return fmt.Errorf("%w: unknown permission %q", ErrInvalidArgument, permission)
	}
	if addons.IsBundled(id) {
		return fmt.Errorf("%w: %q is Sona's own add-on; its permissions are part of it", ErrInvalidArgument, id)
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
	if permission == cli.PermNetwork {
		// The network is the sandbox's, given when the process starts: only
		// ending the add-on takes it back. It runs again once approved.
		s.mu.Lock()
		r := s.runners[id]
		delete(s.runners, id)
		s.mu.Unlock()
		if r != nil {
			r.stop()
		}
	}
	s.changed()
	return nil
}

// OpenFolder creates the add-ons folder, mode 0700 like the config
// directory, if it is not there yet and opens it in the file manager: how a
// person installs an add-on. Sona never reads or runs what is put there
// until the add-on is enabled.
func (s *AddonsService) OpenFolder() error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("add-ons: create %s: %w", s.root, err)
	}
	if s.e.OpenURL == nil {
		return fmt.Errorf("add-ons: open folder: %w", errors.ErrUnsupported)
	}
	return s.e.OpenURL("file://" + s.root)
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

// Invoke asks a running add-on to run one of the actions or commands its
// manifest declares (action is addon.<id>.<name>) and waits for its answer: at
// most the deadline ctx carries, or ten seconds, and never more than thirty. A
// hung add-on costs the caller that wait and nothing else. target is the item
// an action was chosen on, nil for a command; an action Sona never read in
// the manifest is refused, so no add-on is sent a call it didn't offer.
func (s *AddonsService) Invoke(ctx context.Context, id, action string, target *AddonTarget) (AddonResult, error) {
	local, ok := addons.LocalID(id, action)
	if !ok || len(action) > 200 {
		return AddonResult{}, fmt.Errorf("%w: %q is not an action of add-on %q", ErrInvalidArgument, action, id)
	}
	s.mu.Lock()
	r := s.runners[id]
	s.mu.Unlock()
	sess, m := r.current()
	if sess == nil {
		return AddonResult{}, ErrAddonUnavailable
	}
	var raw json.RawMessage
	switch {
	case slices.ContainsFunc(m.Contributions.Actions, func(a addons.Action) bool { return a.ID == local }):
		if target == nil || !slices.Contains(addons.ActionKinds, target.Kind) || target.ID == "" || len(target.ID) > 200 {
			return AddonResult{}, fmt.Errorf("%w: action %q needs an item", ErrInvalidArgument, action)
		}
		t := AddonTarget{Kind: target.Kind, ID: target.ID, Name: addons.Sanitize(target.Name, 400)}
		raw, _ = json.Marshal(t)
	case slices.ContainsFunc(m.Contributions.Commands, func(a addons.Command) bool { return a.ID == local }):
		if target != nil {
			return AddonResult{}, fmt.Errorf("%w: command %q has no item", ErrInvalidArgument, action)
		}
	default:
		return AddonResult{}, fmt.Errorf("%w: add-on %q declares no %q", ErrInvalidArgument, id, local)
	}
	res, err := sess.Invoke(ctx, action, raw)
	if err != nil {
		var ce *cli.Error
		if errors.As(err, &ce) && ce.Code == cli.AddonErrTimeout {
			return AddonResult{}, fmt.Errorf("add-on %s: %w", id, context.DeadlineExceeded)
		}
		return AddonResult{}, fmt.Errorf("add-on %s: %w", id, ErrAddonUnavailable)
	}
	// What the add-on says is text for a person: clean of control and
	// bidirectional characters and cut, whatever it sent.
	return AddonResult{Toast: addons.Sanitize(res.Toast, cli.MaxToast), URL: res.URL}, nil
}

func (s *AddonsService) changed() {
	s.tick()
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
	s.wasmMu.Lock()
	if s.wasm != nil {
		s.wasm.Close()
		s.wasm = nil
	}
	s.wasmMu.Unlock()
}

// allowed reports whether the user's grant for id holds permission now: it
// is read from the settings at every call, never kept.
func (s *AddonsService) allowed(id, permission string) bool {
	if addons.IsBundled(id) {
		return true // Sona's own: the manifest's list is all there is to grant
	}
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
	manifest addons.Manifest
	own      *addonOwn
	// dev is the `addons dev` run this one is, nil for an installed add-on.
	dev *devRun
	// exit is how the last run of the add-on ended.
	exit error
	// sandbox is why the sandbox couldn't be set up (addons.Sandbox*), when
	// the state is AddonNoSandbox.
	sandbox string
	// unsandboxed is why a bundled add-on runs as a plain process
	// (addons.Sandbox*), "" when it runs confined or not at all.
	unsandboxed string
	// sockets is what the `discord` add-on was shown of Discord's IPC when it
	// started (socketsID); it changes when Discord restarts.
	sockets string
}

func (r *addonRunner) snapshot() (AddonState, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state, r.restarts
}

// sandboxReason is why the sandbox couldn't be set up, "" when it could.
func (r *addonRunner) sandboxReason() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != AddonNoSandbox {
		return ""
	}
	return r.sandbox
}

// unsandboxedReason is why the add-on runs without the sandbox, "" when it
// runs inside it or isn't running.
func (r *addonRunner) unsandboxedReason() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.state {
	case AddonStarting, AddonRunning, AddonRestarting:
		return r.unsandboxed
	}
	return ""
}

// crashed reports that the supervisor switched the add-on off.
func (r *addonRunner) crashed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == AddonCrashed
}

// current is the running add-on's session and manifest; nil when it isn't up.
func (r *addonRunner) current() (*cli.Session, addons.Manifest) {
	if r == nil {
		return nil, addons.Manifest{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != AddonRunning {
		return nil, addons.Manifest{}
	}
	return r.session, r.manifest
}

func (r *addonRunner) ownData() *addonOwn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.own
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
		en := s.entryOf(r.id, r.dir)
		if en.Problem != nil {
			log.Notef("refused: %s", en.Problem.Code)
			r.set(AddonInvalid, nil)
			return
		}
		if !covers(s.setting(en).Granted, en.Manifest.Permissions) {
			log.Notef("not started: the manifest asks for a permission that isn't granted")
			r.set(AddonNeedsApproval, nil)
			return
		}
		began := time.Now()
		if err := r.runOnce(ctx, en, log); err != nil {
			// The sandbox can't be set up: the add-on is not started and not
			// restarted. Turning it off and on, or the next start, tries again.
			reason, _ := addons.SandboxReason(err)
			r.mu.Lock()
			r.sandbox = reason
			r.mu.Unlock()
			r.set(AddonNoSandbox, nil)
			return
		}
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
	// A runner that was replaced or removed meanwhile (turned off, begun
	// again) has no say over the add-on's switch.
	s.mu.Lock()
	current := s.runners[r.id] == r
	s.mu.Unlock()
	if !current {
		return
	}
	// Crashed first: saving the switch runs reconcile, which leaves a
	// crashed add-on as it is.
	r.set(AddonCrashed, nil)
	if _, err := s.e.updateSettings(func(st *config.Settings) { setEnabled(st, r.id, false) }); err != nil {
		s.e.Logger.Warn("addons: switch off", "addon", r.id, "err", err)
	}
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventAddonsNotice, AddonNotice{ID: r.id, Name: name, Kind: "crashed"})
	}
}

// workDir is the add-on's working directory: its folder, and for one of
// Sona's own add-ons the folder Sona's executable is installed in.
func (r *addonRunner) workDir(en addons.Entry) string {
	if en.Bundled {
		return filepath.Dir(en.Exe)
	}
	return filepath.Join(r.s.root, r.dir)
}

// setEnabled saves whether add-on id is on. An add-on with no saved entry
// has none to change, unless it is one of Sona's own, which is on by
// default and so needs the entry that says it isn't.
func setEnabled(st *config.Settings, id string, on bool) {
	a, ok := st.Addons[id]
	if !ok && !addons.IsBundled(id) {
		return
	}
	if st.Addons == nil {
		st.Addons = config.Addons{}
	}
	a.Enabled = on
	st.Addons[id] = a
}

// startFunc starts one run of an add-on, whatever its runtime.
type startFunc func() (addons.Running, error)

// runOnce starts the installed add-on and serves it until its channel ends,
// then makes sure it is gone: stopped, its group killed, reaped. It returns
// the sandbox's failure to start, which is not a crash: restarting won't help.
func (r *addonRunner) runOnce(ctx context.Context, en addons.Entry, log *addons.Log) error {
	s, o := r.s, r.s.opts
	declared := en.Manifest.Permissions
	data := s.e.Paths.AddonDataDir(r.id)
	if err := addons.EnsureData(data); err != nil {
		s.e.Logger.Warn("addons: data folder", "addon", r.id, "err", err)
		log.Notef("could not make its data folder")
		return nil
	}
	var start startFunc
	if en.Manifest.Wasm() {
		start = func() (addons.Running, error) {
			// The bytes read now are the bytes compiled and run.
			module, err := addons.ReadModule(en.Exe)
			if err != nil {
				return nil, err
			}
			return s.startWasm(en.ID, module, data, log)
		}
	} else {
		spec := addons.Spec{
			ID: r.id, Exe: en.Exe, Args: en.Args, Dir: r.workDir(en), Data: data, Network: slices.Contains(declared, cli.PermNetwork),
			Env: addons.Environment(o.environ(), r.id, data), Stderr: log,
		}
		if en.Bundled {
			// Sona's own program: it reads the install it runs from (and the
			// libraries beside it), as any program of its own does.
			spec.Read = sandbox.ProgramReads(en.Exe, o.environ())
			if en.ID == "discord" {
				r.shareDiscord(&spec)
			}
		}
		start = func() (addons.Running, error) { return r.startProc(spec, en.Bundled, log) }
	}
	// The grant is asked at every call; the manifest's list bounds it.
	allowed := func(p string) bool { return slices.Contains(declared, p) && s.allowed(r.id, p) }
	if err := r.serve(ctx, en, log, start, newAddonOwn(s, en.Manifest, false), allowed); err != nil {
		return err
	}
	r.set(AddonRestarting, nil)
	return nil
}

// startProc starts a native add-on as a process in the OS sandbox, and says in
// its log what the sandbox lacks. Where the sandbox can't be set up and the
// add-on is one of Sona's own (bundled: code-registered, never judged by its
// ID alone), it is started again as a plain child process with Sona's rights,
// which is the trust the same code had in Sona's process before #302; the log
// and AddonInfo.Unsandboxed say so. Any other add-on fails closed.
func (r *addonRunner) startProc(spec addons.Spec, bundled bool, log *addons.Log) (addons.Running, error) {
	r.mu.Lock()
	r.unsandboxed = ""
	r.mu.Unlock()
	p, err := r.s.opts.start(spec)
	if reason, ok := addons.SandboxReason(err); ok && bundled {
		r.s.e.Logger.Warn("addons: sandbox can't start, running a bundled add-on without it", "addon", r.id, "reason", reason, "err", err)
		log.Notef("running without the sandbox: %s (%s)", reason, addons.SandboxText(reason))
		spec.Unsandboxed = true
		if p, err = r.s.opts.start(spec); err == nil {
			r.mu.Lock()
			r.unsandboxed = reason
			r.mu.Unlock()
		}
	}
	if err != nil {
		return nil, err
	}
	for _, note := range p.Notes() {
		log.Notef("sandbox: %s", note)
	}
	return p, nil
}

// startWasm runs a module on the shared WebAssembly host. The module runs
// inside Sona on a goroutine of its own, never the UI's. Its one folder is its
// data folder, mounted at /data (none for a development run: data is "").
func (s *AddonsService) startWasm(id string, module []byte, data string, log *addons.Log) (addons.Running, error) {
	h := s.wasmHost(log)
	p, err := h.Start(addons.WasmSpec{
		Module: module, Name: id + ".wasm", DataDir: data,
		Env:    addons.WasmEnvironment(s.opts.environ(), id, data != ""),
		Stderr: log,
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// wasmHost is the host of the WebAssembly add-ons, made on first use.
func (s *AddonsService) wasmHost(log *addons.Log) *addons.WasmHost {
	s.wasmMu.Lock()
	defer s.wasmMu.Unlock()
	if s.wasm == nil {
		s.wasm = addons.NewWasmHost(s.e.Paths.WasmCacheDir(), s.opts.wasm)
		if note := s.wasm.CacheNote; note != "" {
			s.e.Logger.Warn("addons: compiled wasm modules are kept in memory only", "why", note)
			log.Notef("compiled code is not cached on disk: %s", note)
		}
	}
	return s.wasm
}

// serve starts the add-on with start and serves its session until the
// channel ends, then stops it, kills its group and reaps it. own is its
// settings and secrets, allowed its grant.
func (r *addonRunner) serve(ctx context.Context, en addons.Entry, log *addons.Log, start startFunc, own *addonOwn, allowed func(string) bool) error {
	s, o := r.s, r.s.opts
	proc, err := start()
	if err != nil {
		s.e.Logger.Warn("addons: start", "addon", r.id, "err", err)
		if reason, ok := addons.SandboxReason(err); ok {
			log.Notef("not started: the sandbox can't be set up: %s", addons.SandboxText(reason))
			return err
		}
		log.Notef("could not start")
		return nil
	}
	log.Notef("started (%s, version %s)", proc.Where(), en.Manifest.Version)
	r.mu.Lock()
	r.manifest, r.own = en.Manifest, own
	r.mu.Unlock()
	sess := cli.NewSession(cli.AddonConfig{
		ID: r.id, Host: s.host, Own: own, Allowed: allowed, Credentials: bundledCredentials(en), Bundled: en.Bundled,
		OnConnection: r.connectionReport(en),
		Log:          slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
	r.set(AddonStarting, sess)
	served := make(chan struct{})
	s.e.Go("addon "+r.id+" watch", func() {
		defer close(served)
		// The time to say hello runs from when the add-on is running: a
		// module is compiled first, and the first compile is seconds that
		// are not the module's.
		select {
		case <-proc.Ready():
		case <-sess.Done():
		case <-ctx.Done():
		}
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
		case <-ctx.Done():
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
	r.exit = err
	if en.Bundled && en.ID == "discord" && s.discordConnected != nil {
		s.discordConnected(false)
	}
	return nil
}

// bundled lists Sona's own add-ons as they run on this system: without
// Discord's where its presence runs in Sona's process
// (Platform.DiscordInProcess).
func (s *AddonsService) bundled() []addons.Entry {
	all := addons.BundledEntries(s.opts.exe)
	if !s.e.Platform.DiscordInProcess {
		return all
	}
	return slices.DeleteFunc(all, func(en addons.Entry) bool { return en.ID == "discord" })
}

// shareDiscord is the one sandbox exception (docs/security-review.md,
// "Add-ons"): the `discord` add-on is shown the endpoints of Discord's IPC
// that Sona finds now, by F4's checks (discord.Sockets), and told which they
// are. It is not given the network, and no other socket.
func (r *addonRunner) shareDiscord(spec *addons.Spec) {
	socks := discord.Sockets()
	for _, k := range socks {
		spec.Sockets = append(spec.Sockets, k.Path)
	}
	spec.Env = append(spec.Env, discord.EnvSockets+"="+discord.SocketsEnv(socks))
	r.mu.Lock()
	r.sockets = socketsID(socks)
	r.mu.Unlock()
}

// socketsID is what changes when Discord's endpoints do: which exist, and
// when each was made.
func socketsID(socks []discord.Socket) string {
	var b strings.Builder
	for _, k := range socks {
		b.WriteString(k.Path + "\x00" + k.ID + "\x00")
	}
	return b.String()
}

// connectionReport is what a session does with the connection a bundled
// add-on reports: nil for every add-on but `discord`, which has the method.
//
// A bind mount shows the add-on the socket that existed when it started: a
// Discord that started, or restarted, since has made another, which the
// add-on can't reach. So when it says it isn't connected and the endpoints
// Sona finds now are not the ones it was shown, Sona starts it again.
func (r *addonRunner) connectionReport(en addons.Entry) func(bool) {
	if !en.Bundled || en.ID != "discord" {
		return nil
	}
	s := r.s
	return func(connected bool) {
		if s.discordConnected != nil {
			s.discordConnected(connected)
		}
		if connected {
			return
		}
		r.mu.Lock()
		shown := r.sockets
		r.mu.Unlock()
		if socketsID(discord.Sockets()) == shown {
			return
		}
		s.mu.Lock()
		current := s.runners[r.id] == r
		s.mu.Unlock()
		if current {
			// Off the session's goroutine: the new run stops this one first.
			s.e.Go("addon "+r.id+" restart", func() {
				s.e.Logger.Debug("addons: discord's sockets changed; starting its add-on again")
				s.begin(en)
			})
		}
	}
}

// bundledCredentials are the build credentials of one of Sona's own add-ons,
// sent in its hello answer and nowhere else (never the environment, which
// is an allowlist, nor a log): the Last.fm API account to lastfm, the
// Discord application ID to discord. Any other add-on gets nil.
func bundledCredentials(en addons.Entry) map[string]string {
	if !en.Bundled {
		return nil
	}
	switch en.ID {
	case "lastfm":
		if key, secret, ok := scrobble.LastFMAccount(); ok {
			return map[string]string{cli.CredAPIKey: key, cli.CredAPISecret: secret}
		}
	case "discord":
		if id := discord.ClientID(); id != "" {
			return map[string]string{cli.CredApplicationID: id}
		}
	}
	return nil
}
