// Package reminders is release reminders (Go's RemindersService): an upcoming
// album's "Remind me" in its menus, for one desktop notification when it comes
// out. The reminders mirror "reminders:changed" (store.UseReminders); the
// button on an album's page is kit.RemindButton, over the same pieces
// (kit.SetReminder), so the page ports without importing this feature.
package reminders

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// reminders is the feature's state.
type reminders struct {
	env *module.Env
	now func() time.Time
	// list is RemindersService.List, available is kit.RemindAvailable (the
	// platform has notifications, the album a date and a catalogue ID) and set
	// is kit.SetReminder; tests replace them.
	list      func(ctx context.Context) ([]services.Reminder, error)
	available func(services.Album) bool
	set       func(album services.Album, on bool)
}

// New is the release reminders module.
func New(env *module.Env) module.Module {
	r := &reminders{env: env, now: time.Now}
	r.available = func(a services.Album) bool { return kit.RemindAvailable(env.Core, a) }
	r.set = func(a services.Album, on bool) { kit.SetReminder(env.Core, a, on) }
	if env.Core != nil {
		r.list = services.Service[services.RemindersService](env.Core).List
	}
	return module.Module{
		ID:      "reminders",
		Actions: []kit.ActionDef{r.toggleAction()},
		Init: func() {
			store.On(env.Bus, services.EventRemindersChanged, func(list []services.Reminder) {
				store.Set(env.Queries, store.KeyReminders, list)
			})
		},
	}
}

// loaded is the reminders, loaded when an album's menu opens ([] on failure).
func (r *reminders) loaded() kit.MenuData {
	return kit.MenuData{
		ID: "reminders.list",
		Load: func(ctx context.Context, _ []kit.Item) any {
			if r.list != nil {
				if list, err := r.list(ctx); err == nil && list != nil {
					return list
				}
			}
			return []services.Reminder{}
		},
	}
}

// upcoming reports whether an album is still to come at now: Go marks it
// (Prerelease) when it makes the DTO, and ReleaseAt, local midnight on the
// release date (0 without a full date), lets an open page see the release
// happen (shared/lib/release.ts's isUpcoming).
func upcoming(a services.Album, now time.Time) bool {
	return a.Prerelease && (a.ReleaseAt == 0 || now.UnixMilli() < a.ReleaseAt)
}

// toggleAction is the album menu's "Remind me" / "Turn off reminder" (id
// reminders.toggle, group item, order 25): listed for upcoming albums with a
// release date, a catalogue ID and notifications, greyed out until the list
// has loaded.
func (r *reminders) toggleAction() kit.ActionDef {
	data := r.loaded()
	reminded := func(a *kit.ActionContext) bool {
		list, _ := kit.DataOf[[]services.Reminder](a, data)
		_, ok := store.ReminderFor(list, a.Item().Album().CatalogID)
		return ok
	}
	return kit.ActionDef{
		ID: "reminders.toggle", Target: kit.KindAlbum, Icon: "bell", Group: kit.GroupItem, Order: 25,
		Data: []kit.MenuData{data},
		Label: func(a *kit.ActionContext) string {
			if reminded(a) {
				return i18n.T("Turn off reminder")
			}
			return i18n.T("Remind me")
		},
		Available: func(a *kit.ActionContext) bool {
			album := a.Item().Album()
			return upcoming(album, r.now()) && r.available(album)
		},
		Enabled: func(a *kit.ActionContext) bool {
			_, ok := kit.DataOf[[]services.Reminder](a, data)
			return ok
		},
		Run: func(a *kit.ActionContext) { r.set(a.Item().Album(), !reminded(a)) },
	}
}
