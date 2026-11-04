package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// RemindAvailable reports whether an album can have a release reminder: the
// platform has notifications, and the album has a catalogue ID and a full
// release date (shared/lib/release.ts and reminders/index.ts's isAvailable;
// the caller decides the album is upcoming).
func RemindAvailable(core *services.Core, album services.Album) bool {
	if core == nil || album.CatalogID == "" || album.ReleaseAt == 0 {
		return false
	}
	return services.Service[services.PlatformService](core).Capabilities().Notifications
}

// RemindButton is "Remind me" on an upcoming album's page: an outline toggle
// (its name stays the same; pressed means a reminder is set, with the ringing
// bell) for one desktop notification when the album comes out. It builds
// nothing where the platform has no notifications and for an album without a
// full release date or a catalogue ID, and is disabled until the reminders
// have loaded. It sets or removes the reminder itself (SetReminder).
func RemindButton(c *ui.Context, cl *store.Client, core *services.Core, album services.Album) {
	if !RemindAvailable(core, album) {
		return
	}
	r := store.UseReminders(cl, core)
	_, on := store.ReminderFor(r.Data, album.CatalogID)
	icon := "bell"
	tip := i18n.T("Get a notification when it's out")
	if on {
		icon = "bell-ring"
		tip = i18n.T("You'll get a notification on {releaseDate}", "releaseDate", i18n.FormatDate(time.UnixMilli(album.ReleaseAt), i18n.DateFull))
	}
	pending := !r.HasData && r.Err == nil
	if Button(c.Key("remind"), i18n.T("Remind me"), ButtonOpts{
		Variant: Outline, Size: SizeSM, Icon: icon, Pressed: &on, Disabled: pending, Tip: tip,
	}).Clicked() {
		SetReminder(core, album, !on)
	}
}
