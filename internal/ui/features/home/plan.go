package home

import "github.com/coredev-uk/sona-go/internal/services"

// openShelves is how many plain cover shelves Home shows in full; later ones
// are listed in the "More for you" index (v0.2.0's OPEN_SHELVES).
const openShelves = 3

// recentlyPlayed is Apple's source for the Recently Played shelf
// (Section.Source).
const recentlyPlayed = "recently-played"

// homePlan is how Home sets Apple's shelves: the lead (the first "feature"
// shelf, Top Picks), the shelves shown in full in Apple's order, and the
// index (plain cover shelves after the first openShelves, listed one line
// each). Recently Played, station shelves, features, heroes and track shelves
// are always shown in full.
type homePlan struct {
	// lead is nil when there is no feature shelf with items.
	lead  *services.Section
	open  []services.Section
	index []services.Section
}

// planHome is v0.2.0's planHome.
func planHome(sections []services.Section) homePlan {
	var p homePlan
	lead := -1
	for i := range sections {
		if sections[i].Kind == services.SectionFeature && len(sections[i].Items) > 0 {
			lead = i
			p.lead = &sections[i]
			break
		}
	}
	plain := 0
	for i := range sections {
		if i == lead {
			continue
		}
		s := sections[i]
		plainShelf := s.Kind == services.SectionShelf && s.Source != recentlyPlayed && !isStations(s)
		if plainShelf {
			plain++
			if plain > openShelves {
				p.index = append(p.index, s)
				continue
			}
		}
		p.open = append(p.open, s)
	}
	return p
}

// isStations: a shelf of stations only (Stations for You, moods, favourite
// artists' stations): not empty, and every item is a station.
func isStations(s services.Section) bool {
	if len(s.Items) == 0 {
		return false
	}
	for i := range s.Items {
		if s.Items[i].Kind != services.ItemStation {
			return false
		}
	}
	return true
}

// withoutSharedCaption is the shelf's items without a caption every one of
// them shares ("Apple Music for You" under each Made for You playlist): said
// once, it is noise. A shelf whose captions differ (or that has fewer than
// two items) is returned as it is, the same slice.
func withoutSharedCaption(items []services.Item) []services.Item {
	if len(items) < 2 || items[0].Caption == "" {
		return items
	}
	first := items[0].Caption
	for i := range items {
		if items[i].Caption != first {
			return items
		}
	}
	out := make([]services.Item, len(items))
	for i := range items {
		out[i] = items[i]
		out[i].Caption = ""
	}
	return out
}
