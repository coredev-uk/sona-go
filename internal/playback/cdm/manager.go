// Package cdm gets Google's Widevine CDM for Sona: it downloads the CDM
// from Google's servers, as Firefox's GMP updater does, verifies it and
// keeps it installed and up to date in Sona's data directory. Browser and
// system copies are never used; SONA_WIDEVINE_CDM (EnvOverride) is the
// developer override.
//
// The version, package URL and SHA-512 come from Mozilla's public Firefox
// GMP manifest (Mozilla, pinned); the package is accepted only from
// Google's download hosts over HTTPS, only with the manifest's size and
// SHA-512, and only as a CRX3 whose signatures prove Google's Widevine
// component key and Google's publisher key (Widevine). Only the platform's
// library, its signature file and the component manifest are extracted,
// with size limits. A new version becomes current only after that and a
// load check, and the previous one is kept for rollback (ReportInit). The
// CDM is never patched, and the playback host child that loads it makes
// no network requests: everything here runs in the parent.
package cdm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is where the CDM stands.
type State string

const (
	// StateIdle: nothing installed and no download yet (it starts after
	// the first paint).
	StateIdle State = "idle"
	// StateDownloading: nothing installed; the download is running.
	StateDownloading State = "downloading"
	// StateReady: a CDM is installed (or the override names one).
	StateReady State = "ready"
	// StateFailed: nothing usable; Code says why.
	StateFailed State = "failed"
)

// Where the CDM comes from.
const (
	SourceDownload = "download"
	SourceOverride = "override"
)

// Status is the CDM's state for the UI and Diagnostics.
type Status struct {
	State State
	// Source is SourceDownload or SourceOverride.
	Source string
	// Version and Path are the CDM new playback hosts load ("" when
	// none).
	Version, Path string
	// Code: why State is StateFailed, or, when ready, why the last update
	// check failed ("" when it passed).
	Code string
	// Received and Total are the running download's progress in bytes.
	Received, Total int64
	// Updating: a newer version is downloading while one is ready; it
	// applies to the next playback host.
	Updating bool
	// CheckedAt is the last update check that completed.
	CheckedAt time.Time
}

// rollbackAfter is how many playback hosts in a row may fail to load the
// current version before Sona rolls back to the previous one.
const rollbackAfter = 3

// Manager keeps the CDM installed: it installs one after the first paint
// when there is none, checks for updates at most once per UpdateInterval,
// rolls back a version that playback hosts fail to load, and replaces an
// installed library damaged on disk (checked against the SHA-256 recorded
// at install when it starts and when a playback host fails to load it).
// A new version applies to the next playback host; a running one keeps
// its library.
type Manager struct {
	o     Options
	s     store
	ctx   context.Context
	stop  context.CancelFunc
	wake  chan struct{}
	wg    sync.WaitGroup
	start sync.Once
	// op serialises update checks and init records in this process (the
	// directory's file lock does so across processes).
	op sync.Mutex

	mu       sync.Mutex
	closed   bool
	loaded   bool
	ptr      pointer
	st       Status
	failures int // update checks in a row that failed (backoff)
	lastTry  time.Time
	// unsupported: Google ships no CDM for this platform; only the
	// override can name one.
	unsupported bool
	subs        map[int]func(Status)
	nextSub     int
	// reports are ReportInit calls waiting to be recorded, in call order;
	// recording is true while a goroutine records them.
	reports   []initReport
	recording bool
	// installing is closed when the running check finishes, once it has
	// verified a first version (nothing else installed) and is loading
	// it: Library waits for it, up to installWait. Nil otherwise.
	installing  chan struct{}
	installWait time.Duration
}

// initReport is one ReportInit call.
type initReport struct {
	lib string
	err error
}

// New returns a Manager. It reads nothing until it is first asked. On a
// platform Google ships no CDM for, the Manager reports
// CodeUnsupportedPlatform and never downloads.
func New(o Options) (*Manager, error) {
	o, err := o.withDefaults()
	unsupported := CodeOf(err) == CodeUnsupportedPlatform
	if err != nil && !unsupported {
		return nil, err
	}
	m := &Manager{
		o:    o,
		s:    store{root: o.Dir, p: o.Platform},
		wake: make(chan struct{}, 1),
		subs: map[int]func(Status){},
		st:   Status{State: StateIdle, Source: SourceDownload},

		unsupported: unsupported,
		installWait: installWait,
	}
	if o.Override != "" {
		m.st.Source = SourceOverride
	}
	m.ctx, m.stop = context.WithCancel(context.Background())
	return m, nil
}

// load reads the pointer (or resolves the override) once. Call with mu
// held.
func (m *Manager) loadLocked() {
	if m.loaded {
		return
	}
	m.loaded = true
	if m.o.Override != "" {
		lib, err := resolveOverride(m.o.Override, m.o.Platform)
		if err != nil {
			m.st.State, m.st.Code = StateFailed, CodeOverrideMissing
			m.o.Logger.Warn("widevine: "+EnvOverride+" names no CDM", "value", m.o.Override, "err", err)
			return
		}
		m.st.State, m.st.Path, m.st.Version = StateReady, lib, LibraryVersion(lib)
		m.o.Logger.Info("widevine: using the developer override "+EnvOverride+"; nothing is downloaded", "path", lib, "version", m.st.Version)
		return
	}
	if m.unsupported {
		m.st.State, m.st.Code = StateFailed, CodeUnsupportedPlatform
		return
	}
	ptr, err := m.s.read()
	if err != nil {
		m.o.Logger.Warn("widevine: install directory unreadable", "err", err)
	}
	m.adoptLocked(ptr)
}

// adoptLocked takes ptr as the installed state. Call with mu held.
func (m *Manager) adoptLocked(ptr pointer) {
	m.ptr = ptr
	m.st.CheckedAt = ptr.CheckedAt
	if m.s.installed(ptr.Current) {
		m.st.State, m.st.Version, m.st.Path = StateReady, ptr.Current, m.s.lib(ptr.Current)
		return
	}
	m.st.Version, m.st.Path = "", ""
	if m.st.State == StateReady {
		m.st.State = StateIdle
	}
}

// Status returns the CDM's state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
	return m.st
}

// Source is where the CDM comes from: SourceOverride when the developer
// override is set, else SourceDownload. It reads nothing.
func (m *Manager) Source() string {
	if m.o.Override != "" {
		return SourceOverride
	}
	return SourceDownload
}

// Subscribe calls fn with every status change until cancel.
func (m *Manager) Subscribe(fn func(Status)) (cancel func()) {
	m.mu.Lock()
	id := m.nextSub
	m.nextSub++
	m.subs[id] = fn
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}
}

// setLocked changes the status and returns the notification to run after
// unlocking.
func (m *Manager) setLocked(change func(*Status)) func() {
	old := m.st
	change(&m.st)
	if m.st == old {
		return func() {}
	}
	st := m.st
	fns := make([]func(Status), 0, len(m.subs))
	for _, fn := range m.subs {
		fns = append(fns, fn)
	}
	return func() {
		for _, fn := range fns {
			fn(st)
		}
	}
}

// installWait is how long Library waits for a first install that is
// moments away (its package verified and in place, the load check
// running) before reporting ErrDownloading.
const installWait = 20 * time.Second

// Library returns the library a new playback host loads: the override
// when set, else the current installed version. Without one it fails
// with ErrDownloading or ErrUnavailable (both playback.ErrNoCDM), the
// latter wrapped in an Error carrying Status.Code when there is one.
// While the first version is past its download and being checked, it
// waits for that check (until ctx is done, at most installWait) rather
// than fail a play the install is moments from serving; a download
// still running fails at once, and the UI shows its progress.
func (m *Manager) Library(ctx context.Context) (string, error) {
	m.mu.Lock()
	m.loadLocked()
	if done := m.installing; done != nil {
		m.mu.Unlock()
		t := time.NewTimer(m.installWait)
		select {
		case <-done:
		case <-ctx.Done():
		case <-t.C:
		}
		t.Stop()
		m.mu.Lock()
	}
	defer m.mu.Unlock()
	switch {
	case m.st.State == StateReady:
		return m.st.Path, nil
	case m.st.State == StateDownloading:
		return "", ErrDownloading
	case m.st.Code != "":
		return "", &Error{Code: m.st.Code, Err: ErrUnavailable}
	}
	return "", ErrUnavailable
}

// Start begins keeping the CDM installed: at once when none is installed,
// else when an update check is due. Call it after the first paint (it
// reads the network). It does nothing with the override set, or where
// Google ships no CDM.
func (m *Manager) Start() {
	m.start.Do(func() {
		m.mu.Lock()
		m.loadLocked()
		if m.closed || m.o.Override != "" || m.unsupported {
			m.mu.Unlock()
			return
		}
		m.wg.Add(1)
		m.mu.Unlock()
		go m.loop()
	})
}

// Retry runs an update check now (the UI's Retry after a failure).
func (m *Manager) Retry() {
	m.Start()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Close stops the checks and any download, and waits for them.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.stop()
	m.wg.Wait()
}

func (m *Manager) loop() {
	defer m.wg.Done()
	m.repairInstalled()
	for {
		if wait := m.untilCheck(); wait > 0 {
			select {
			case <-m.o.After(wait):
			case <-m.wake:
			case <-m.ctx.Done():
				return
			}
		}
		if m.ctx.Err() != nil {
			return
		}
		m.runCheck()
	}
}

// untilCheck is how long until the next update check: none installed,
// at once; else the update interval after the last completed check. After
// failed checks the wait backs off from the retry interval, doubling up
// to the update interval.
func (m *Manager) untilCheck() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.o.Now()
	var due time.Time
	if m.s.installed(m.ptr.Current) && !m.ptr.CheckedAt.IsZero() {
		due = m.ptr.CheckedAt.Add(m.o.UpdateInterval)
	}
	if m.failures > 0 {
		backoff := m.o.RetryInterval << min(m.failures-1, 16)
		if backoff <= 0 || backoff > m.o.UpdateInterval {
			backoff = m.o.UpdateInterval
		}
		if retry := m.lastTry.Add(backoff); retry.After(due) {
			due = retry
		}
	}
	return due.Sub(now)
}

// runCheck runs one update check and publishes its outcome.
func (m *Manager) runCheck() {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	installed := m.s.installed(m.ptr.Current)
	notify := m.setLocked(func(s *Status) {
		s.Received, s.Total, s.Code = 0, 0, ""
		if installed {
			s.Updating = true
		} else {
			s.State = StateDownloading
		}
	})
	m.mu.Unlock()
	notify()

	var lastPct int64 = -1
	progress := func(received, total int64) {
		pct := received * 100 / max(total, 1)
		if pct == lastPct {
			return
		}
		lastPct = pct
		m.mu.Lock()
		notify := m.setLocked(func(s *Status) { s.Received, s.Total = received, total })
		m.mu.Unlock()
		notify()
	}
	verified := func() {
		if installed {
			return
		}
		m.mu.Lock()
		m.installing = make(chan struct{})
		m.mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(m.ctx, checkTimeout)
	t0 := time.Now()
	ptr, changed, err := check(ctx, m.o, progress, verified)
	cancel()

	m.mu.Lock()
	m.lastTry = m.o.Now()
	if err != nil {
		m.failures++
	} else {
		m.failures = 0
	}
	if ptr.Platform != "" {
		m.ptr = ptr
	}
	notify = m.setLocked(func(s *Status) {
		s.Updating, s.Received, s.Total = false, 0, 0
		m.adoptLocked(m.ptr)
		if err != nil {
			s.Code = CodeOf(err)
			if s.State != StateReady {
				s.State = StateFailed
			}
		}
	})
	// Waiting Library calls read the outcome just published.
	if m.installing != nil {
		close(m.installing)
		m.installing = nil
	}
	m.mu.Unlock()
	notify()
	switch {
	case err != nil && m.ctx.Err() == nil:
		m.o.Logger.Warn("widevine: update check failed", "code", CodeOf(err), "err", err, "took", time.Since(t0).Round(time.Millisecond))
	case changed:
		m.o.Logger.Info("widevine: new version applies to the next playback host", "version", ptr.Current)
	case err == nil:
		m.o.Logger.Info("widevine: up to date", "version", ptr.Current, "took", time.Since(t0).Round(time.Millisecond))
	}
}

// ReportInit tells the manager how a playback host's start went with
// lib: err nil when it loaded the CDM, else the CDM failure (the library
// failed to load or initialise, or the host died while loading it; not a
// sandbox or audio failure). After rollbackAfter failures in a row with
// the current version, the previous one becomes current again and the
// failed one is removed and never installed again. With no previous
// version nothing changes. Hosts using the override are not counted. It
// returns at once: reports are recorded in call order, after a running
// update check.
func (m *Manager) ReportInit(lib string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
	if m.closed || m.o.Override != "" {
		return
	}
	// While earlier reports wait, the pointer here may be stale: queue
	// everything and let recordInit check it against the file. Otherwise a
	// report for another library, or a success with no failures on
	// record, changes nothing.
	if !m.recording && (m.ptr.Current == "" || lib != m.s.lib(m.ptr.Current) || err == nil && m.ptr.Failures == 0) {
		return
	}
	m.reports = append(m.reports, initReport{lib: lib, err: err})
	if m.recording {
		return
	}
	m.recording = true
	m.wg.Add(1)
	go m.recordReports()
}

// recordReports records the queued reports one at a time, oldest first.
func (m *Manager) recordReports() {
	defer m.wg.Done()
	for {
		m.mu.Lock()
		if len(m.reports) == 0 {
			m.recording = false
			m.mu.Unlock()
			return
		}
		r := m.reports[0]
		m.reports = m.reports[1:]
		m.mu.Unlock()
		m.recordInit(r.lib, r.err)
	}
}

// recordInit is ReportInit's work, serialised with update checks.
func (m *Manager) recordInit(lib string, err error) {
	m.op.Lock()
	defer m.op.Unlock()
	unlock, lerr := m.s.lock()
	if lerr != nil {
		m.o.Logger.Warn("widevine: record the playback host's start", "err", lerr)
		return
	}
	defer unlock()
	ptr, rerr := m.s.read()
	if rerr != nil || ptr.Current == "" || lib != m.s.lib(ptr.Current) {
		return
	}
	// A load failure may be a library damaged on disk rather than a bad
	// version: that one is replaced, not counted towards a rollback.
	if err != nil && m.s.repair(&ptr, m.o.Logger) {
		m.commitRepair(ptr, true)
		return
	}
	if err == nil {
		ptr.Failures = 0
	} else {
		ptr.Failures++
	}
	rollback := ptr.Failures >= rollbackAfter && m.s.installed(ptr.Previous)
	bad := ptr.Current
	if rollback {
		if ptr.Rejected == nil {
			ptr.Rejected = map[string]string{}
		}
		ptr.Rejected[bad] = CodeLoad
		delete(ptr.SHA256, bad)
		ptr.Current, ptr.Previous, ptr.Failures = ptr.Previous, "", 0
	}
	werr := m.s.write(ptr)
	m.mu.Lock()
	if werr == nil {
		m.ptr = ptr
	}
	notify := m.setLocked(func(*Status) { m.adoptLocked(m.ptr) })
	m.mu.Unlock()
	notify()
	switch {
	case werr != nil:
		m.o.Logger.Warn("widevine: record the playback host's start", "err", werr)
	case rollback:
		_ = os.RemoveAll(filepath.Join(m.o.Dir, bad))
		m.o.Logger.Warn("widevine: rolled back; the new version failed to load", "failed", bad, "version", ptr.Current, "failures", rollbackAfter, "err", err)
	case err != nil:
		m.o.Logger.Warn("widevine: the playback host failed to load the CDM", "version", ptr.Current, "failures", ptr.Failures, "err", err)
	}
}

// repairInstalled checks the installed libraries against the hashes
// recorded at install (store.repair) when the manager starts, off the
// first-paint path; a damaged one is replaced by the first check.
func (m *Manager) repairInstalled() {
	m.op.Lock()
	defer m.op.Unlock()
	unlock, err := m.s.lock()
	if err != nil {
		m.o.Logger.Warn("widevine: check the installed CDM", "err", err)
		return
	}
	defer unlock()
	ptr, err := m.s.read()
	if err != nil || !m.s.repair(&ptr, m.o.Logger) {
		return
	}
	m.commitRepair(ptr, false)
}

// commitRepair saves and publishes a repaired pointer. Without a current
// version left, wake asks the loop to download it again now. Call with op
// and the directory's lock held.
func (m *Manager) commitRepair(ptr pointer, wake bool) {
	if err := m.s.write(ptr); err != nil {
		m.o.Logger.Warn("widevine: save the repaired install", "err", err)
	}
	m.mu.Lock()
	m.ptr = ptr
	notify := m.setLocked(func(*Status) { m.adoptLocked(m.ptr) })
	m.mu.Unlock()
	notify()
	if wake && ptr.Current == "" {
		select {
		case m.wake <- struct{}{}:
		default:
		}
	}
}

// resolveOverride returns the library the override names: the path
// itself, or the platform's library in that directory.
func resolveOverride(p string, pl Platform) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%s is not an absolute path", p)
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		p = filepath.Join(p, pl.Lib)
		if st, err = os.Stat(p); err != nil {
			return "", err
		}
	}
	if !st.Mode().IsRegular() {
		return "", errors.New(p + " is not a file")
	}
	return p, nil
}
