package artist

import (
	"regexp"
	"slices"
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is v0.2.0's features/artist/artist.ts: the pure rules of the
// artist page, which the views only draw.

const (
	// topRows are the top songs on the page; the rest are behind "See all".
	topRows = 6
	// essentialRows are the essential albums beside top songs.
	essentialRows = 6
	// catalogueRows are the releases in each catalogue list before "See all".
	catalogueRows = 5
)

const (
	// wideFrom is the content width from which the lead has a second
	// column (top songs beside the essential albums) and About sets the
	// facts beside the biography.
	wideFrom = 800
	// slipFrom is the content width from which the latest release stands in
	// a column of its own between the text and the portrait.
	slipFrom = 1040
	// narrowUnder is the content width under which the portrait drops to its
	// small size.
	narrowUnder = 720
	// columnMin is a catalogue column's smallest width, columnGap the gap
	// between columns.
	columnMin = 260
	columnGap = 32
	// leadSide is the essential albums panel beside the top songs.
	leadSide = 340
	// timelineTwoFrom is the content width from which a year with several
	// releases sets them in two columns.
	timelineTwoFrom = 840
)

// layout is how the page lays out at a content width.
type layout struct {
	// wide: essential albums beside the top songs, the facts beside the
	// biography.
	wide bool
	// slipBeside: the latest release stands as the header's second column
	// (a stacked cover and its lines); otherwise it is a row under the
	// text.
	slipBeside bool
	// portrait is the artist's round picture, at the header's trailing
	// edge.
	portrait float32
	// slip is the latest release's cover.
	slip float32
}

// layoutFor is how the page lays out at a content width (0 before the first
// layout: the full-size portrait, so nothing jumps).
func layoutFor(width float32) layout {
	l := layout{wide: width >= wideFrom, slipBeside: width >= slipFrom, portrait: 200, slip: 88}
	switch {
	case l.slipBeside:
		l.slip = 152
	case width > 0 && width < narrowUnder:
		l.portrait, l.slip = 112, 64
	case width > 0:
		l.portrait = 176
	}
	return l
}

// catalogueColumns is how many catalogue lists sit side by side: as many as
// fit, at most one each.
func catalogueColumns(width float32, lists int) int {
	fit := int((width + columnGap) / (columnMin + columnGap))
	return max(1, min(lists, fit))
}

// The page's bands and the artist views each holds, in the order they show.
var (
	// viewsAlbums are cover tiles: the artist's albums.
	viewsAlbums = []string{services.ViewFullAlbums}
	// viewsCatalogue are compact lists side by side, the label's back
	// catalogue.
	viewsCatalogue = []string{services.ViewSingles, services.ViewLiveAlbums, services.ViewCompilations}
	viewsVideos    = []string{services.ViewTopMusicVideos, services.ViewMusicVideos}
	// viewsAppears is a compact list, like the catalogue: albums by others
	// the artist is on.
	viewsAppears = []string{services.ViewAppearsOn}
	viewsRelated = []string{services.ViewPlaylists, services.ViewSimilarArtists}
)

// shelvesFor are the artist's shelves for views, in that order (Apple sends
// each at most once).
func shelvesFor(page *services.CatalogArtist, views []string) []services.ArtistShelf {
	var out []services.ArtistShelf
	for _, v := range views {
		for _, sh := range page.Shelves {
			if sh.View == v {
				out = append(out, sh)
			}
		}
	}
	return out
}

// videoShelf is the page's one video shelf: Apple's "top" pick or the full
// list, whichever comes first in viewsVideos.
func videoShelf(page *services.CatalogArtist) (services.ArtistShelf, bool) {
	if s := shelvesFor(page, viewsVideos); len(s) > 0 {
		return s[0], true
	}
	return services.ArtistShelf{}, false
}

// essentials are the essential albums Apple picks for the artist, for the
// lead.
func essentials(page *services.CatalogArtist) (services.ArtistShelf, bool) {
	for _, sh := range page.Shelves {
		if sh.View == services.ViewFeaturedAlbums && sh.Kind == services.ShelfAlbums {
			return sh, true
		}
	}
	return services.ArtistShelf{}, false
}

// ownReleases are the artist's own release views: the count is complete when
// none of them has more to fetch.
var ownReleases = slices.Concat(viewsAlbums, viewsCatalogue)

// releaseCount is how many releases the artist has, when the page holds all
// of them (no own-release shelf has more to fetch): a small artist's whole
// output. ok is false when Apple has more than the page shows.
func releaseCount(page *services.CatalogArtist) (n int, ok bool) {
	ids := map[string]bool{}
	for _, sh := range page.Shelves {
		if !slices.Contains(ownReleases, sh.View) {
			continue
		}
		if sh.More {
			return 0, false
		}
		for _, a := range sh.Albums {
			ids[a.ID] = true
		}
	}
	if page.LatestRelease != nil {
		ids[page.LatestRelease.ID] = true
	}
	return len(ids), true
}

// artistLine is the artist's catalogue line (docs/design.md): "Artist ·
// Alternative · Abingdon, Oxfordshire, England", with the release count when
// the page holds every release ("Artist · 1 release"; 0 leaves it out).
// extra is a library line.
func artistLine(genre, origin string, releases int, extra string) string {
	count := ""
	if releases > 0 {
		count = i18n.T("{releases, plural, one {# release} other {# releases}}", "releases", releases)
	}
	var parts []string
	for _, p := range []string{i18n.T("Artist"), genre, origin, count, extra} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

var paragraphBreak = regexp.MustCompile(`\n{2,}`)

// paragraphs splits a biography on blank lines.
func paragraphs(bio string) []string { return paragraphBreak.Split(bio, -1) }

// bioLead is the biography's first paragraph, for the header.
func bioLead(bio string) string {
	return strings.TrimSpace(paragraphs(bio)[0])
}

var releaseSuffix = regexp.MustCompile(`^(.+?) - (Single|EP)$`)

// releaseTitle is a single's or EP's title without Apple's " - Single" /
// " - EP" suffix, and which it was ("single", "ep", or "" for neither): the
// catalogue lists say the kind in their own line.
func releaseTitle(name string) (title, kind string) {
	m := releaseSuffix.FindStringSubmatch(name)
	if m == nil {
		return name, ""
	}
	return m[1], strings.ToLower(m[2])
}
