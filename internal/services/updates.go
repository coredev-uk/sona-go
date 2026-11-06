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

	mu       sync.Mutex
	status   UpdateStatus
	manifest *update.Source // the update found, minus the download client
}

func newUpdatesService(e *Env) *UpdatesService {
	s := &UpdatesService{
		o: e, target: update.Target(runtime.GOOS, runtime.GOARCH), key: update.PublicKey(), updater: e.SelfUpdater,
		status: UpdateStatus{Current: e.Version, Checkable: update.Valid(e.Version), AppImage: update.AppImage()},
	}
	client := &http.Client{Transport: e.transport(), Timeout: 15 * time.Second}
	s.checker.HTTP = client
	s.feed.HTTP = client
	if exe, err := os.Executable(); err == nil && e.Paths.State != "" {
		if g, ok := update.NewGuard(e.Paths.State, exe, runtime.GOOS); ok {
			s.guard = &g
		}
	}
	s.status.SelfUpdate, s.status.Disabled = s.selfState()
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

// bundle is the Diagnostics section: what updates do here and the last
// rollback.
func (s *UpdatesService) bundle() any {
	st := s.Status()
	out := map[string]any{"current": st.Current, "selfUpdate": st.SelfUpdate, "disabled": st.Disabled, "target": s.target}
	if s.guard != nil {
		if p := s.guard.Pending(); p != nil {
			out["pending"] = p
		}
		if r := s.guard.LastRollback(); r != nil {
			out["lastRollback"] = r
		}
	}
	return out
}

// Status returns the last check's result.
func (s *UpdatesService) Status() UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Check asks for the newest release now (Settings → General), whatever the
// start-up setting says. Builds without a release version are not checked.
func (s *UpdatesService) Check(ctx context.Context) (UpdateStatus, error) {
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
	src.Progress = s.progress
	st.Installing, st.InstallError, st.Downloaded, st.Total = true, "", 0, src.Update.Size
	from, to := st.Current, st.Latest
	s.mu.Unlock()
	s.emitInstall()

	err := s.install(ctx, &src, from, to)
	s.mu.Lock()
	st.Installing = false
	if err != nil {
		st.InstallError = CodeOf(err)
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

// install keeps the running version, marks the update on trial, and has
// MyGo swap it in. Nothing stays marked after a failure.
func (s *UpdatesService) install(ctx context.Context, src *update.Source, from, to string) error {
	if err := s.guard.Snapshot(); err != nil {
		return fmt.Errorf("%w: keeping the current version: %v", update.ErrInstall, err)
	}
	if err := s.guard.Mark(from, to); err != nil {
		return fmt.Errorf("%w: %v", update.ErrInstall, err)
	}
	if err := s.updater.Install(ctx, src); err != nil {
		_ = s.guard.Unmark()
		if errors.Is(err, update.ErrSignature) || errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("%w: %v", update.ErrInstall, err)
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
		rel   update.Release
		man   *update.Source
		notes string
		err   error
	)
	if s.Status().SelfUpdate {
		rel, man, err = s.checkFeed(ctx)
		if man != nil {
			notes = man.Update.Notes
		}
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
		st.Latest, st.URL, st.Published, st.Notes = rel.Version, rel.URL, rel.Published.UnixMilli(), notes
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
	rel := update.Release{Version: m.Version, URL: "https://github.com/" + update.Repo + "/releases/tag/v" + m.Version, Published: published}
	return rel, &update.Source{Update: m, Raw: raw}, nil
}
