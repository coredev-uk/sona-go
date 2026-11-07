package settings

import (
	"context"
	"errors"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// keyLinkHandler is the query of the app that opens Apple Music links.
var keyLinkHandler = store.Key{"link-handler"}

// linkState is the answer of DesktopService.LinkHandler: who opens the links
// now, or that this build can't be chosen to (not installed as an app).
type linkState struct {
	services.LinkHandlerStatus
	Unsupported bool
}

// linkHandlerState outlives a frame: the change in flight.
type linkHandlerState struct{ setting bool }

func (s *settings) linkHandler() store.Result[linkState] {
	api := s.desktop()
	return store.Use(s.env.Queries, keyLinkHandler, func(context.Context) (linkState, error) {
		st, err := api.LinkHandler()
		if errors.Is(err, errors.ErrUnsupported) {
			return linkState{Unsupported: true}, nil
		}
		return linkState{LinkHandlerStatus: st}, err
	})
}

// notInstalled is why an installed build is needed, per platform.
func notInstalled(os string) string {
	if os == "darwin" {
		return i18n.T("Only the installed Sona.app can be chosen for them, not a source build or a development bundle.")
	}
	return i18n.T("Sona's desktop entry isn't installed (a source build, a Flatpak or an AppImage that isn't integrated), so your desktop can't choose Sona for them.")
}

// linkHandlerRow is Apple Music links: whether music:// and itms:// links
// (the Open in Music buttons of web pages) open in Sona, and a button that
// makes Sona the app that opens them. macOS gives Music.app those schemes
// until the user chooses another app, and https://music.apple.com links stay
// with the browser.
func (s *settings) linkHandlerRow(c *ui.Context, caps services.Capabilities) {
	title := i18n.T("Open Apple Music links in Sona")
	if !caps.LinkHandler {
		// Windows has no row: its installer registers the links for the user.
		if caps.NeedsAppBundle {
			s.unavailableRow(c, "link-handler", title, needsAppBundle())
		}
		return
	}
	res := s.linkHandler()
	if res.HasData && res.Data.Unsupported {
		s.unavailableRow(c, "link-handler", title, notInstalled(caps.OS))
		return
	}
	ls := stateOf[linkHandlerState](s, "link-handler")
	desc := i18n.T("Links that start with music:// or itms://, such as the Open in Music button on a web page, open in the app you choose. Links that start with https:// stay with your browser.")
	sona := res.HasData && res.Data.Sona
	o := rowOpts{ID: "link-handler", Title: title, Desc: desc}
	switch {
	case res.HasData && !sona && res.Data.Other != "":
		other := res.Data.Other
		o.Notes = func() {
			text(c, i18n.T("They open in {app} now.", "app", other), "secondary").TextColor(theme.C("ink")).Margin(2, 0, 0, 0)
		}
	case res.HasData && !sona:
		o.Notes = func() {
			text(c, i18n.T("Another app opens them now."), "secondary").TextColor(theme.C("ink")).Margin(2, 0, 0, 0)
		}
	case !res.HasData && res.Err != nil:
		reason := i18n.ErrorMessage(res.Err)
		o.Notes = func() {
			text(c, i18n.T("Couldn't tell which app opens them: {reason}", "reason", reason), "secondary").TextColor(theme.C("ink")).Margin(2, 0, 0, 0)
		}
	}
	s.row(c, o, func() ui.Element {
		if sona {
			ui.Row(c.Key("sona")).Gap(6).AlignItems(ui.Center).Role(ui.RoleStatus).Label(title).Children(func() {
				ui.Icon(c, theme.Icon("circle-check")).FontSize(16).TextColor(theme.C("positive"))
				text(c, i18n.T("Sona opens them"), "secondary").TextColor(theme.C("ink"))
			})
			return ui.Element{}
		}
		label := i18n.T("Make Sona the default")
		if ls.setting {
			label = i18n.T("Setting…")
		}
		b := kit.Button(c.Key("set"), label, kit.ButtonOpts{
			Variant: kit.Outline, Size: kit.SizeSM, Spin: ls.setting, Disabled: ls.setting || (!res.HasData && res.Err == nil),
		})
		if b.Clicked() {
			s.makeLinkHandler(ls, res.Data.Other)
		}
		return b
	})
}

// makeLinkHandler asks the system to open Apple Music links with Sona. The
// system may ask the user first or keep its choice, so the answer decides
// what the row shows, and a choice that didn't take says so.
func (s *settings) makeLinkHandler(ls *linkHandlerState, other string) {
	ls.setting = true
	api := s.desktop()
	go func() {
		st, err := api.SetLinkHandler(context.Background())
		s.env.Post(func() {
			ls.setting = false
			switch {
			case err != nil:
				kit.ToastErrorf(i18n.T("Couldn't change the app for Apple Music links"), i18n.ErrorMessage(err))
				s.env.Queries.Invalidate(keyLinkHandler)
				return
			case !st.Sona:
				name := st.Other
				if name == "" {
					name = other
				}
				kit.ToastErrorf(i18n.T("Sona isn't the default yet"), keptMessage(name))
			}
			store.Set(s.env.Queries, keyLinkHandler, linkState{LinkHandlerStatus: st})
		})
	}()
}

// keptMessage says which app the system kept for the links.
func keptMessage(name string) string {
	if name == "" {
		return i18n.T("Your system kept its choice.")
	}
	return i18n.T("Your system kept {app}.", "app", name)
}
