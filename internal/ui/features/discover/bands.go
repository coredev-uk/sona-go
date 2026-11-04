package discover

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// bandID is one of Discover's bands (v0.2.0's bands.ts): the page's shelves
// grouped by what they hold, so the page reads as songs and albums, then
// playlists, radio, videos and the ways to browse further.
type bandID string

const (
	bandMusic     bandID = "music"
	bandPlaylists bandID = "playlists"
	bandRadio     bandID = "radio"
	bandVideo     bandID = "video"
	bandBrowse    bandID = "browse"
)

// bandIDs is the bands' order on the page.
var bandIDs = []bandID{bandMusic, bandPlaylists, bandRadio, bandVideo, bandBrowse}

// band is a band's sections, in Apple's order.
type band struct {
	id       bandID
	sections []services.Section
}

// bandTitle is a band's heading.
func bandTitle(id bandID) string {
	switch id {
	case bandMusic:
		return i18n.T("Songs and albums")
	case bandPlaylists:
		return i18n.T("Playlists")
	case bandRadio:
		return i18n.T("Radio")
	case bandVideo:
		return i18n.T("Videos")
	}
	return i18n.T("Browse")
}

// itemBand is the band an item belongs to.
func itemBand(it services.Item) bandID {
	switch it.Kind {
	case services.ItemAlbum, services.ItemSong, services.ItemArtist:
		return bandMusic
	case services.ItemPlaylist:
		return bandPlaylists
	case services.ItemStation:
		return bandRadio
	case services.ItemMusicVideo:
		return bandVideo
	}
	// Curators, rooms, hubs and web links; a web link to a video is a video.
	if kit.ItemVideo(it) {
		return bandVideo
	}
	return bandBrowse
}

// sectionBand is the band a section belongs to: by its kind, else by most of
// its items (the first item's band breaks a tie).
func sectionBand(s services.Section) bandID {
	switch s.Kind {
	case services.SectionTracks:
		return bandMusic
	case services.SectionLive, services.SectionStations:
		return bandRadio
	case services.SectionBricks, services.SectionLinks:
		return bandBrowse
	}
	type count struct {
		id bandID
		n  int
	}
	// In the order the bands are first seen, so a tie goes to the first.
	var counts []count
	for _, it := range s.Items {
		id := itemBand(it)
		found := false
		for i := range counts {
			if counts[i].id == id {
				counts[i].n++
				found = true
				break
			}
		}
		if !found {
			counts = append(counts, count{id, 1})
		}
	}
	best, most := bandBrowse, 0
	for _, c := range counts {
		if c.n > most {
			best, most = c.id, c.n
		}
	}
	return best
}

// toBands is the page's sections in bands, in bandIDs order, keeping Apple's
// order inside each band. A paragraph goes with the section after it (it
// introduces it); a trailing one with the last section.
func toBands(sections []services.Section) []band {
	byBand := map[bandID][]services.Section{}
	var last bandID
	var pending []services.Section
	for _, s := range sections {
		if s.Kind == services.SectionText {
			pending = append(pending, s)
			continue
		}
		last = sectionBand(s)
		byBand[last] = append(append(byBand[last], pending...), s)
		pending = nil
	}
	if len(pending) > 0 {
		id := last
		if id == "" {
			id = bandBrowse
		}
		byBand[id] = append(byBand[id], pending...)
	}
	var out []band
	for _, id := range bandIDs {
		if found := byBand[id]; len(found) > 0 {
			out = append(out, band{id: id, sections: found})
		}
	}
	return out
}
