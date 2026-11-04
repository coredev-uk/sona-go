package kit

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// The account-side actions several features share (the player bar and the
// account menu switch private listening, Settings and the player bar retry
// the Widevine download, an album page and its menu set a reminder). Each
// runs its service call off the main thread and answers a failure with a
// toast titled by what was attempted and worded by Go's error code
// (v0.2.0's run(title, promise)). The shared mirrors they write are in
// store (UsePrivate, UseReminders…).

// SetPrivate turns private listening on or off (PrivateService.Set). Go's
// "private:changed" updates the mirror; the answer is also written to it, as
// v0.2.0 did, so the switch follows at once.
func SetPrivate(cl *store.Client, core *services.Core, on bool) {
	setPrivate(cl, services.Service[services.PrivateService](core).Set, on)
}

// setPrivate is SetPrivate over the call it makes (tests replace it).
func setPrivate(cl *store.Client, set func(on bool) (services.PrivateState, error), on bool) {
	go func() {
		st, err := set(on)
		if err != nil {
			title := i18n.T("Couldn't turn off private listening")
			if on {
				title = i18n.T("Couldn't turn on private listening")
			}
			ToastErrorf(title, i18n.ErrorText(services.CodeOf(err)))
			return
		}
		store.Set(cl, store.KeyPrivate, st)
	}()
}

// RetryCDM asks Go to download the Widevine CDM, or check for an update, now
// (CDMService.Retry); "cdm:status" follows. A failure toasts "Couldn't start
// the Widevine download".
func RetryCDM(core *services.Core) { RetryCDMWith(services.Service[services.CDMService](core).Retry, nil) }

// RetryCDMWith is RetryCDM over retry (CDMService.Retry, or a test's). done,
// when not nil, runs once retry returned and a failure has been toasted, on
// the call's goroutine (post it to the UI's own for state): a button that
// reads "Retrying…" while the call runs ends it there.
func RetryCDMWith(retry func() error, done func()) {
	go func() {
		if err := retry(); err != nil {
			ToastErrorf(i18n.T("Couldn't start the Widevine download"), i18n.ErrorText(services.CodeOf(err)))
		}
		if done != nil {
			done()
		}
	}()
}

// SetReminder turns the release reminder of an upcoming catalogue album on or
// off (RemindersService.Set/Remove). The notification's words are the
// interface's (Go has none of its own): the album's name as its title, "Out
// now · {artist}" as its body. Go's "reminders:changed" updates the mirror.
func SetReminder(core *services.Core, album services.Album, on bool) {
	setReminder(services.Service[services.RemindersService](core), album, on)
}

// reminderAPI is what SetReminder asks of RemindersService.
type reminderAPI interface {
	Set(ctx context.Context, id, message string) (services.Reminder, error)
	Remove(ctx context.Context, id string) error
}

func setReminder(svc reminderAPI, album services.Album, on bool) {
	go func() {
		ctx := context.Background()
		name := album.Name
		if !on {
			if err := svc.Remove(ctx, album.CatalogID); err != nil {
				ToastErrorf(i18n.T("Couldn't remove the reminder"), i18n.ErrorText(services.CodeOf(err)))
				return
			}
			ShowToast(ToastSpec{Title: i18n.T("You won't be reminded about “{name}”", "name", name)})
			return
		}
		if _, err := svc.Set(ctx, album.CatalogID, i18n.T("Out now · {artistName}", "artistName", album.Artist)); err != nil {
			ToastErrorf(i18n.T("Couldn't set the reminder"), i18n.ErrorText(services.CodeOf(err)))
			return
		}
		ShowToast(ToastSpec{Title: i18n.T("You'll be reminded when “{name}” is out", "name", name)})
	}()
}
