package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
)

// OnboardingRoute is the first-run setup's page (frontend
// features/onboarding). It marks config.Onboarding.Completed through
// SystemService.UpdateSettings when it is finished or skipped.
const OnboardingRoute = "/welcome"

// StartRoute is the route the main window opens on. A computer where the
// first-run setup was neither finished nor skipped, and where no Apple
// Music account is signed in (the account Sona keeps in SQLite: cold start
// reads nothing else, so the keyring is not asked), opens on the setup.
// Every other start opens where Settings → General says, except that
// resuming never lands on the setup again. When the database can't be
// read, the settings' route comes back with the error.
func StartRoute(ctx context.Context, d *db.DB, s config.Settings) (string, error) {
	route := s.Desktop.StartRoute()
	if isOnboardingRoute(route) {
		route = s.Desktop.StartPage
	}
	if s.Onboarding.Completed {
		return route, nil
	}
	_, err := d.RQ.GetAccount(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return OnboardingRoute, nil
	case err != nil:
		return route, fmt.Errorf("start route: read the account: %w", err)
	}
	return route, nil
}

// isOnboardingRoute reports whether r is the setup's page, with or
// without its search string.
func isOnboardingRoute(r string) bool {
	rest, ok := strings.CutPrefix(r, OnboardingRoute)
	return ok && (rest == "" || rest[0] == '?')
}
