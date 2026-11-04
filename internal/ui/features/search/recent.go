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
	return it
}

// isLibraryEntry reports whether a recent entry was a library item (its ID
// is a library ID).
func isLibraryEntry(r services.RecentSearch) bool {
	return r.Kind != services.RecentTerm && r.Source == services.SourceLibrary
}

// recentPath is the page a remembered item opens. Songs open their song
// page (a recent entry does not keep the album a song result linked to);
// library songs have none: "".
func recentPath(r services.RecentSearch) string {
	if r.Kind != services.ItemSong {
		return kit.ItemLink(recentItem(r))
	}
	if isLibraryEntry(r) {
		return ""
	}
	return kit.ItemLink(recentItem(r))
}
