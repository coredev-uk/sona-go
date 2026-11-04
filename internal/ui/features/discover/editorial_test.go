package discover

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func editorial(id, title string, sections ...services.Section) services.EditorialPage {
	return services.EditorialPage{ID: id, Title: title, Sections: sections, FetchedAt: uitest.Epoch.UnixMilli()}
}

func TestCuratorPageFromAppleShowsItsBannerHeaderAndShelves(t *testing.T) {
	r := newRig(t, "/curator/990050553", withApple(&fakeApple{t: t}), withSize(1132, 1200))
	r.tt.Until("the curator", r.tt.Shows("The Zane Lowe Show", "The Interview Series"))
	if !r.tt.HasText("Radio show with Zane Lowe") {
		t.Errorf("no byline: %q", r.tt.Texts())
	}
}

func TestMultiroomPageFromAppleShowsItsTitleAndParagraph(t *testing.T) {
	r := newRig(t, "/multiroom/1425024236", withApple(&fakeApple{t: t}), withSize(1132, 1200))
	r.tt.Until("the multiroom", r.tt.Shows("Coming Soon"))
}

func TestEditorialPageWithSquareArtHasTheCollectionHeader(t *testing.T) {
	api := &fakeAPI{}
	p := editorial("1", "A Curator", shelfOf("s", "Shelf", albumItem("1", "An Album")))
	p.Art = services.Art{Large: "/art/0123456789abcdef0123456789ab0001/600x600.jpg"}
	p.Subtitle, p.Notes, p.Color = "Curator kind", "Editorial notes about this curator.", "#336699"
	api.pages = map[string]services.EditorialPage{"curator/1": p}
	r := newRig(t, "/curator/1", withAPI(api))
	r.tt.Until("the page", r.tt.Shows("A Curator", "Curator kind", "Editorial notes about this curator.", "Shelf"))
	if r.tt.HasText("Updated") {
		t.Error("a fresh page shows its freshness in the header")
	}
}

func TestEditorialPageStaleShowsTheFreshnessLineAsMeta(t *testing.T) {
	api := &fakeAPI{}
	p := editorial("1", "A Curator", shelfOf("s", "Shelf", albumItem("1", "An Album")))
	p.Art = services.Art{Large: "/art/0123456789abcdef0123456789ab0001/600x600.jpg"}
	p.Stale, p.FetchedAt = true, uitest.Epoch.Add(-time.Hour).UnixMilli()
	api.pages = map[string]services.EditorialPage{"curator/1": p}
	r := newRig(t, "/curator/1", withAPI(api))
	r.tt.Until("the stale line", r.tt.Shows("Couldn't reach Apple Music · showing the copy from 1 hr. ago"))
}

func TestEditorialPageWithABannerHasPageHeaderSubtitleAndNotes(t *testing.T) {
	api := &fakeAPI{}
	p := editorial("9", "A Campaign", shelfOf("s", "Shelf", albumItem("1", "An Album")))
	p.Wide = services.Art{Large: "/art/0123456789abcdef0123456789ab0009/2400x600.jpg"}
	p.Subtitle, p.Notes = "A tagline", "What the campaign is about."
	api.pages = map[string]services.EditorialPage{"multiroom/9": p}
	r := newRig(t, "/multiroom/9", withAPI(api))
	r.tt.Until("the page", r.tt.Shows("A Campaign", "A tagline", "What the campaign is about.", "Shelf"))
	p.Stale = true
	api2 := &fakeAPI{pages: map[string]services.EditorialPage{"grouping/9": p}}
	r2 := newRig(t, "/grouping/9", withAPI(api2))
	r2.tt.Until("the stale line in place of the subtitle", r2.tt.Shows("A Campaign", "Couldn't reach Apple Music"))
	if r2.tt.HasText("A tagline") {
		t.Error("the subtitle shows beside the stale line")
	}
}

func TestEditorialPageWithNoSectionsSaysSo(t *testing.T) {
	api := &fakeAPI{pages: map[string]services.EditorialPage{"grouping/1": editorial("1", "Empty hub")}}
	r := newRig(t, "/grouping/1", withAPI(api))
	r.tt.Until("the empty state", r.tt.Shows("Empty hub", "Nothing here right now", "Apple Music has no music on this page at the moment."))
}

func TestEditorialPagesLoadingAndErrorTitlesByKind(t *testing.T) {
	api := &fakeAPI{block: make(chan struct{}), pages: map[string]services.EditorialPage{"curator/1": editorial("1", "A Curator", shelfOf("s", "Shelf", albumItem("1", "A")))}}
	r := newRig(t, "/curator/1", withAPI(api))
	r.tt.Until("the skeleton", r.tt.Shows("Loading"))
	close(api.block)
	r.tt.Until("the page", r.tt.Shows("A Curator"))

	for path, title := range map[string]string{
		"/curator/1":   "Couldn't load this curator",
		"/multiroom/1": "Couldn't load this collection",
		"/grouping/1":  "Couldn't load this page",
	} {
		bad := &fakeAPI{err: applemusic.ErrNotFound}
		rr := newRig(t, path, withAPI(bad))
		rr.tt.Until("the error of "+path, rr.tt.Shows(title, i18n.ErrorText(services.CodeNotFound), "Try again"))
	}
}
