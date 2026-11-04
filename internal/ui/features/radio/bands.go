package radio

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is v0.2.0's features/radio/bands.ts: where an editorial section
// of Apple's radio page goes on Sona's: "live" with the live stations at the
// top (local broadcasters, or Apple's own live shelves when Sona couldn't
// list those), then the bands in this order: stations to tune to, shows and
// episodes to listen back to, and the ways to browse further.

type place string

const (
	placeLive     place = "live"
	placeStations place = "stations"
	placeShows    place = "shows"
	placeBrowse   place = "browse"
)

type bandID string

const (
	bandStations bandID = "stations"
	bandShows    bandID = "shows"
	bandBrowse   bandID = "browse"
)

// bandIDs are the bands in page order.
var bandIDs = []bandID{bandStations, bandShows, bandBrowse}

// band is one band of the page and the sections placed in it.
type band struct {
	id       bandID
	sections []services.Section
}

// bandTitle is a band's heading.
func bandTitle(id bandID) string {
	switch id {
	case bandStations:
		return i18n.T("Stations")
	case bandShows:
		return i18n.T("Shows and episodes")
	}
	return i18n.T("Browse")
}

func itemPlace(it services.Item) place {
	switch it.Kind {
	case services.ItemStation:
		if it.Station != nil {
			switch it.Station.Kind {
			case services.StationLive, services.StationBroadcast:
				return placeLive
			case services.StationEpisode:
				return placeShows
			}
		}
		return placeStations
	case services.ItemRoom, services.ItemMultiroom, services.ItemGrouping, services.ItemCharts, services.ItemLink:
		return placeBrowse
	}
	// Shows (curators), interviews (videos), sessions and DJ mixes (albums).
	return placeShows
}

// sectionPlace is where a section goes: by its kind, else by most of its
// items (the place seen first breaks a tie).
func sectionPlace(s services.Section) place {
	switch s.Kind {
	case services.SectionLive, services.SectionStations:
		return placeLive
	case services.SectionBricks, services.SectionLinks:
		return placeBrowse
	}
	var order []place
	counts := map[place]int{}
	for _, it := range s.Items {
		p := itemPlace(it)
		if counts[p] == 0 {
			order = append(order, p)
		}
		counts[p]++
	}
	best, most := placeShows, 0
	for _, p := range order {
		if counts[p] > most {
			best, most = p, counts[p]
		}
	}
	return best
}

// placeSections places Apple's radio sections: the live ones, then every
// band in bandIDs order (an empty one too, as the Stations band also holds
// the user's own stations), Apple's order kept inside each. A paragraph goes
// with the section after it (it introduces it); a trailing one with the last
// section.
func placeSections(sections []services.Section) (live []services.Section, bands []band) {
	by := map[place][]services.Section{}
	var last place
	var pending []services.Section
	for _, s := range sections {
		if s.Kind == services.SectionText {
			pending = append(pending, s)
			continue
		}
		last = sectionPlace(s)
		by[last] = append(append(by[last], pending...), s)
		pending = nil
	}
	if len(pending) > 0 {
		p := last
		if p == "" {
			p = placeShows
		}
		by[p] = append(by[p], pending...)
	}
	for _, id := range bandIDs {
		bands = append(bands, band{id: id, sections: by[place(id)]})
	}
	return by[placeLive], bands
}
