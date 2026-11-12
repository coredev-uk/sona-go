package search

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// profileView is the profile page's state.
type profileView struct {
	pageView
	grid  kit.ItemGridState
	menus *kit.ActionMenus
}

// profilePage is v0.2.0's ProfilePage: an Apple Music listener (found
// through search): who they are and their public playlists.
func (s *search) profilePage(c *ui.Context, r *ui.Route) {
	id := r.Param("id")
	v := &s.pages.profile
	res := s.profile(id)
	s.scrollPage(c, &v.pageView, id, func(pane *kit.Pane) {
		switch {
		case res.Err != nil:
			kit.ErrorBlock(c, i18n.T("Couldn't load this profile"), res.Err, kit.ErrorBlockOpts{
				OnRetry:  func() { s.env.Queries.Invalidate(keyProfile(id)) },
				Retrying: res.Fetching,
			})
		case !res.HasData:
			ui.Row(c.Key("loading")).AlignItems(ui.Center).Gap(24).Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
				ui.Box(c).Size(128, 128).Shrink(0).Radius(64).Background(theme.C("hover"))
				ui.Column(c).Gap(8).Children(func() {
					kit.Skeleton(c, 224, 32)
					kit.Skeleton(c, 112, 16)
				})
			})
		default:
			s.profileBody(c, pane, res.Data)
		}
	})
}

// profileBody is the loaded profile: the header, then the playlists.
func (s *search) profileBody(c *ui.Context, pane *kit.Pane, d services.ProfileDetail) {
	v := &s.pages.profile
	p := d.Profile
	name := p.Name
	ui.Row(c.Key("header")).FillWidth().AlignItems(ui.Center).Gap(24).Margin(0, 0, 40, 0).Children(func() {
		kit.PersonAvatar(c, s.env.Art, name, p.Art.Large, 128).Shrink(0)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				theme.Text(c, name, "display").TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0).Role(ui.RoleHeading).Level(1)
				if p.Verified {
					ui.Icon(c, theme.Icon("badge-check")).FontSize(24).TextColor(theme.C("ink-muted")).Shrink(0).
						Role(ui.RoleImage).Label(i18n.T("Verified"))
				}
			})
			ui.Row(c).AlignItems(ui.Center).Gap(6).Margin(4, 0, 0, 0).Children(func() {
				theme.Text(c, "@"+p.Handle, "secondary").TextColor(theme.C("ink-muted")).SingleLine()
				if p.Private {
					theme.Text(c, "·", "secondary").TextColor(theme.C("ink-muted")).Role(ui.RoleNone)
					ui.Icon(c, theme.Icon("lock")).FontSize(14).TextColor(theme.C("ink-muted")).Role(ui.RoleNone)
					theme.Text(c, i18n.T("Private profile"), "secondary").TextColor(theme.C("ink-muted")).SingleLine()
				}
			})
		})
	})

	if len(d.Playlists) == 0 {
		text := i18n.T("{name} hasn't shared any playlists on their profile.", "name", name)
		if p.Private {
			text = i18n.T("{name} keeps their profile private, so only followers see what they share.", "name", name)
		}
		kit.EmptyState(c, i18n.T("No public playlists"), text, kit.EmptyOpts{})
		return
	}
	items := make([]services.Item, len(d.Playlists))
	for i := range d.Playlists {
		pl := d.Playlists[i]
		items[i] = services.Item{Kind: services.ItemPlaylist, ID: pl.ID, Name: pl.Name, Subtitle: pl.Curator, Art: pl.Art, Playlist: &pl}
	}
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Public playlists"), First: true}, func() {
		ctx := s.env.ItemsCtx(s.itemMenus(&v.menus, nil))
		kit.ItemGrid(c, &v.grid, len(items), func(i int) *services.Item { return &items[i] }, ctx,
			kit.ItemGridOpts{Label: i18n.T("Playlists shared by {name}", "name", name), Pane: pane})
	})
}
