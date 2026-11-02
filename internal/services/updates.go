package services

import (
	"context"
	"errors"
	"sync"

	"github.com/coredev-uk/sona-go/internal/update"
)

// releasesPage is opened when no release is known yet.
const releasesPage = "https://github.com/" + update.Repo + "/releases"

// UpdatesService checks GitHub for a newer Sona release: once per start
// after the network opens (unless settings.updates.check is off) and on
// request. The request carries nothing about the user (internal/update).
type UpdatesService struct {
	o       *Env
	checker update.Checker

	mu     sync.Mutex
	status UpdateStatus
}

func newUpdatesService(e *Env) *UpdatesService {
	s := &UpdatesService{o: e, status: UpdateStatus{
		Current:   e.Version,
		Checkable: update.Valid(e.Version),
		AppImage:  update.AppImage(),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	e.OnClose(cancel)
	e.OnNetwork(func() {
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
	return s
}

// Status returns the last check's result.
func (s *UpdatesService) Status() UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Check asks GitHub now (Settings → Diagnostics), whatever the start-up
// setting says. Builds without a release version are not checked.
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

func (s *UpdatesService) check(ctx context.Context) (UpdateStatus, error) {
	if !s.Status().Checkable {
		return s.Status(), nil
	}
	rel, err := s.checker.Latest(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.status
	switch {
	case errors.Is(err, update.ErrNoRelease):
		st.Latest, st.Available, st.URL, st.Published = "", false, "", 0
	case err != nil:
		st.Error = CodeOf(err)
		s.o.Logger.Warn("update check failed", "err", err)
		return *st, err
	default:
		st.Latest, st.URL, st.Published = rel.Version, rel.URL, rel.Published.UnixMilli()
		st.Available = update.Newer(rel.Version, st.Current)
	}
	st.Error = ""
	st.Checked = s.o.Now().UnixMilli()
	s.o.Logger.Info("update check", "current", st.Current, "latest", st.Latest, "available", st.Available)
	return *st, nil
}
