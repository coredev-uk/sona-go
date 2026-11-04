package kit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// toasted runs fn, which raises toasts off the main thread, and returns the
// texts the Toaster shows.
func toasted(t *testing.T, fn func(), want ...string) *ui.Tester {
	t.Helper()
	tt := controlsTester(t, 600, 300, func(c *ui.Context) { Toaster(c) })
	CloseToast("")
	fn()
	// The toast is raised by a goroutine after its service call: poll, in
	// real time, with a deadline.
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		tt.Frame()
		ok := true
		for _, w := range want {
			ok = ok && tt.HasText(w)
		}
		if ok {
			return tt
		}
		tt.Advance(50 * time.Millisecond)
	}
	t.Fatalf("no toast with %q: %q", want, tt.Texts())
	return tt
}

func TestSetPrivateWritesTheAnswerAndToastsAFailure(t *testing.T) {
	cl := store.NewClient(nil)
	setPrivate(cl, func(on bool) (services.PrivateState, error) {
		return services.PrivateState{On: on, Source: "setting", Saved: on}, nil
	}, true)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if s, ok := store.Peek[services.PrivateState](cl, store.KeyPrivate); ok {
			if !s.On {
				t.Fatalf("mirror = %+v", s)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the answer was never written to the mirror")
		}
	}
	for _, c := range []struct {
		on    bool
		title string
	}{{true, "Couldn't turn on private listening"}, {false, "Couldn't turn off private listening"}} {
		toasted(t, func() {
			setPrivate(cl, func(bool) (services.PrivateState, error) {
				return services.PrivateState{}, privatemode.ErrForced
			}, c.on)
		}, c.title)
	}
}

func TestRetryCDMToastsAFailure(t *testing.T) {
	toasted(t, func() { RetryCDMWith(func() error { return errors.New("x") }, nil) }, "Couldn't start the Widevine download")
}

type fakeReminders struct {
	mu   sync.Mutex
	sets []string
	fail error
}

func (f *fakeReminders) Set(_ context.Context, id, msg string) (services.Reminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, id+"|"+msg)
	return services.Reminder{}, f.fail
}

func (f *fakeReminders) Remove(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, "remove "+id)
	return f.fail
}

func TestSetReminderSendsTheNotificationWordsAndToasts(t *testing.T) {
	album := services.Album{CatalogID: "1700", Name: "Out Soon", Artist: "An Artist"}
	f := &fakeReminders{}
	toasted(t, func() { setReminder(f, album, true) }, "You'll be reminded when “Out Soon” is out")
	f.mu.Lock()
	if len(f.sets) != 1 || f.sets[0] != "1700|Out now · An Artist" {
		t.Errorf("Set got %v", f.sets)
	}
	f.mu.Unlock()
	toasted(t, func() { setReminder(f, album, false) }, "You won't be reminded about “Out Soon”")
	f.fail = errors.New("db")
	toasted(t, func() { setReminder(f, album, true) }, "Couldn't set the reminder")
	toasted(t, func() { setReminder(f, album, false) }, "Couldn't remove the reminder")
}

func TestRemindableNeedsNotificationsACatalogIDAndADate(t *testing.T) {
	ok := services.Album{CatalogID: "1", ReleaseAt: 1}
	for _, c := range []struct {
		name string
		n    bool
		a    services.Album
		want bool
	}{
		{"all there", true, ok, true},
		{"no notifications", false, ok, false},
		{"no catalogue ID", true, services.Album{ReleaseAt: 1}, false},
		{"no full release date", true, services.Album{CatalogID: "1"}, false},
	} {
		if got := remindable(c.n, c.a); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
