package album

import (
	"slices"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// maxWriters: at most this many songwriters are named; the rest are counted.
const maxWriters = 8

// fact is one line of the fine print: what it is ("Label") and the record's
// entry.
type fact struct{ label, value string }

// model is everything the page derives from the album's data: the part of
// v0.2.0's AlbumBody that is not drawing.
type model struct {
	album  services.Album
	header services.Header
	list   []services.Song
	// page is Apple's catalogue page for the album: the album's own when it
	// is a catalogue one, a library album's linked one, else nil.
	page      *services.CatalogAlbum
	catalogue bool // the loaded album is itself the catalogue page

	upcoming    bool
	popular     map[string]bool
	compilation bool
	writers     []string
	classical   bool
	composers   int // how many composers a classical album's headings tell apart
	discs       []disc
	lede        string
	notes       string
	share       *share
	meta        string
	songsOut    int
	facts       []fact
	rights      string
	motion      string
}

// withCatalogueRelease gives a library album the release state of its linked
// catalogue album when that one is not out yet. Apple answers a pre-added
// album's library entry without a releaseDate or isPrerelease, and the
// library keeps what it last synced: the catalogue page, fetched with the
// album, is the one that knows. The songs the album's library entry dates
// not themselves are not out either (services.markUnreleased).
func withCatalogueRelease(d services.AlbumDetail, page *services.CatalogAlbum, now time.Time) services.AlbumDetail {
	if page == nil || !kit.IsUpcoming(page.Album, now) {
		return d
	}
	d.Album.Prerelease, d.Album.ReleaseAt, d.Album.ReleaseDate = true, page.Album.ReleaseAt, page.Album.ReleaseDate
	d.Tracks = slices.Clone(d.Tracks)
	for i := range d.Tracks {
		if d.Tracks[i].ReleaseDate == "" {
			d.Tracks[i].Playable = false
		}
	}
	return d
}

// newModel works the page out of the loaded album and, when there is one,
// Apple's page for it.
func newModel(detail services.AlbumDetail, page *services.CatalogAlbum, catalogue bool, now time.Time) model {
	album := detail.Album
	m := model{album: album, header: detail.Header, page: page, catalogue: catalogue}
	m.upcoming = kit.IsUpcoming(album, now)
	m.list = withoutPlaceholderCredits(detail.Tracks, m.upcoming)

	// "Popular" marks a song to play: never one that can't be.
	m.popular = map[string]bool{}
	if page != nil {
		ids := map[string]bool{}
		for _, id := range page.PopularIDs {
			ids[id] = true
		}
		for _, s := range m.list {
			if s.Playable && ids[s.CatalogID] {
				m.popular[s.CatalogID] = true
			}
		}
	}
	m.compilation = isCompilation(m.list, album.Artist)
	// The catalogue's songs carry Apple's credits (a library album's linked
	// page's do).
	credited := m.list
	if page != nil {
		credited = page.Tracks
	}
	m.writers = writersOf(credited)
	m.classical = hasWorks(m.list)
	if m.classical {
		m.composers = len(m.writers)
	}
	// A list long enough to be virtualised has no disc headings to jump to.
	if len(m.list) <= kit.VirtualFrom {
		m.discs = discsOf(m.list)
	}

	label, copyright, notes := detail.Label, detail.Copyright, detail.Notes
	if page != nil {
		label = orElse(label, page.Label)
		copyright = orElse(copyright, page.Copyright)
		notes = orElse(notes, page.Notes)
	}
	m.rights = copyright
	m.notes = notes
	// The header carries Apple's one-line note; the full notes follow the songs.
	if page != nil {
		m.lede = page.NotesShort
	}
	if !catalogue {
		tracks := 0
		if page != nil {
			tracks = len(page.Tracks)
		}
		m.share = libraryShare(len(m.list), tracks)
	}

	// The catalogue line describes the record: a partial library album counts
	// the whole album; an album not out yet leaves out a running time its
	// unpublished songs can't add to.
	songs := len(m.list)
	running := detail.DurationMs
	switch {
	case m.share != nil:
		songs = m.share.total
		running = 0
		if page != nil {
			running = page.DurationMs
		}
	case m.upcoming && anyWithoutDuration(m.list):
		running = 0
	}
	kind := i18n.T("Album")
	if album.Single {
		kind = i18n.T("Single")
	}
	meta := catalogueLine(kind, i18n.YearOf(album.ReleaseDate), songs, running)
	if album.Genre != "" {
		meta += " · " + album.Genre
	}
	m.meta = meta

	for _, s := range m.list {
		if s.Playable {
			m.songsOut++
		}
	}

	firstYear := firstReleased(credited, album.ReleaseDate)
	var genres []string
	switch {
	case page != nil && len(page.Genres) > 0:
		genres = page.Genres
	case album.Genre != "":
		genres = []string{album.Genre}
	}
	if !m.upcoming && album.ReleaseDate != "" {
		// An album not out yet says its day in the countdown.
		m.facts = append(m.facts, fact{i18n.T("Released"), i18n.FormatISODate(album.ReleaseDate, i18n.DateLong)})
	}
	if firstYear != "" {
		m.facts = append(m.facts, fact{i18n.T("First released"), firstYear})
	}
	if len(genres) > 0 {
		m.facts = append(m.facts, fact{
			// translators: {0} is the number of genres.
			i18n.T("{0, plural, one {Genre} other {Genres}}", "0", len(genres)),
			strings.Join(genres, ", "),
		})
	}
	if len(m.writers) > 0 {
		people := joinNames(m.writers)
		if m.classical {
			// translators: {0} is the number of composers.
			m.facts = append(m.facts, fact{i18n.T("{0, plural, one {Composer} other {Composers}}", "0", len(m.writers)), people})
		} else {
			m.facts = append(m.facts, fact{i18n.T("Written by"), people})
		}
	}
	// The label, unless the copyright line already names it.
	if label != "" && !strings.Contains(copyright, label) {
		m.facts = append(m.facts, fact{i18n.T("Label"), label})
	}
	// A library album plays the motion cover of its catalogue album.
	m.motion = detail.Header.Motion
	if m.motion == "" && page != nil {
		m.motion = page.Header.Motion
	}
	return m
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func anyWithoutDuration(songs []services.Song) bool {
	for _, s := range songs {
		if s.DurationMs == 0 {
			return true
		}
	}
	return false
}

// joinNames is names as one phrase in the shown language: "A, B and C", or
// "A, B and 4 others" past maxWriters.
func joinNames(names []string) string {
	rest := len(names) - maxWriters
	shown := names
	if rest > 1 {
		shown = append(append([]string(nil), names[:maxWriters]...),
			i18n.T("{rest, plural, one {# other} other {# others}}", "rest", rest))
	}
	return i18n.List(shown)
}

// playOutLabel is the Play button of an album not out yet: "Play the 3 songs
// out now".
func (m *model) playOutLabel() string {
	return i18n.T("{songsOut, plural, one {Play the # song out now} other {Play the # songs out now}}", "songsOut", m.songsOut)
}

// libraryShareText is "3 of 13 songs in your library".
func libraryShareText(s share) string {
	return i18n.T("{total, plural, one {{have} of # song in your library} other {{have} of # songs in your library}}",
		"total", s.total, "have", s.have)
}

// songsOutText is "3 of 12 songs out now".
func (m *model) songsOutText() string {
	return i18n.T("{total, plural, one {{out} of # song out now} other {{out} of # songs out now}}",
		"total", len(m.list), "out", m.songsOut)
}

// subline is the line under a song's title on the album: a credit other than
// the album artist's (when the list has no Artist column), the composer of a
// piece outside any work on a classical album of several composers, then why
// the song doesn't play, so a greyed row is never left unexplained.
func (m *model) subline(now time.Time, restricted bool) func(services.Song) string {
	releaseDate := i18n.FormatISODate(m.album.ReleaseDate, i18n.DateLong)
	out := ""
	if m.upcoming {
		out = kit.ReleaseBadge(m.album, now)
	}
	if out == "" {
		out = i18n.T("Out {releaseDate}", "releaseDate", releaseDate)
	}
	unavailable := i18n.T("Unavailable")
	withheld := i18n.T("Explicit content is restricted on this account")
	composers := m.composers > 1
	return func(s services.Song) string {
		var parts []string
		if !m.compilation && s.Artist != "" && s.Artist != m.album.Artist {
			parts = append(parts, s.Artist)
		}
		if composers && s.Work == "" && s.Composer != "" {
			parts = append(parts, s.Composer)
		}
		switch {
		case !s.Playable && m.upcoming:
			parts = append(parts, out)
		case !s.Playable:
			parts = append(parts, unavailable)
		case restricted && s.Explicit:
			parts = append(parts, withheld)
		}
		return strings.Join(parts, " · ")
	}
}
