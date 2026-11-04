package fixture

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
)

// WriteProfile creates the profile of a signed-out user with the fixture
// library under paths: settings.json (the first-run setup marked done, so
// the app opens on Albums) and a migrated sona.db seeded with Albums and
// Playlists. It writes no credentials.
func WriteProfile(ctx context.Context, paths config.Paths) error {
	if err := paths.Ensure(); err != nil {
		return err
	}
	s, err := config.OpenStore(paths.SettingsFile())
	if err != nil {
		return err
	}
	if _, err := s.Update(func(s *config.Settings) {
		s.Onboarding.Completed = true
		s.Theme = config.ThemeLight
		s.Language = "en"
		s.AutoPlay = false
	}); err != nil {
		return err
	}
	d, err := db.Open(ctx, paths.DatabaseFile())
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return Seed(ctx, d, Albums(), Playlists())
}
