package home

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// homeStale is how long a read of Home is fresh (v0.2.0: staleTime 60 s).
const homeStale = time.Minute

// homePage is HomePage: the header and what the account's sign-in state and
// Home's query allow. Signed out, offline without a copy, or empty: the
// local shelves under a notice.
func (h *home) homePage(c *ui.Context, _ *ui.Route) {
	h.page.Scroll(c, "", func(*kit.Pane, float32) {
		root := ui.Column(c.Key("home"))
		// The refresh mutation belongs to the page's mount, as useMutation's.
		ui.Local(root, "mount", func() struct{} {
			h.refreshErr = nil
			return struct{}{}
		})
		root.Children(func() { h.homeBody(c) })
	})
}

func (h *home) homeBody(c *ui.Context) {
	auth, known := h.authState()
	state := ""
	if known {
		state = auth.State
	}
	ctx := h.env.ItemsCtx(nil) // once per build: the sections share it
	offline := known && auth.Offline
	signedIn := account.HasAccount(state)

	var res store.Result[services.HomePage]
	if signedIn {
		res = store.Use(h.env.Queries, keyHome, h.disc.Home, store.Options{Stale: homeStale})
	}
	var plan homePlan
	subline := ""
	if signedIn && res.HasData {
		plan = planHome(res.Data.Sections)
		if res.Data.FetchedAt > 0 {
			fetched := i18n.RelativeTime(time.UnixMilli(res.Data.FetchedAt), h.now())
			if res.Data.Stale {
				subline = i18n.T("Couldn't refresh · from {fetched}", "fetched", fetched)
			} else {
				subline = i18n.T("Picked for you {fetched}", "fetched", fetched)
			}
		}
	}

	header := func() {
		var actions func()
		// Offline a refresh can only fail; Go renews Home when it reconnects.
		if signedIn && !offline {
			actions = func() { h.refreshButton(c) }
		}
		kit.PageHeader(c, i18n.T("For you"), subline, actions)
	}
	refreshError := func() {
		if h.refreshErr == nil {
			return
		}
		kit.ErrorBlock(c, i18n.T("Couldn't refresh your recommendations"), h.refreshErr, kit.ErrorBlockOpts{OnRetry: h.refresh}).
			Margin(0, 0, 32, 0)
	}

	switch {
	case state == "restoring" || (signedIn && !res.HasData && res.Err == nil):
		// Restoring: Go is still reading the sign-in from the keyring, so
		// not signed out.
		header()
		h.leadSkeleton(c)
	case !signedIn:
		header()
		if known {
			h.signInLead(c)
			h.localShelves(c, false, ctx)
		}
	case res.Err != nil:
		header()
		if offline {
			h.offlineLead(c)
		} else {
			kit.QueryError(c, h.env.Queries, i18n.T("Couldn't load your recommendations"), res, keyHome)
		}
		h.localShelves(c, true, ctx)
	case plan.lead == nil && len(plan.open) == 0 && len(plan.index) == 0:
		header()
		refreshError()
		h.localShelves(c, true, ctx)
	default:
		var lead *services.Item
		if plan.lead != nil {
			lead = &plan.lead.Items[0]
		}
		h.leadBand(c, lead, func() {
			header()
			refreshError()
			if plan.lead != nil {
				h.homeLead(c, *plan.lead, ctx)
			}
		})
		for i := range plan.open {
			h.homeSection(c, plan.open[i], ctx)
		}
		if len(plan.index) > 0 {
			h.shelfIndex(c, plan.index, ctx)
		}
	}
}

// now is the clock the freshness line is read against.
func (h *home) now() time.Time {
	if h.env.Players != nil && h.env.Players.Now != nil {
		return h.env.Players.Now()
	}
	return time.Now()
}

// refresh asks Go for a new Home (RefreshHome); the page it returns replaces
// the cached one.
func (h *home) refresh() {
	if h.refreshing {
		return
	}
	h.refreshing, h.refreshErr = true, nil
	disc := h.disc
	go func() {
		page, err := disc.RefreshHome(context.Background())
		h.env.Post(func() {
			h.refreshing = false
			if err != nil {
				h.refreshErr = err
				return
			}
			store.Set(h.env.Queries, keyHome, page)
		})
	}()
}

// refreshButton is the header's ghost Refresh button: disabled while it
// runs, with the icon spinning (not under reduced motion) and the label
// "Refreshing…".
func (h *home) refreshButton(c *ui.Context) {
	tk := theme.Current()
	label := i18n.T("Refresh")
	if h.refreshing {
		label = i18n.T("Refreshing…")
	}
	b := ui.ButtonBase(c.Key("refresh")).Height(32).PaddingX(10).Gap(6).Center().Radius(tk.Radius["md"]).Label(label)
	bg := ui.Transparent
	if b.Hovered() && !h.refreshing {
		bg = theme.C("accent-soft")
	}
	b.Background(bg).TextColor(theme.C("ink"))
	if h.refreshing {
		b.Disabled(true) // MyGo paints a disabled element at 50%
	}
	theme.Fade(b, "feedback")
	b.Children(func() {
		ic := ui.Icon(c.Key("icon"), theme.Icon("rotate-cw")).FontSize(16)
		theme.Spin(ic, h.refreshing)
		theme.Text(c, label, "label").SingleLine()
	})
	if b.Clicked() && !h.refreshing {
		h.refresh()
		// The button keeps the focus while it is disabled and its name
		// changes: say so once, for what the name change may not carry.
		c.Announce(i18n.T("Refreshing…"))
	}
}
