package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/update"
)

// releasesPage is opened when no release is known yet.
const releasesPage = "https://github.com/" + update.Repo + "/releases"

// SelfUpdater is MyGo's updater as UpdatesService uses it (main wires it on
// macOS and Windows builds): it swaps the running app for a signed archive.
type SelfUpdater interface {
	// Enabled: this build has a feed and a key, and Sona can write where
	// it is installed.
	Enabled() bool
	// Install downloads the archive through src, which verifies it, and
	// replaces the app. The new version runs after Relaunch.
	Install(ctx context.Context, src *update.Source) error
	// Relaunch quits Sona and starts the installed version.
	Relaunch()
}

// UpdatesService finds newer Sona releases: once per start after the
// network opens (unless settings.updates.check is off) and on request. The
// request carries nothing about the user (internal/update).
//
// Where Sona updates itself (macOS and Windows, a signed build in a place it
// can write) the check is one GET of the release's update-<target>.json and
// Install downloads, verifies and swaps the app in. Elsewhere it asks GitHub
// for the latest release and only reports it: Linux packages update through
// their package manager.
type UpdatesService struct {
	o       *Env
	checker update.Checker // the GitHub API check, where Sona doesn't update itself
	feed    update.Feed    // update-<target>.json, where it does
	target  string         // manifest target ("" on Linux)
	key     string         // the public key signatures must match ("" is off)
	updater SelfUpdater
	guard   *update.Guard // rollback guard (nil without a self-updatable install)

	mu         sync.Mutex
	status     UpdateStatus
	manifest   *update.Source // the update found, minus the download client
	restoring  bool           // the previous version is being put back (never cleared by success: Sona restarts)
	installErr string         // why the last install failed, for Diagnostics (the status carries only the code)
}

func newUpdatesService(e *Env) *UpdatesService {
	s := &UpdatesService{
		o: e, target: update.Target(runtime.GOOS, runtime.GOARCH), key: update.PublicKey(), updater: e.SelfUpdater,
		status: UpdateStatus{Current: e.Version, Checkable: update.Valid(e.Version), AppImage: update.AppImage()},
	}
	client := &http.Client{Transport: e.transport(), Timeout: 15 * time.Second}
	s.checker.HTTP = client
	s.feed.HTTP = client
	s.feed.Key = s.key
	if exe, err := os.Executable(); err == nil && e.Paths.State != "" {
		if g, ok := update.NewGuard(e.Paths.State, exe, runtime.GOOS); ok {
			s.guard = &g
		}
	}
	s.status.SelfUpdate, s.status.Disabled = s.selfState()
	s.status.Declined = s.declined()
	ctx, cancel := context.WithCancel(context.Background())
	e.OnClose(cancel)
	e.OnNetwork(func() {
		// The first paint is what an update on trial must reach.
		if s.guard != nil {
			if err := s.guard.Unmark(); err != nil {
				e.Logger.Warn("update: clearing the pending marker", "err", err)
			}
		}
		if !e.Settings.Get().Updates.Check {
			return
		}
		e.Go("update check", func() {
			st, err := s.check(ctx)
			if err == nil && st.Available {
				e.Emit.Emit(EventUpdateAvailable, st)
			}
		})
	})
	e.OnDiagnostics("Updates", s.bundle)
	return s
}

// selfState says whether this install updates itself, and why not when it
// is a platform that could.
func (s *UpdatesService) selfState() (bool, string) {
	switch {
	case s.target == "" || !s.status.Checkable:
		return false, ""
	case s.key == "":
		return false, UpdatesUnsigned
	case s.updater == nil || !s.updater.Enabled() || s.guard == nil:
		return false, UpdatesNotWritable
	}
	return true, ""
}

// declined is the version the user went back from when this run is the
// version they went back to: the sidebar notice doesn't offer it again.
// Nothing declines a version without that restore on record, or after a
// restore that failed. A restore that didn't know the version it restored
// (a Windows snapshot made before Sona recorded it) counts as this run's.
func (s *UpdatesService) declined() string {
	if s.guard == nil {
		return ""
	}
	if r := s.guard.LastRollback(); r != nil && r.Manual && r.Error == "" && (r.To == "" || r.To == s.status.Current) {
		return r.From
	}
	return ""
}

// bundle is the Diagnostics section: what updates do here, the previous
// version kept, the last rollback (Manual says the user asked for it) and why
// the last install failed.
func (s *UpdatesService) bundle() any {
	st := s.Status()
	out := map[string]any{"current": st.Current, "selfUpdate": st.SelfUpdate, "disabled": st.Disabled, "target": s.target}
	if st.InstallError != "" {
		s.mu.Lock()
		out["installError"] = map[string]any{"code": st.InstallError, "detail": s.installErr, "latest": st.Latest}
		s.mu.Unlock()
	}
	if s.guard != nil {
		if p := s.guard.Pending(); p != nil {
			out["pending"] = p
		}
		if v, ok := s.guard.HasPrevious(); ok {
			out["previous"] = v
		}
		if r := s.guard.LastRollback(); r != nil {
			out["lastRollback"] = r
		}
	}
	return out
}

// Previous reports the version the last update replaced, which is kept
// beside the install (Settings → Diagnostics offers to go back to it). It
// is unavailable where Sona doesn't update itself (Linux, a development
// build, a place it can't write) and while an update installs or a restore
// runs.
func (s *UpdatesService) Previous() PreviousVersion {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.previousLocked()
}

func (s *UpdatesService) previousLocked() PreviousVersion {
	st := &s.status
	if s.guard == nil || !st.SelfUpdate || st.Installing || st.Installed || s.restoring {
		return PreviousVersion{}
	}
	v, ok := s.guard.HasPrevious()
	return PreviousVersion{Available: ok, Version: v}
}

// RestorePrevious puts the previous version back in place of the running
// one and restarts into it. The running version is recorded as the one the
// user went back from (UpdateStatus.Declined, update-rollback.json). Sona
// restarts the way an update does, through the app's normal quit, so
// settings and the queue are saved; nothing the user keeps (settings,
// sign-in, library) is touched. A failure leaves Sona running as it was
// (CodeRestoreFailed), and CodeUnsupported where Sona doesn't update
// itself.
//
// Where the restore renames the install's own files (Windows), the playback
// host is let go of first, as for an update (install), and no play starts
// one from the folder while its entries move. A failed restore releases it.
func (s *UpdatesService) RestorePrevious() error {
	s.mu.Lock()
	if !s.status.SelfUpdate {
		s.mu.Unlock()
		return unsupported("restoring the previous version")
	}
	if !s.previousLocked().Available {
		s.mu.Unlock()
		return fmt.Errorf("%w: there is no previous version to restore right now", update.ErrRestore)
	}
	s.restoring = true
	from := s.status.Current
	s.mu.Unlock()

	release := s.holdHost(context.Background())
	if err := s.guard.RestorePrevious(from); err != nil {
		release()
		s.mu.Lock()
		s.restoring = false
		s.mu.Unlock()
		s.o.Logger.Warn("restoring the previous version failed", "err", err)
		return fmt.Errorf("%w: %v", update.ErrRestore, err)
	}
	s.o.Logger.Info("restored the previous version", "from", from)
	s.o.Go("restore relaunch", s.updater.Relaunch)
	return nil
}

// holdHost lets go of the playback host where swapping the install's files
// needs it (Guard.SwapsInPlace) and returns its release, which does nothing
// when nothing was held. A host that won't let go is logged and the swap
// goes on: it may still work, and if it can't, it fails and says so.
func (s *UpdatesService) holdHost(ctx context.Context) (release func()) {
	release = func() {}
	if !s.guard.SwapsInPlace() || s.o.Coord == nil {
		return release
	}
	r, err := s.o.Coord.HoldHost(ctx)
	if err != nil {
		s.o.Logger.Warn("update: the playback host was not let go of; replacing the files anyway", "err", err)
		return release
	}
	return r
}

// Status returns the last check's result.
func (s *UpdatesService) Status() UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Check asks for the newest release now (Settings → General), whatever the
// start-up setting says. Builds without a release version are not checked.
// A check by hand ends what UpdateStatus.Declined held back.
func (s *UpdatesService) Check(ctx context.Context) (UpdateStatus, error) {
	s.mu.Lock()
	s.status.Declined = ""
	s.mu.Unlock()
	return s.check(ctx)
}

// OpenRelease opens the latest release's page (or the releases list) in
// the browser.
func (s *UpdatesService) OpenRelease() error {
	url := s.Status().URL
	if url == "" {
		url = releasesPage
	}
	return s.o.OpenURL(url)
}

// Install downloads the update found by the last check, verifies its
// signature, replaces the app and restarts into it ("Install and
// restart"). Progress and the outcome arrive as EventUpdateInstall. A
// failure leaves Sona as it was. Platforms and installs that don't update
// themselves refuse (CodeUnsupported).
func (s *UpdatesService) Install(ctx context.Context) (UpdateStatus, error) {
	s.mu.Lock()
	st := &s.status
	switch {
	case !st.SelfUpdate:
		s.mu.Unlock()
		return s.Status(), unsupported("installing updates")
	case st.Installing:
		s.mu.Unlock()
		return s.Status(), nil
	case !st.Available || s.manifest == nil:
		s.mu.Unlock()
		return s.Status(), fmt.Errorf("%w: no update to install", ErrInvalidArgument)
	}
	src := *s.manifest
	src.HTTP = &http.Client{Transport: s.o.transport()}
	src.Key = s.key
	src.Target, src.Current = s.target, st.Current
	src.Progress = s.progress
	st.Installing, st.InstallError, st.Downloaded, st.Total = true, "", 0, src.Update.Size
	s.installErr = ""
	st.Declined = "" // the user chose the version they went back from: the notice follows the install
	from, to := st.Current, st.Latest
	s.mu.Unlock()
	s.emitInstall()

	err := s.install(ctx, &src, from, to)
	s.mu.Lock()
	st.Installing = false
	if err != nil {
		st.InstallError = CodeOf(err)
		s.installErr = err.Error()
		s.o.Logger.Warn("update install failed", "err", err)
	} else {
		st.Installed = true
		s.o.Logger.Info("update installed", "from", from, "to", to)
	}
	out := *st
	s.mu.Unlock()
	s.emitInstall()
	if err != nil {
		return out, err
	}
	s.o.Go("update relaunch", s.updater.Relaunch)
	return out, nil
}

// install stages the running version, marks the update on trial, and has MyGo
// swap it in; the staged version is kept once the new one is in. A failure
// leaves Sona, the version kept before and the marker as they were.
//
// Where the swap renames the install's own files (Windows), the playback host
// is let go of once the archive is downloaded and verified (update.Source's
// Ready), just before MyGo unpacks and swaps: a host that outlived the swap
// would load the new version's plugins from lib\ as it needs them, into a
// process built against the old ones. (A folder whose DLLs a process has
// loaded does rename on Windows 11; a file opened without FILE_SHARE_DELETE
// doesn't, docs/architecture.md.) Playback is paused where it is and no host
// starts until the install has failed, when it is released and a play starts
// one again; after a good install Sona restarts.
func (s *UpdatesService) install(ctx context.Context, src *update.Source, from, to string) error {
	if err := s.guard.Stage(from); err != nil {
		return fmt.Errorf("%w: keeping the current version: %v", update.ErrInstall, err)
	}
	if err := s.guard.Mark(from, to); err != nil {
		s.guard.Abandon()
		return fmt.Errorf("%w: %v", update.ErrInstall, err)
	}
	release := func() {}
	src.Ready = func(ctx context.Context) { release = s.holdHost(ctx) }
	if err := s.updater.Install(ctx, src); err != nil {
		release()
		_ = s.guard.Unmark()
		s.guard.Abandon()
		if errors.Is(err, update.ErrSignature) || errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("%w: %v", update.ErrInstall, err)
	}
	// The new version is in: what it replaced is the previous version. A
	// failure here costs the way back, not the update.
	if err := s.guard.Commit(); err != nil {
		s.o.Logger.Warn("update: keeping the replaced version failed", "err", err)
	}
	return nil
}

// progress records the download and tells the window when a whole percent
// went by.
func (s *UpdatesService) progress(done, total int64) {
	s.mu.Lock()
	st := &s.status
	step := func(n int64) int64 {
		if total <= 0 {
			return 0
		}
		return n * 100 / total
	}
	changed := step(done) != step(st.Downloaded)
	st.Downloaded, st.Total = done, total
	s.mu.Unlock()
	if changed {
		s.emitInstall()
	}
}

func (s *UpdatesService) emitInstall() {
	s.o.Emit.Emit(EventUpdateInstall, s.Status())
}

func (s *UpdatesService) check(ctx context.Context) (UpdateStatus, error) {
	if !s.Status().Checkable {
		return s.Status(), nil
	}
	var (
		rel update.Release
		man *update.Source
		err error
	)
	if s.Status().SelfUpdate {
		rel, man, err = s.checkFeed(ctx)
	} else {
		rel, err = s.checker.Latest(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.status
	switch {
	case errors.Is(err, update.ErrNoRelease):
		st.Latest, st.Available, st.URL, st.Published, st.Notes = "", false, "", 0, ""
		s.manifest = nil
	case err != nil:
		st.Error = CodeOf(err)
		s.o.Logger.Warn("update check failed", "err", err)
		return *st, err
	default:
		st.Latest, st.URL, st.Published, st.Notes = rel.Version, rel.URL, rel.Published.UnixMilli(), update.WithoutVersionHeading(rel.Notes, rel.Version)
		st.Available = update.Newer(rel.Version, st.Current)
		s.manifest = man
	}
	st.Error = ""
	st.Checked = s.o.Now().UnixMilli()
	s.o.Logger.Info("update check", "current", st.Current, "latest", st.Latest, "available", st.Available)
	return *st, nil
}

// checkFeed reads the update manifest of this platform's latest release.
func (s *UpdatesService) checkFeed(ctx context.Context) (update.Release, *update.Source, error) {
	m, raw, err := s.feed.Manifest(ctx, s.target, s.o.Settings.Get().Updates.Prereleases)
	if err != nil {
		return update.Release{}, nil, err
	}
	published, _ := time.Parse(time.RFC3339, m.Date)
	rel := update.Release{Version: m.Version, URL: "https://github.com/" + update.Repo + "/releases/tag/v" + m.Version, Notes: m.Notes, Published: published}
	return rel, &update.Source{Update: m, Raw: raw}, nil
}
