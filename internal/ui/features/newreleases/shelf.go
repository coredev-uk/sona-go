package newreleases

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// shelfRows is how many releases For You's shelf shows: the page behind "See
// all" has the rest.
const shelfRows = 4

// shelfItems are the releases the shelf is for: those still to come (soonest
// first), then the unread ones that are out (newest first). Read releases
// that are out stay on the page only.
func shelfItems(groups []group) []services.NewRelease {
	var out []services.NewRelease
	for _, g := range groups {
		for _, r := range g.items {
			if g.id == "soon" || !r.Read {
				out = append(out, r)
			}
		}
	}
	return out
}

// forYouShelf is Env.NewReleasesShelf, For You's "New from Artists You Love":
// the next shelfRows of the upcoming and unread releases, with the whole list
// one "See all" away. Nothing is drawn while the tracker is off, the list is
// loading or can't be had (For You has plenty without), and when no release is
// unread or upcoming: the shelf comes back with the next one.
func (n *newReleases) forYouShelf(c *ui.Context, frame func(title string, more func(), body func())) {
	if !n.tracking() {
		return
	}
	list := store.UseNewReleases(n.env.Queries, n.env.Core)
	if !list.HasData {
		return
	}
	now := n.now()
	items := shelfItems(groupsOf(list.Data.Items, now))
	if len(items) == 0 {
		return
	}
	shown := []group{{id: "shelf", title: shelfTitle(), items: items[:min(len(items), shelfRows)]}}
	frame(shelfTitle(), func() { n.env.Nav.Push(path) }, func() {
		n.releases(c, &n.shelf, shown, now, false)
	})
}
