// Package notifications is the notification centre: the bell in the top bar
// before the account, and the panel it opens. The panel has what is going on
// now at its top (each module's Activity: the library sync, an update, the
// Widevine download, an add-on waiting for approval), then the entries Go
// keeps (NotificationsService: a release that came out, new releases from
// favourite artists, an add-on switched off), newest first, and the
// library's sync line at its foot. Opening the panel marks the entries read;
// the ones that were new keep their mark while it stays open. Toasts stay
// what they were: quick feedback on something the person just did.
package notifications

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// keyNotices is the centre's entries, mirrored from "notifications:changed".
var keyNotices = store.Key{"notifications"}

// api is what the centre asks of NotificationsService; tests replace it.
type api interface {
	List(ctx context.Context) (services.Notifications, error)
	MarkRead(ctx context.Context) error
	Dismiss(ctx context.Context, id int64) error
	Clear(ctx context.Context) error
}

// feature is the centre's state (main thread).
type feature struct {
	env *module.Env
	api api
	now func() time.Time

	open bool
	// at is the page the panel opened on: a row's action that opens another
	// (Review, an entry) closes it.
	at string
	// fresh is the entries that were unread when the panel opened: they keep
	// their mark while it is open, though Go has marked them read.
	fresh map[int64]bool
}

// New is the notification centre's module.
func New(env *module.Env) module.Module {
	var a api
	if env.Core != nil {
		a = services.Service[services.NotificationsService](env.Core)
	}
	return newFeature(env, a).module()
}

func newFeature(env *module.Env, a api) *feature {
	return &feature{env: env, api: a, now: time.Now}
}

func (f *feature) module() module.Module {
	return module.Module{
		ID:    "notifications",
		Slots: module.Slots{TopBarNotices: f.bell},
		Init: func() {
			store.Mirror[services.Notifications](f.env.Bus, f.env.Queries, f.env.Post, keyNotices, nil, services.EventNotificationsChanged)
		},
	}
}

// entries is the centre's list (no data until Go answers).
func (f *feature) entries() store.Result[services.Notifications] {
	return store.Use(f.env.Queries, keyNotices, func(ctx context.Context) (services.Notifications, error) {
		if f.api == nil {
			return services.Notifications{}, nil
		}
		return f.api.List(ctx)
	}, store.Options{Stale: store.Forever})
}

// bellState is what the bell shows, from the entries and every Activity.
type bellState struct {
	unread int
	// busy is the first Activity with work under way.
	busy        *module.ActivityStatus
	news, alert bool
}

func readBell(list services.Notifications, statuses []module.ActivityStatus) bellState {
	b := bellState{unread: list.Unread, news: list.Unread > 0}
	for i := range statuses {
		st := &statuses[i]
		if st.Busy && b.busy == nil {
			b.busy = st
		}
		b.news = b.news || st.News
		b.alert = b.alert || st.Alert
	}
	return b
}

// name is the bell's accessible name: "Notifications", how many are new,
// and the work under way.
func (b bellState) name() string {
	name := i18n.T("Notifications")
	if b.unread > 0 {
		// translators: The notification bell's name with how many entries are unread.
		name = i18n.T("Notifications, {n} new", "n", i18n.FormatNumber(float64(b.unread)))
	}
	if b.busy != nil && b.busy.Label != "" {
		// translators: The notification bell's name while something runs: "Notifications, 2 new" then "Syncing songs, 812 of 2,040".
		name = i18n.T("{notifications}; {activity}", "notifications", name, "activity", b.busy.Label)
	}
	return name
}

// bell is Slots.TopBarNotices: a 32px ghost button with the bell glyph. A
// dot at its top end says there is something to see: --ink for news
// (unread entries, an update), --danger when something failed. Work under
// way draws a short bar at its foot that moves only when the work reports
// progress (nothing loops). The panel hangs from it, end-aligned.
func (f *feature) bell(c *ui.Context, acts []module.Activity) {
	list := f.entries().Data
	statuses := make([]module.ActivityStatus, len(acts))
	for i, a := range acts {
		if a.Status != nil {
			statuses[i] = a.Status(c)
		}
	}
	b := readBell(list, statuses)
	if f.open && f.location() != f.at {
		f.open = false
	}
	tk := theme.Current()

	btn := ui.ButtonBase(c.Key("notices:bell")).Label(b.name()).HasPopup(ui.PopupDialog).Expanded(f.open).
		Size(32, 32).Radius(tk.Radius["md"]).Center()
	fg := theme.C("ink-muted")
	switch {
	case f.open:
		btn.Background(theme.C("active"))
		fg = theme.C("ink")
	case btn.Hovered():
		btn.Background(theme.C("hover"))
		fg = theme.C("ink")
	}
	theme.Fade(btn, "feedback")
	btn.Children(func() {
		theme.Glyph(c.Key("notices:glyph"), "bell", 16, theme.GlyphOpts{Color: fg})
		if b.alert || b.news {
			dot := theme.C("ink")
			if b.alert {
				dot = theme.C("danger")
			}
			ui.Box(c.Key("notices:dot")).Absolute().Top(5).Right(5).Size(8, 8).Radius(tk.Radius["full"]).
				Background(dot).Border(2, theme.C("surface"))
		}
		if b.busy != nil {
			// The work's bar: 14 wide under the glyph, on a faint track.
			p := b.busy.Progress
			track := ui.Box(c.Key("notices:track")).Absolute().Bottom(5).Left(9).Size(14, 2).Radius(1).
				Background(theme.C("ink-faint").Alpha(0.4)).Clip()
			track.Children(func() {
				if p >= 0 {
					ui.Box(c.Key("notices:fill")).Height(2).Width(14 * float32(min(1, p))).Background(fg)
				}
			})
		}
	})
	if btn.Clicked() {
		if f.open {
			f.open = false
		} else {
			f.show(list)
		}
	}
	if !f.open {
		kit.Tooltip(c, btn, i18n.T("Notifications"))
	}
	kit.Popover(c, btn, &f.open, kit.PopoverOpts{Width: 360, Padding: 8, End: true}, func() {
		f.panel(c, acts, statuses, list)
	})
}

// show opens the panel and marks the entries read, keeping which were new.
func (f *feature) show(list services.Notifications) {
	f.open, f.at = true, f.location()
	f.fresh = map[int64]bool{}
	for _, n := range list.Items {
		if !n.Read {
			f.fresh[n.ID] = true
		}
	}
	if list.Unread > 0 && f.api != nil {
		go func() {
			if err := f.api.MarkRead(context.Background()); err != nil {
				kit.ToastFailure(i18n.T("Couldn't mark notifications read"), err)
			}
		}()
	}
}

// location is the page shown now ("" without a router).
func (f *feature) location() string {
	if f.env.Nav == nil {
		return ""
	}
	return f.env.Nav.Location()
}

// dismiss drops one entry.
func (f *feature) dismiss(id int64) {
	if f.api == nil {
		return
	}
	go func() {
		if err := f.api.Dismiss(context.Background(), id); err != nil {
			kit.ToastFailure(i18n.T("Couldn't remove the notification"), err)
		}
	}()
}

// clear drops every entry.
func (f *feature) clear() {
	if f.api == nil {
		return
	}
	go func() {
		if err := f.api.Clear(context.Background()); err != nil {
			kit.ToastFailure(i18n.T("Couldn't clear notifications"), err)
		}
	}()
}

// follow opens an entry's page and closes the panel. The shell opens it as
// it opens a link ("app:open").
func (f *feature) follow(t services.OpenTarget) {
	f.open = false
	if t.Kind == "" {
		return
	}
	f.env.Bus.Emit(services.EventAppOpen, t)
}
