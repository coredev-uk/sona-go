package store

import (
	"context"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

func TestCDMProgressIsClampedAndUnknownWithoutATotal(t *testing.T) {
	for _, c := range []struct {
		recv, total int64
		want        float64
		known       bool
	}{
		{50, 200, 0.25, true},
		{0, 100, 0, true},
		{300, 100, 1, true}, // never past the end
		{-5, 100, 0, true},
		{10, 0, 0, false}, // size unknown
		{10, -1, 0, false},
	} {
		got, ok := CDMProgress(services.CDMStatus{Received: c.recv, Total: c.total})
		if got != c.want || ok != c.known {
			t.Errorf("CDMProgress(%d of %d) = %v, %v; want %v, %v", c.recv, c.total, got, ok, c.want, c.known)
		}
	}
}

func TestCDMCanRetry(t *testing.T) {
	st := func(state, code string, updating bool) *services.CDMStatus {
		return &services.CDMStatus{State: state, Code: code, Updating: updating}
	}
	for _, c := range []struct {
		name string
		s    *services.CDMStatus
		want bool
	}{
		{"nothing known yet", nil, false},
		{"failed download", st("failed", "network", false), true},
		{"ready, the update check failed", st("ready", "network", false), true},
		{"ready and fine", st("ready", "", false), false},
		{"idle", st("idle", "", false), false},
		{"downloading", st("downloading", "", false), false},
		{"unsupported platform", st("failed", "unsupported_platform", false), false},
		{"while an update downloads", st("ready", "network", true), false},
		{"failed with no code", st("failed", "", false), false},
	} {
		if got := CDMCanRetry(c.s); got != c.want {
			t.Errorf("%s: CDMCanRetry = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPrivateForcedOnlyBySONAPrivate(t *testing.T) {
	for source, want := range map[string]bool{"environment": true, "setting": false, "development": false, "": false} {
		if got := PrivateForced(services.PrivateState{On: true, Source: source}); got != want {
			t.Errorf("source %q: forced = %v, want %v", source, got, want)
		}
	}
}

func TestPeekReadsWithoutFetchingAndSetIfEmptyKeepsNewerData(t *testing.T) {
	cl, _ := newTestClient()
	if _, ok := Peek[int](cl, Key{"n"}); ok {
		t.Error("peeked data that was never set")
	}
	if !SetIfEmpty(cl, Key{"n"}, 1) {
		t.Fatal("SetIfEmpty did not fill an empty query")
	}
	if SetIfEmpty(cl, Key{"n"}, 2) {
		t.Error("SetIfEmpty replaced data")
	}
	if v, _ := Peek[int](cl, Key{"n"}); v != 1 {
		t.Errorf("peek = %d, want 1", v)
	}
	if _, ok := Peek[string](cl, Key{"n"}); ok {
		t.Error("peeked an int as a string")
	}
	Set(cl, Key{"n"}, 3)
	if v, _ := Peek[int](cl, Key{"n"}); v != 3 {
		t.Errorf("peek after Set = %d, want 3", v)
	}
}

func TestReminderForFindsByCatalogAlbumID(t *testing.T) {
	list := []services.Reminder{{AlbumID: "a", Name: "A"}, {AlbumID: "b", Name: "B"}}
	if r, ok := ReminderFor(list, "b"); !ok || r.Name != "B" {
		t.Errorf("ReminderFor(b) = %+v, %v", r, ok)
	}
	if _, ok := ReminderFor(list, "c"); ok {
		t.Error("found a reminder that is not there")
	}
	if _, ok := ReminderFor(nil, "a"); ok {
		t.Error("found a reminder in no list")
	}
}

func TestSharedMirrorsAreEmptyWithoutServices(t *testing.T) {
	cl, _ := newTestClient()
	if r := UsePrivate(cl, nil); r.HasData {
		t.Error("private data without services")
	}
	if r := UseCDM(cl, nil); r.HasData {
		t.Error("cdm data without services")
	}
	if r := UseUpdates(cl, nil); r.HasData {
		t.Error("update data without services")
	}
	if r := UseReminders(cl, nil); r.HasData {
		t.Error("reminders without services")
	}
}

func TestSetSupersedesAFetchInFlight(t *testing.T) {
	cl, _ := newTestClient()
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	fetch := func(ctx context.Context) (int, error) {
		started <- struct{}{}
		<-release
		return 1, nil // what was true before the write
	}
	Use(cl, Key{"n"}, fetch)
	<-started
	Set(cl, Key{"n"}, 2)
	close(release)
	time.Sleep(20 * time.Millisecond) // the fetch's answer has been posted (the client's poster runs at once)
	r := Use(cl, Key{"n"}, fetch)
	if !r.HasData || r.Data != 2 || r.Fetching {
		t.Errorf("after the write and the late answer: %+v, want 2 and no fetch", r)
	}
}
