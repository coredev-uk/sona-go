package search

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// Search result groups: Apple's group keys (SearchSection.Group) and how
// Sona titles them (v0.2.0's groups.ts). Apple's own group names are not
// shown.

// Songs shown before "See all", and top results shown as rows.
const (
	songRows = 8
	topRows  = 9
)

// groupTitle is the section title for a group; an unknown group gets its
// key in sentence case.
func groupTitle(group string) string {
	switch group {
	case "top":
		return i18n.T("Top results")
	case "artist":
		return i18n.T("Artists")
	case "album":
		return i18n.T("Albums")
	case "song":
		return i18n.T("Songs")
	case "playlist":
		return i18n.T("Playlists")
	case "radio_show":
		return i18n.T("Radio shows")
	case "radio_episode":
		return i18n.T("Radio episodes")
	case "station":
		return i18n.T("Stations")
	case "music_video":
		return i18n.T("Music videos")
	case "category":
		return i18n.T("Categories")
	case "curator":
		return i18n.T("Curators")
	case "record_label":
		return i18n.T("Labels")
	case "profile":
		return i18n.T("Profiles")
	case "shared_playlist":
		return i18n.T("Playlists by listeners")
	}
	words := strings.TrimSpace(strings.Join(strings.FieldsFunc(group, func(r rune) bool { return r == '_' || r == '-' }), " "))
	if words == "" {
		return i18n.T("Results")
	}
	r, n := utf8.DecodeRuneInString(words)
	return string(unicode.ToUpper(r)) + words[n:] + "s"
}

// groupLoadError is the error title of a group's "see all" page.
func groupLoadError(group string) string {
	switch group {
	case "top":
		return i18n.T("Couldn't load top results")
	case "artist":
		return i18n.T("Couldn't load artists")
	case "album":
		return i18n.T("Couldn't load albums")
	case "song":
		return i18n.T("Couldn't load songs")
	case "playlist":
		return i18n.T("Couldn't load playlists")
	case "radio_show":
		return i18n.T("Couldn't load radio shows")
	case "radio_episode":
		return i18n.T("Couldn't load radio episodes")
	case "station":
		return i18n.T("Couldn't load stations")
	case "music_video":
		return i18n.T("Couldn't load music videos")
	case "category":
		return i18n.T("Couldn't load categories")
	case "curator":
		return i18n.T("Couldn't load curators")
	case "record_label":
		return i18n.T("Couldn't load labels")
	case "profile":
		return i18n.T("Couldn't load profiles")
	case "shared_playlist":
		return i18n.T("Couldn't load playlists by listeners")
	}
	return i18n.T("Couldn't load these results")
}

// groupEmpty is the empty-state title of a group's "see all" page.
func groupEmpty(group, q string) string {
	switch group {
	case "top":
		return i18n.T("No top results for “{q}”", "q", q)
	case "artist":
		return i18n.T("No artists for “{q}”", "q", q)
	case "album":
		return i18n.T("No albums for “{q}”", "q", q)
	case "song":
		return i18n.T("No songs for “{q}”", "q", q)
	case "playlist":
		return i18n.T("No playlists for “{q}”", "q", q)
	case "radio_show":
		return i18n.T("No radio shows for “{q}”", "q", q)
	case "radio_episode":
		return i18n.T("No radio episodes for “{q}”", "q", q)
	case "station":
		return i18n.T("No stations for “{q}”", "q", q)
	case "music_video":
		return i18n.T("No music videos for “{q}”", "q", q)
	case "category":
		return i18n.T("No categories for “{q}”", "q", q)
	case "curator":
		return i18n.T("No curators for “{q}”", "q", q)
	case "record_label":
		return i18n.T("No labels for “{q}”", "q", q)
	case "profile":
		return i18n.T("No profiles for “{q}”", "q", q)
	case "shared_playlist":
		return i18n.T("No playlists by listeners for “{q}”", "q", q)
	}
	return i18n.T("Nothing for “{q}”", "q", q)
}

// hasSeeAll: groups have a "see all" page when Apple has a next page or the
// section holds more than it shows (Top results is a fixed set).
func hasSeeAll(group string, next, shown, total int) bool {
	return group != "top" && (next > 0 || total > shown)
}

// seeAllWanted is hasSeeAll for a section of results.
func seeAllWanted(sec services.SearchSection) bool {
	shown := len(sec.Items)
	if sec.Group == "song" {
		shown = songRows
	}
	return hasSeeAll(sec.Group, sec.Next, shown, len(sec.Items))
}

// kindLabel is the noun for one item of a kind, for the "Album · Artist"
// line of mixed lists; "" for a kind with none.
func kindLabel(it services.Item) string {
	switch it.Kind {
	case services.ItemSong:
		return i18n.T("Song")
	case services.ItemAlbum:
		return i18n.T("Album")
	case services.ItemArtist:
		return i18n.T("Artist")
	case services.ItemPlaylist:
		return i18n.T("Playlist")
	case services.ItemStation:
		if it.Station != nil {
			switch it.Station.Kind {
			case services.StationEpisode:
				return i18n.T("Radio episode")
			case services.StationLive, services.StationBroadcast:
				return i18n.T("Live radio")
			}
		}
		return i18n.T("Station")
	case services.ItemMusicVideo:
		return i18n.T("Music video")
	case services.ItemCurator:
		return i18n.T("Curator")
	case services.ItemProfile:
		return i18n.T("Profile")
	case services.ItemRoom, services.ItemMultiroom, services.ItemGrouping:
		return i18n.T("Collection")
	case services.ItemCharts:
		return i18n.T("Charts")
	}
	return ""
}

// itemMeta is "Song · Artist": the item's kind, then its own subtitle.
func itemMeta(it services.Item) string {
	return joinMeta(kindLabel(it), it.Subtitle)
}

func joinMeta(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// itemPath is the Sona page a result opens, "" when it has none (a song
// plays in place; a station with nothing to show plays or opens in the
// browser; web-only items). The caller links only when env.HasRoute says
// the page is ported.
func itemPath(it services.Item) string {
	if it.Kind == services.ItemSong {
		return ""
	}
	return kit.ItemLink(it)
}
