package newreleases

import (
	"context"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry (DIPs), as the other pages' (concerts, library).
const (
	padTop    = 28
	padBottom = 48
	// recentDays is how long a release counts as this week's.
	recentDays = 7
)

// group is a run of the list under one heading: releases still to come, those
// out in the last week, and the rest.
type group struct {
	id    string
	title string
	items []services.NewRelease
}

// groupsOf sorts a list (newest first, as Go sends it) into its groups, the
// ones not out yet soonest first; a group with nothing in it is left out.
func groupsOf(items []services.NewRelease, now time.Time) []group {
	soon := group{id: "soon", title: i18n.T("Coming soon")}
	week := group{id: "week", title: i18n.T("This week")}
	earlier := group{id: "earlier", title: i18n.T("Earlier")}
	for _, r := range items {
		at := r.Album.ReleaseAt
		if at == 0 {
			at = r.FoundAt // a year alone: when Sona found it
		}
		switch {
		case r.Album.Prerelease:
			soon.items = append(soon.items, r)
		case now.Sub(time.UnixMilli(at)) < recentDays*24*time.Hour:
			week.items = append(week.items, r)
		default:
			earlier.items = append(earlier.items, r)
		}
	}
	slices.Reverse(soon.items)
	var out []group
	for _, g := range []group{soon, week, earlier} {
		if len(g.items) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// releasesPage is the page: the header (title, what it lists and when it was
// last checked, Mark all as read and Clear), then the releases under their
// headings, or the state that says why there are none.
func (n *newReleases) releasesPage(c *ui.Context, _ *ui.Route) {
	list := store.UseNewReleases(n.env.Queries, n.env.Core)
	st := store.UseSettings(n.env.Queries, n.env.Core)
	on := st.HasData && st.Data.ArtistTracker.Enabled
	now := n.now()

	tk := theme.Current()
	gutter := kit.PageGutter(n.page.width)
	devstate.Scroll(c, &n.page.pos)
	sc := ui.Scroll(c).TrackScroll(&n.page.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+tk.Metric["scrollbar"], padBottom, gutter).Children(func() {
			n.header(c, list.Data, list.HasData, on, now)
			switch {
			case list.Err != nil && !list.HasData:
				kit.ErrorBlock(c, i18n.T("Couldn't load the new releases"), list.Err, kit.ErrorBlockOpts{
					OnRetry:  func() { n.env.Queries.Invalidate(store.KeyNewReleases) },
					Retrying: list.Fetching,
				})
			case !list.HasData || !st.HasData:
				skeleton(c)
			case len(list.Data.Items) == 0 && !on:
				kit.EmptyState(c, i18n.T("Release tracking is off"),
					i18n.T("Turn on “Track new releases from favourite artists” in Settings, and the new albums, EPs and singles of the artists you favourite will be listed here and on For You."),
					kit.EmptyOpts{Action: &kit.EmptyAction{Label: i18n.T("Open Settings"), Run: func() { n.env.Nav.Push(settingsPath) }}})
			case len(list.Data.Items) == 0:
				kit.EmptyState(c, i18n.T("Nothing new yet"),
					i18n.T("Sona looks about once a day. When an artist you've favourited puts something out, it shows up here and on For You."),
					kit.EmptyOpts{})
			default:
				n.releases(c, &n.list, groupsOf(list.Data.Items, now), now, true)
			}
		})
	})
	n.page.width = sc.Bounds().W
}

// header is the title with its two actions beside it, and under them what
// the list holds and how old the last check is.
func (n *newReleases) header(c *ui.Context, r services.NewReleases, known, on bool, now time.Time) {
	subline := i18n.T("From the artists you've favourited")
	switch {
	case !known:
		subline = ""
	case !on && len(r.Items) == 0:
		subline = "" // the empty state says so
	case !on:
		subline = i18n.T("Release tracking is off")
	case r.CheckedAt > 0:
		subline += " · " + i18n.T("Checked {when}", "when", i18n.RelativeTime(time.UnixMilli(r.CheckedAt), now))
	default:
		subline += " · " + i18n.T("Not checked yet")
	}
	ui.Column(c.Key("header")).Margin(0, 0, 24, 0).Shrink(0).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(16).Children(func() {
			theme.Text(c, shelfTitle(), "display").TextColor(theme.C("ink")).SingleLine().MinWidth(0).Shrink(1).
				Role(ui.RoleHeading).Level(1)
			if len(r.Items) == 0 {
				return
			}
			if kit.Button(c.Key("read-all"), i18n.T("Mark all as read"), kit.ButtonOpts{
				Variant: kit.Outline, Size: kit.SizeSM, Icon: "check", Disabled: r.Unread == 0,
			}).Clicked() {
				n.do(func(s api) error { return s.MarkAllRead(context.Background()) })
			}
			if kit.Button(c.Key("clear"), i18n.T("Clear list"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "trash"}).Clicked() {
				n.do(func(s api) error { return s.Clear(context.Background()) })
			}
		})
		if subline != "" {
			theme.Text(c, subline, "secondary").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
		}
	})
}

// skeleton is the list's frame while it loads: a heading and three rows at
// their height, flat bars.
func skeleton(c *ui.Context) ui.Element {
	e := ui.Column(c.Key("skeleton")).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	e.Children(func() {
		kit.Skeleton(c.Key("heading"), 120, 20).Margin(0, 0, 12, 0)
		for i := range 3 {
			ui.Row(c.Key(i)).Height(rowH).PaddingX(rowPadX).Gap(rowGap).AlignItems(ui.Center).Children(func() {
				ui.Box(c.Key("dot")).Size(dotSlot, 1)
				kit.Skeleton(c.Key("art"), cover, cover)
				ui.Column(c.Key("text")).Grow(1).Gap(8).Children(func() {
					kit.Skeleton(c.Key("a"), 220, 16)
					kit.Skeleton(c.Key("b"), 140, 12)
				})
			})
		}
	})
	return e
}
