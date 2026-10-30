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

// updatesEnv builds an UpdatesService against a fake GitHub serving
// latest, with the start-up check on or off.
func updatesEnv(t *testing.T, version, latest string, check bool) (*UpdatesService, *Env, *recorder, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v` + latest + `","html_url":"https://github.com/coredev-uk/sona-go/releases/tag/v` + latest + `"}`))
	}))
	t.Cleanup(srv.Close)
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *config.Settings) { s.Updates.Check = check }); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	e := &Env{Deps: Deps{Settings: st, Gate: NewGate(time.Hour, nil), Emit: rec, Logger: discard(), Now: time.Now, Version: version}}
	s := newUpdatesService(e)
	s.checker = update.Checker{API: srv.URL}
	t.Cleanup(e.close)
	return s, e, rec, &hits
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUpdateNoticeForNewerRelease(t *testing.T) {
	s, e, rec, hits := updatesEnv(t, "0.1.0", "0.2.0", true)
	if hits.Load() != 0 {
		t.Fatal("checked before the network opened")
	}
	e.Gate.Open()
	waitFor(t, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.events) > 0
	})
	if rec.events[0] != EventUpdateAvailable {
		t.Fatalf("events = %v", rec.events)
	}
	st := rec.data[0].(UpdateStatus)
	if !st.Available || st.Latest != "0.2.0" || st.Current != "0.1.0" || st.URL == "" || st.Checked == 0 {
		t.Fatalf("status = %+v", st)
	}
	if got := s.Status(); got != st {
		t.Fatalf("Status() = %+v, want %+v", got, st)
	}
}

func TestUpdateNoNoticeWhenCurrent(t *testing.T) {
	s, e, rec, _ := updatesEnv(t, "0.2.0", "0.2.0", true)
	e.Gate.Open()
	waitFor(t, func() bool { return s.Status().Checked != 0 })
	if s.Status().Available || len(rec.events) != 0 {
		t.Fatalf("status %+v, events %v", s.Status(), rec.events)
	}
}

func TestUpdateNoCheckWhenDisabled(t *testing.T) {
	s, e, rec, hits := updatesEnv(t, "0.1.0", "0.2.0", false)
	e.Gate.Open()
	time.Sleep(100 * time.Millisecond)
	if hits.Load() != 0 || len(rec.events) != 0 || s.Status().Checked != 0 {
		t.Fatalf("checked while disabled: hits %d, events %v", hits.Load(), rec.events)
	}
	// An explicit check still runs.
	st, err := s.Check(context.Background())
	if err != nil || !st.Available || hits.Load() != 1 {
		t.Fatalf("Check = %+v, %v (hits %d)", st, err, hits.Load())
	}
}

func TestUpdateDevBuildNeverChecks(t *testing.T) {
	s, e, _, hits := updatesEnv(t, "dev", "0.2.0", true)
	e.Gate.Open()
	st, err := s.Check(context.Background())
	time.Sleep(50 * time.Millisecond)
	if err != nil || st.Checkable || hits.Load() != 0 {
		t.Fatalf("Check = %+v, %v (hits %d)", st, err, hits.Load())
	}
}
