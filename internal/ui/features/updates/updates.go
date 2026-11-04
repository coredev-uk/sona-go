// Package updates is the update notice: the quiet line at the foot of the
// sidebar, shown only while a release newer than this build exists, with its
// version and a link to the release page. Go checks GitHub after the first
// paint (unless turned off in Settings → General) and sends
// "updates:available"; Settings → General shows the same status and checks on
// request (store.UseUpdates). Updating is the platform's: a package manager,
// an AppImage's own updater, or the release page.
package updates

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// updates is the feature's state.
type updates struct {
	env *module.Env
	// openRelease opens the release page (UpdatesService.OpenRelease); tests
	// replace it.
	openRelease func() error
}

// New is the update notice's module.
func New(env *module.Env) module.Module {
	var open func() error
	if env.Core != nil {
		open = services.Service[services.UpdatesService](env.Core).OpenRelease
	}
	return newUpdates(env, open).module()
}

func newUpdates(env *module.Env, open func() error) *updates {
	return &updates{env: env, openRelease: open}
}

func (u *updates) module() module.Module {
	return module.Module{
		ID:    "updates",
		Slots: module.Slots{SidebarFooter: u.notice},
		Init: func() {
			store.On(u.env.Bus, services.EventUpdateAvailable, func(s services.UpdateStatus) {
				store.Set(u.env.Queries, store.KeyUpdates, s)
			})
		},
	}
}

// notice builds nothing unless a newer release exists.
func (u *updates) notice(c *ui.Context) {
	r := store.UseUpdates(u.env.Queries, u.env.Core)
	if !r.HasData || !r.Data.Available {
		return
	}
	latest := r.Data.Latest
	meta := theme.Current().Type["meta"]
	ui.Row(c.Key("update-notice")).FillWidth().Gap(8).PaddingX(10).Margin(0, 0, 8, 0).AlignItems(ui.Center).Justify(ui.SpaceBetween).
		Role(ui.RoleStatus).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
			ui.Icon(c, theme.Icon("circle-arrow-up")).FontSize(12).TextColor(theme.C("ink")).Shrink(0)
			ui.Text(c, i18n.T("Sona {latest} is available", "latest", latest)).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).
				TextColor(theme.C("ink")).SingleLine().MinWidth(0).Shrink(1)
		})
		view := ui.ButtonBase(c.Key("update-view")).Radius(theme.Current().Radius["sm"]).Shrink(0).Label(i18n.T("View"))
		fg := theme.C("ink-muted")
		if view.Hovered() {
			fg = theme.C("ink")
		}
		view.TextColor(fg).Children(func() {
			t := ui.Text(c, i18n.T("View")).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).SingleLine()
			if view.Hovered() {
				t.Underline()
			}
		})
		if view.Clicked() {
			u.view()
		}
	})
}

// view opens the release page; a failure toasts "Couldn't open the release".
func (u *updates) view() {
	open := u.openRelease
	if open == nil {
		return
	}
	go func() {
		if err := open(); err != nil {
			kit.ToastErrorf(i18n.T("Couldn't open the release"), i18n.ErrorText(services.CodeOf(err)))
		}
	}()
}
