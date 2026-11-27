package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/update"
)

func TestWhatsNewDue(t *testing.T) {
	for _, tc := range []struct {
		name          string
		seen, current string
		onboarded     bool
		due           string
		record        bool
	}{
		{"an update", "0.1.0", "0.2.0", true, "0.2.0", false},
		{"an update that skips versions", "0.1.0", "0.5.0", true, "0.5.0", false},
		{"an update to a release candidate", "0.1.0", "0.2.0-rc.1", true, "0.2.0-rc.1", false},
		{"the same version again", "0.2.0", "0.2.0", true, "", false},
		{"a downgrade", "0.3.0", "0.2.0", true, "", false},
		{"a development build", "0.1.0", "dev", true, "", false},
		{"a development build on a fresh install", "", "dev", false, "", false},
		{"a fresh install", "", "0.2.0", false, "", true},
		{"set up before versions were recorded", "", "0.2.0", true, "0.2.0", false},
		{"a version that is not one", "garbage", "0.2.0", true, "0.2.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			due, record := whatsNewDue(tc.seen, tc.current, tc.onboarded)
			if due != tc.due || record != tc.record {
				t.Errorf("whatsNewDue(%q, %q, %v) = %q, %v; want %q, %v", tc.seen, tc.current, tc.onboarded, due, record, tc.due, tc.record)
			}
		})
	}
}

// whatsNewEnv is an UpdatesService on version, whose settings start as
// prepare leaves them, against a fake GitHub that answers the release of tag
// v<version> with body (status 404 when body is ""). tagHits counts the
// requests for it.
func whatsNewEnv(t *testing.T, version, body string, prepare func(*config.Settings)) (*UpdatesService, *Env, *recorder, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/coredev-uk/sona-go/releases/tags/v"+version {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		hits.Add(1)
		if body == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(prepare); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	e := &Env{Deps: Deps{Settings: st, Gate: NewGate(time.Hour, nil), Emit: rec, Logger: discard(), Now: time.Now, Version: version}}
	s := &UpdatesService{o: e, status: UpdateStatus{Current: version, Checkable: true}}
	s.checker = update.Checker{API: srv.URL}
	ctx, cancel := context.WithCancel(context.Background())
	e.OnClose(cancel)
	s.initWhatsNew(ctx)
	t.Cleanup(e.close)
	return s, e, rec, &hits
}

const releaseBody = `{"tag_name":"v0.2.0","published_at":"2026-11-27T12:00:00Z","body":"## [0.2.0](https://example.test/compare) (2026-11-27)\n\n### Features\n\n- a thing (#1)\n\n` + update.InstallNotesMarker + `\n## Installing\n"}`

func upgraded(s *config.Settings) {
	s.Onboarding.Completed = true
	s.Updates.WhatsNew = true
	s.Updates.WhatsNewSeen = "0.1.0"
}

// The first start after an update fetches that version's notes after the
// network opens and not before, announces them without the heading and the
// install instructions, and records the version as seen only once the window
// says it showed them.
func TestWhatsNewAfterAnUpdate(t *testing.T) {
	s, e, rec, hits := whatsNewEnv(t, "0.2.0", releaseBody, upgraded)
	if hits.Load() != 0 {
		t.Fatal("fetched before the network opened")
	}
	e.Gate.Open()
	waitFor(t, func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return len(rec.events) > 0 })
	if rec.events[0] != EventWhatsNew {
		t.Fatalf("events = %v", rec.events)
	}
	want := WhatsNew{Version: "0.2.0", Published: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC).UnixMilli(), Notes: "### Features\n\n- a thing (#1)"}
	if got := rec.data[0].(WhatsNew); got != want {
		t.Fatalf("announced %+v, want %+v", got, want)
	}
	if got := s.WhatsNew(); got != want {
		t.Errorf("WhatsNew() = %+v, want %+v", got, want)
	}
	if seen := e.Settings.Get().Updates.WhatsNewSeen; seen != "0.1.0" {
		t.Fatalf("seen = %q before the dialog was shown", seen)
	}
	if err := s.MarkWhatsNewSeen("0.2.0"); err != nil {
		t.Fatal(err)
	}
	if seen := e.Settings.Get().Updates.WhatsNewSeen; seen != "0.2.0" {
		t.Errorf("seen = %q after the dialog was shown", seen)
	}
	if got := s.WhatsNew(); got.Version != "" {
		t.Errorf("WhatsNew() = %+v after it was shown", got)
	}
	// Once seen, the next start is not an update.
	if due, _ := whatsNewDue(e.Settings.Get().Updates.WhatsNewSeen, "0.2.0", true); due != "" {
		t.Errorf("due %q again", due)
	}
}

// A fresh install is not an update: nothing is fetched and the version is
// recorded, so the next update is the first one shown.
func TestWhatsNewNotOnAFreshInstall(t *testing.T) {
	_, e, rec, hits := whatsNewEnv(t, "0.2.0", releaseBody, func(s *config.Settings) { s.Updates.WhatsNew = true })
	e.Gate.Open()
	time.Sleep(100 * time.Millisecond)
	if hits.Load() != 0 || len(rec.events) != 0 {
		t.Fatalf("fetched on a fresh install: hits %d, events %v", hits.Load(), rec.events)
	}
	if seen := e.Settings.Get().Updates.WhatsNewSeen; seen != "0.2.0" {
		t.Errorf("seen = %q, want the running version", seen)
	}
}

func TestWhatsNewNotOnADowngradeOrADevBuild(t *testing.T) {
	for _, version := range []string{"0.0.9", "dev"} {
		t.Run(version, func(t *testing.T) {
			_, e, rec, hits := whatsNewEnv(t, version, releaseBody, upgraded)
			e.Gate.Open()
			time.Sleep(100 * time.Millisecond)
			if hits.Load() != 0 || len(rec.events) != 0 {
				t.Fatalf("fetched: hits %d, events %v", hits.Load(), rec.events)
			}
			if seen := e.Settings.Get().Updates.WhatsNewSeen; seen != "0.1.0" {
				t.Errorf("seen = %q, a downgrade or a dev build must leave it", seen)
			}
		})
	}
}

// Turned off, nothing is fetched, and turning it on later does not show the
// notes of a version that came while it was off.
func TestWhatsNewOffRecordsTheVersionAndFetchesNothing(t *testing.T) {
	_, e, rec, hits := whatsNewEnv(t, "0.2.0", releaseBody, func(s *config.Settings) {
		upgraded(s)
		s.Updates.WhatsNew = false
	})
	e.Gate.Open()
	time.Sleep(100 * time.Millisecond)
	if hits.Load() != 0 || len(rec.events) != 0 {
		t.Fatalf("fetched while off: hits %d, events %v", hits.Load(), rec.events)
	}
	if seen := e.Settings.Get().Updates.WhatsNewSeen; seen != "0.2.0" {
		t.Errorf("seen = %q", seen)
	}
}

// A release without notes is seen at once and shows nothing; a missing
// release is the same.
func TestWhatsNewReleaseWithoutNotesIsSeen(t *testing.T) {
	for name, body := range map[string]string{
		"empty body":    `{"tag_name":"v0.2.0","body":""}`,
		"only the head": `{"tag_name":"v0.2.0","body":"## [0.2.0](https://example.test) (2026-11-27)\n"}`,
		"no release":    "",
	} {
		t.Run(name, func(t *testing.T) {
			_, e, rec, hits := whatsNewEnv(t, "0.2.0", body, upgraded)
			e.Gate.Open()
			waitFor(t, func() bool { return e.Settings.Get().Updates.WhatsNewSeen == "0.2.0" })
			if hits.Load() != 1 || len(rec.events) != 0 {
				t.Errorf("hits %d, events %v", hits.Load(), rec.events)
			}
		})
	}
}

// A failed fetch is skipped silently and tried again at the next start: the
// version stays unseen.
func TestWhatsNewFailedFetchIsSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	s, e, rec, _ := whatsNewEnv(t, "0.2.0", releaseBody, upgraded)
	s.checker = update.Checker{API: srv.URL}
	e.Gate.Open()
	time.Sleep(100 * time.Millisecond)
	if len(rec.events) != 0 || e.Settings.Get().Updates.WhatsNewSeen != "0.1.0" {
		t.Errorf("events %v, seen %q", rec.events, e.Settings.Get().Updates.WhatsNewSeen)
	}
}

func TestMarkWhatsNewSeenNeverLowersTheVersion(t *testing.T) {
	s, e, _, _ := whatsNewEnv(t, "0.2.0", releaseBody, func(s *config.Settings) { s.Updates.WhatsNewSeen = "0.3.0" })
	if err := s.MarkWhatsNewSeen("0.2.0"); err != nil {
		t.Fatal(err)
	}
	if seen := e.Settings.Get().Updates.WhatsNewSeen; seen != "0.3.0" {
		t.Errorf("seen = %q", seen)
	}
}

// UpdateSettings from a window holding an older copy must not forget the
// version seen, or the dialog would open again.
func TestUpdateSettingsKeepsTheVersionSeen(t *testing.T) {
	_, e, _, _ := whatsNewEnv(t, "0.2.0", releaseBody, func(s *config.Settings) { s.Updates.WhatsNewSeen = "0.2.0" })
	stale := e.Settings.Get()
	stale.Updates.WhatsNewSeen = "0.1.0"
	sys := &SystemService{o: e}
	saved, err := sys.UpdateSettings(stale)
	if err != nil || saved.Updates.WhatsNewSeen != "0.2.0" {
		t.Fatalf("saved %q, %v", saved.Updates.WhatsNewSeen, err)
	}
}

func TestCurrentNotes(t *testing.T) {
	s, _, _, _ := whatsNewEnv(t, "0.2.0", releaseBody, func(s *config.Settings) { s.Updates.WhatsNewSeen = "0.2.0" })
	w, err := s.CurrentNotes(context.Background())
	if err != nil || w.Version != "0.2.0" || w.Notes != "### Features\n\n- a thing (#1)" {
		t.Fatalf("CurrentNotes = %+v, %v", w, err)
	}
}
