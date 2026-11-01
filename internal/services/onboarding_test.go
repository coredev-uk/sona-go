package services

import (
	"path/filepath"
	"testing"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
)

func TestStartRoute(t *testing.T) {
	ctx := t.Context()
	open := func(t *testing.T, account bool) *db.DB {
		t.Helper()
		d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = d.Close() })
		if account {
			if err := (AccountStore{DB: d}).Save(ctx, auth.Account{StorefrontID: "gb", Language: "en-GB"}); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	settings := func(completed bool, start, last string) config.Settings {
		s := config.Defaults()
		s.Onboarding.Completed = completed
		s.Desktop.StartPage = start
		if last != "" {
			s.Desktop.ResumeLastRoute, s.Desktop.LastRoute = true, last
		}
		return s.Normalized()
	}
	for _, tc := range []struct {
		name      string
		account   bool
		completed bool
		start     string
		last      string
		want      string
	}{
		{"first start", false, false, "/", "", OnboardingRoute},
		{"first start, another start page", false, false, "/charts", "", OnboardingRoute},
		{"setup finished, signed out", false, true, "/charts", "", "/charts"},
		{"signed in before the setup existed", true, false, "/charts", "", "/charts"},
		{"signed in, setup finished", true, true, "/", "", "/"},
		{"resumes elsewhere", true, true, "/", "/album/1", "/album/1"},
		{"never resumes into the setup", true, false, "/charts", "/welcome?step=widevine", "/charts"},
		{"never resumes into the setup, finished", false, true, "/", OnboardingRoute, "/"},
		{"a page that only starts like it", true, true, "/", "/welcomes", "/welcomes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := StartRoute(ctx, open(t, tc.account), settings(tc.completed, tc.start, tc.last))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("StartRoute = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("unreadable database", func(t *testing.T) {
		d := open(t, false)
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := StartRoute(ctx, d, settings(false, "/charts", ""))
		if err == nil {
			t.Fatal("no error from a closed database")
		}
		if got != "/charts" {
			t.Fatalf("StartRoute = %q, want the settings' /charts", got)
		}
	})
}
