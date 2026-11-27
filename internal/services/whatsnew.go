package services

import (
	"context"
	"errors"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/update"
)

// whatsNewDue says what the start of version current does about the What's
// New dialog, given the newest version whose notes were dealt with (seen) and
// whether the first-run setup was finished or skipped (onboarded).
//
// due is current when it is the first start after an update: it is newer than
// seen, or nothing is recorded on a computer that was set up before Sona
// recorded it. record says to write current as seen without showing anything:
// a fresh install (onboarding covers it) is not an update. A development
// build, a downgrade (a rollback) and a version that was seen are neither.
func whatsNewDue(seen, current string, onboarded bool) (due string, record bool) {
	switch {
	case !update.Valid(current):
		return "", false
	case !update.Valid(seen):
		if !onboarded {
			return "", true
		}
		return current, false
	case update.Newer(current, seen):
		return current, false
	}
	return "", false
}

// initWhatsNew decides at construction (settings only: cold start reads no
// network) whether this start is the first of an update, and fetches its notes
// once the network opens. With the dialog turned off the version is recorded
// as seen: turning it on later shows the next update's notes, not this one's.
func (s *UpdatesService) initWhatsNew(ctx context.Context) {
	cfg := s.o.Settings.Get().Updates
	due, record := whatsNewDue(cfg.WhatsNewSeen, s.status.Current, s.o.Settings.Get().Onboarding.Completed)
	if record || due != "" && !cfg.WhatsNew {
		if err := s.markSeen(s.status.Current); err != nil {
			s.o.Logger.Warn("update: recording the running version", "err", err)
		}
		return
	}
	if due == "" {
		return
	}
	s.o.OnNetwork(func() {
		s.o.Go("whats new", func() { s.fetchWhatsNew(ctx, due) })
	})
}

// fetchWhatsNew reads version's notes and announces them (EventWhatsNew). A
// release without notes is seen at once; a fetch that fails is skipped
// silently (it is logged) and tried again at the next start.
func (s *UpdatesService) fetchWhatsNew(ctx context.Context, version string) {
	w, err := s.notesOf(ctx, version)
	switch {
	case err != nil:
		s.o.Logger.Warn("update: fetching the release notes", "version", version, "err", err)
		return
	case w.Notes == "":
		if err := s.markSeen(version); err != nil {
			s.o.Logger.Warn("update: recording the release notes as seen", "err", err)
		}
		return
	}
	s.mu.Lock()
	s.whatsNew = w
	s.mu.Unlock()
	s.o.Emit.Emit(EventWhatsNew, w)
}

// notesOf is version's release notes from GitHub, the install instructions
// and the version's heading left out. A version with no release has none
// (zero Notes, no error).
func (s *UpdatesService) notesOf(ctx context.Context, version string) (WhatsNew, error) {
	rel, err := s.checker.Version(ctx, version)
	switch {
	case errors.Is(err, update.ErrNoRelease):
		return WhatsNew{}, nil
	case err != nil:
		return WhatsNew{}, err
	}
	notes := update.WithoutVersionHeading(rel.Notes, rel.Version)
	if notes == "" {
		return WhatsNew{}, nil
	}
	return WhatsNew{Version: rel.Version, Published: rel.Published.UnixMilli(), Notes: notes}, nil
}

// WhatsNew is the notes of the version just installed while they wait to be
// shown (zero Version otherwise): the window reads it when it starts, in case
// the fetch finished before it listened for EventWhatsNew.
func (s *UpdatesService) WhatsNew() WhatsNew {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.whatsNew
}

// MarkWhatsNewSeen records that the What's New dialog for version was shown,
// so it is not shown again for it or any older version.
func (s *UpdatesService) MarkWhatsNewSeen(version string) error {
	if err := s.markSeen(version); err != nil {
		return err
	}
	s.mu.Lock()
	if s.whatsNew.Version == version {
		s.whatsNew = WhatsNew{}
	}
	s.mu.Unlock()
	return nil
}

// markSeen raises the recorded version to version; it never lowers it, so a
// rollback and an update again do not show a version twice. Settings are not
// announced: the window does not read the version.
func (s *UpdatesService) markSeen(version string) error {
	if !update.Valid(version) {
		return nil
	}
	_, err := s.o.updateSettingsIf(func(st *config.Settings) {
		if seen := st.Updates.WhatsNewSeen; !update.Valid(seen) || update.Newer(version, seen) {
			st.Updates.WhatsNewSeen = version
		}
	}, func(_, _ config.Settings) bool { return false })
	return err
}

// CurrentNotes fetches the running version's release notes (Settings →
// General → Updates opens them again). Zero Notes says the release has none or
// this is a development build; a failed fetch is the error.
func (s *UpdatesService) CurrentNotes(ctx context.Context) (WhatsNew, error) {
	return s.notesOf(ctx, s.status.Current)
}
