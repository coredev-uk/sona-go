package search

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// pageSkeleton is how many placeholder rows or tiles stand for a page that
// is loading.
const pageSkeleton = 6

// groupView is the "see all" page's state.
type groupView struct {
	pageView
	songs kit.TrackListState
	grid  kit.ItemGridState
	menus *kit.ActionMenus
	// listMenus is the song list's own menu: its selection is its scope.
	listMenus *kit.ActionMenus
	// want is how many pages the list asks for: more as its end comes
	// within a screen (v0.2.0's fetchNextPage on the IntersectionObserver).
	want int
	// key is the group and term showing: another one starts at one page.
	key string
	// built is how many items the last frame built: the end's place is
	// only known (Bounds is the last frame's) once two frames agree.
	built int
}

// groupTitle is the page's title: the group's name.
func (s *search) groupTitle(r *ui.Route) string { return groupTitle(r.Param("group")) }

// groupPage is v0.2.0's GroupPage: "see all" for one result group, every
// page Apple has, loaded as it scrolls.
func (s *search) groupPage(c *ui.Context, r *ui.Route) {
	group, q := r.Param("group"), cleanTerm(r.Query("q"))
	v := &s.pages.group
	v.wantFocus, v.claimed = false, false
	if key := group + "\x00" + q; v.key != key {
		v.key, v.want = key, 1
	}
	s.scrollPage(c, &v.pageView, v.key, func(pane *kit.Pane) {
		s.groupHeader(c, group, q)

		// The pages loaded so far, in order: each is asked for at the
		// offset the one before named.
		var items []services.Item
		var failed error
		var failedAt, loaded int
		more, loading, offset := false, false, 0
		for range v.want {
			res := s.groupPageAt(q, group, offset)
			if !res.HasData {
				failed, failedAt = res.Err, offset
				loading = res.Err == nil
				break
			}
			loaded++
			items = append(items, res.Data.Items...)
			more = res.Data.Next > 0
			offset = res.Data.Next
			if !more {
				break
			}
		}
		title := groupTitle(group)

		switch {
		case failed != nil && len(items) == 0:
			kit.ErrorBlock(c, groupLoadError(group), failed, kit.ErrorBlockOpts{
				OnRetry:  func() { s.env.Queries.Invalidate(keyGroup(q, group, failedAt)) },
				Retrying: s.groupPageAt(q, group, failedAt).Fetching,
			})
			return
		case loaded > 0 && len(items) == 0 && !more:
			kit.EmptyState(c, groupEmpty(group, q), i18n.T("Apple Music has nothing more of this kind for the search."), kit.EmptyOpts{})
			return
		}

		ctx := s.env.ItemsCtx(s.itemMenus(&v.menus, nil))
		ctx.OnOpen = s.opened(q, scopeCatalog)
		pending := 0
		if loading {
			pending = pageSkeleton
		}
		label := i18n.T("{title} matching {q}", "title", title, "q", q)
		if group == "song" {
			var songs []services.Song
			var from []services.Item
			for _, it := range items {
				if it.Song != nil {
					songs = append(songs, *it.Song)
					from = append(from, it)
				}
			}
			s.songList(c, &v.songs, &v.listMenus, pane, ctx, songs, pending, true, label, func(i int) {
				ctx.OnOpen(from[i])
				s.env.Queue.PlaySongs(songs, i, searchFrom)
			})
		} else {
			count := len(items) + pending
			kit.ItemGrid(c, &v.grid, count, func(i int) *services.Item {
				if i < len(items) {
					return &items[i]
				}
				return nil
			}, ctx, kit.ItemGridOpts{Label: label, Pane: pane})
		}
		if failed != nil {
			ui.Box(c).Height(24)
			kit.ErrorBlock(c, i18n.T("Couldn't load more"), failed, kit.ErrorBlockOpts{
				OnRetry: func() { s.env.Queries.Invalidate(keyGroup(q, group, failedAt)) },
			})
		}

		// The end of the list: when it comes within a screen of the view,
		// ask for the next page.
		end := ui.Box(c.Key("end")).FillWidth().Height(1).Role(ui.RoleNone)
		settled := v.built == len(items)+pending
		if !settled {
			v.built = len(items) + pending
			c.Invalidate()
		}
		if settled && more && failed == nil && !loading && loaded == v.want {
			if b, p := end.Bounds(), pane.Bounds(); b.H > 0 && p.H > 0 && b.Y < p.Y+2*p.H {
				v.want++
				c.Invalidate()
			}
		}
	})
}

// groupHeader is the page's title and the line under it, "Apple Music
// results for “q”", with the term linking back to the search.
func (s *search) groupHeader(c *ui.Context, group, q string) {
	ui.Column(c.Key("header")).FillWidth().Margin(0, 0, 24, 0).Children(func() {
		theme.Text(c, groupTitle(group), "display").TextColor(theme.C("ink")).SingleLine().Role(ui.RoleHeading).Level(1)
		msg := i18n.T("Apple Music results for <0>“{q}”</0>", "q", q)
		before, rest, _ := strings.Cut(msg, "<0>")
		link, after, _ := strings.Cut(rest, "</0>")
		ui.Row(c.Key("subline")).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			theme.Text(c, before, "secondary").TextColor(theme.C("ink-muted")).SingleLine()
			l := ui.ButtonBase(c.Key("q")).Role(ui.RoleLink).Radius(2)
			l.Children(func() {
				theme.Text(c, link, "secondary").TextColor(theme.C("ink")).SingleLine().Underline()
			})
			if l.Clicked() {
				s.env.Nav.Push(searchLocation(q, scopeCatalog))
			}
			if after != "" {
				theme.Text(c, after, "secondary").TextColor(theme.C("ink-muted")).SingleLine()
			}
		})
	})
}
