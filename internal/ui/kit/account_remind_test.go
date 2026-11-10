package kit

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestRemindButtonStatesAndClicks(t *testing.T) {
	album := services.Album{
		CatalogID: "1700", Name: "Out Soon", Artist: "An Artist", Prerelease: true,
		ReleaseAt: time.Date(2026, time.November, 19, 0, 0, 0, 0, time.Local).UnixMilli(),
	}
	var res store.Result[[]services.Reminder]
	var sets []bool
	tt := controlsTester(t, 400, 200, func(c *ui.Context) {
		remindButton(c, res, album, false, func(on bool) { sets = append(sets, on) })
	})
	tip := func(name, want string) {
		t.Helper()
		tt.Move(390, 190)
		tt.Advance(time.Second)
		r := controlsFind(t, tt, name)
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Advance(time.Second)
		if !tt.HasText(want) {
			t.Errorf("no tip %q: %q", want, tt.Texts())
		}
		tt.Move(390, 190)
		tt.Advance(time.Second)
	}

	// Loading: disabled, clicks do nothing.
	_ = tt.Click("Remind me")
	if len(sets) != 0 {
		t.Errorf("a click while the reminders load set %v", sets)
	}

	// Loaded, no reminder: "Get a notification when it's out", a click sets one.
	res = store.Result[[]services.Reminder]{HasData: true, Data: []services.Reminder{{AlbumID: "other"}}}
	tt.Frame()
	tip("Remind me", "Get a notification when it's out")
	if err := tt.Click("Remind me"); err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || !sets[0] {
		t.Fatalf("sets = %v, want [true]", sets)
	}

	// A reminder is set: pressed, the tip names the day, a click removes it.
	res.Data = []services.Reminder{{AlbumID: "1700"}}
	tt.Frame()
	tip("Remind me", "You'll get a notification on Thursday, November 19, 2026")
	_ = tt.Click("Remind me")
	if len(sets) != 2 || sets[1] {
		t.Fatalf("sets = %v, want [true false]", sets)
	}

	// A failed load is not "loading": the button works (and finds no reminder).
	res = store.Result[[]services.Reminder]{Err: errors.New("down")}
	tt.Frame()
	_ = tt.Click("Remind me")
	if len(sets) != 3 || !sets[2] {
		t.Errorf("sets = %v after a failed load", sets)
	}
}

func TestRemindAvailableNeedsCoreCatalogIDAndDate(t *testing.T) {
	if RemindAvailable(nil, services.Album{CatalogID: "1", ReleaseAt: 1}) {
		t.Error("available with no services")
	}
}

func TestRemindButtonLeadsUntilARemindersSet(t *testing.T) {
	album := services.Album{
		CatalogID: "1700", Name: "Out Soon", Prerelease: true,
		ReleaseAt: time.Date(2026, time.November, 19, 0, 0, 0, 0, time.Local).UnixMilli(),
	}
	res := store.Result[[]services.Reminder]{HasData: true}
	tt := controlsTester(t, 400, 100, func(c *ui.Context) { remindButton(c, res, album, true, func(bool) {}) })
	tt.Advance(time.Second)
	if got, want := face(tt, "Remind me"), rgb(theme.C("ink")); got != want {
		t.Errorf("a lead Remind me is %v, want the primary ink fill %v", got, want)
	}
	res.Data = []services.Reminder{{AlbumID: "1700"}}
	tt.Frame()
	tt.Advance(time.Second)
	if got, want := face(tt, "Remind me"), rgb(theme.C("ink")); got == want {
		t.Errorf("a reminder that is set still leads: %v", got)
	}
}
