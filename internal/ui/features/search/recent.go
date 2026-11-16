package search

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// recentItem is a remembered item as a card: enough to show it and open
// its page (the page loads the rest).
func recentItem(r services.RecentSearch) services.Item {
	it := services.Item{Kind: r.Kind, ID: r.ItemID, Name: r.Name, Subtitle: r.Subtitle}
	if r.Art != "" {
		it.Art = services.Art{Small: r.Art, Medium: r.Art, Large: r.Art}
	}
	if r.Kind == services.ItemStation && r.NoPage {
		// A station with no page plays when chosen, as the live result does.
		it.Station = &services.Station{ID: r.ItemID, Name: r.Name, Playable: true}
	}
	return it
}

// recentVisible reports whether a remembered entry is listed for the scope on
// screen, by the field's list and the results page's shelf alike: the terms
// searched there and the items from there. A library song is listed nowhere:
// it has no page to open and nothing to play it from.
func recentVisible(r services.RecentSearch, scope string) bool {
	switch {
	case r.Kind == services.RecentTerm:
		return cleanScope(r.Scope) == scope
	case r.Kind == services.ItemSong && isLibraryEntry(r):
		return false
	}
	return isLibraryEntry(r) == (scope == scopeLibrary)
}

// isLibraryEntry reports whether a recent entry was a library item (its ID
// is a library ID).
func isLibraryEntry(r services.RecentSearch) bool {
	return r.Kind != services.RecentTerm && r.Source == services.SourceLibrary
}

// recentPath is the page a remembered item opens: "" for one with none (a
// station that plays when chosen). Songs open their song page (a recent
// entry does not keep the album a song result linked to).
func recentPath(r services.RecentSearch) string {
	return kit.ItemLink(recentItem(r))
}
