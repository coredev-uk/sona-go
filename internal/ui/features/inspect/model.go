package inspect

import (
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// panelData is a report as the tabs show it. It is built off the main thread
// (parsing a cached Apple page is the one costly step) and handed to the view
// whole; only the view changes its trees' folds and filters.
type panelData struct {
	// overview is the mapped item as a ledger, library the SQLite row.
	overview, library []ledgerRow
	// mapped, apple and addon are the JSON trees; nil for a tab with nothing.
	mapped, apple, addon *jtree
}

func buildData(rep services.InspectReport) *panelData {
	d := &panelData{mapped: newJTree(rep.Mapped), apple: newJTree(rep.Raw), addon: newJTree(addonDoc(rep.Addon))}
	if d.mapped != nil {
		d.overview = ledgerOf(d.mapped.root, rep.Art)
	}
	if t := parseJSON(rep.Row); t != nil {
		d.library = ledgerOf(t, services.InspectArt{})
	}
	return d
}

// tabLabels are the tabs' names, in order.
func tabLabels() []string {
	return []string{i18n.T("Overview"), i18n.T("Apple"), i18n.T("Library"), i18n.T("Add-on")}
}

// reasons says, for each tab, why it has nothing to show ("" when it has).
// While the report loads only Overview is offered.
func (in *inspector) reasons() []string {
	r := make([]string, tabCount)
	switch {
	case in.err != nil:
		for i := tabApple; i < tabCount; i++ {
			r[i] = i18n.T("The report could not be read")
		}
	case in.loading || in.data == nil:
		for i := tabApple; i < tabCount; i++ {
			r[i] = i18n.T("Loading…")
		}
	default:
		if in.data.apple == nil {
			r[tabApple] = i18n.T("Not in Go's catalogue cache")
		}
		if len(in.data.library) == 0 {
			r[tabLibrary] = i18n.T("Not in your library")
		}
		if in.data.addon == nil {
			r[tabAddon] = i18n.T("Add-ons aren't offered this kind of item")
		}
	}
	return r
}

// kindLabel is how the header names the kind of item.
func kindLabel(k kit.ItemKind) string {
	switch k {
	case kit.KindSong:
		return i18n.T("Song")
	case kit.KindAlbum:
		return i18n.T("Album")
	case kit.KindPlaylist:
		return i18n.T("Playlist")
	case kit.KindArtist:
		return i18n.T("Artist")
	case kit.KindStation:
		return i18n.T("Station")
	case kit.KindMusicVideo:
		return i18n.T("Music video")
	}
	return string(k)
}

// subtitleOf is the line after the kind: the artist, the curator, the
// provider.
func subtitleOf(item kit.Item) string {
	switch t := item.Target.(type) {
	case services.Song:
		return t.Artist
	case services.Album:
		return t.Artist
	case services.Playlist:
		return t.Curator
	case services.Station:
		return t.Provider
	case services.MusicVideo:
		return t.Artist
	}
	return ""
}

// metaLine is "Song · Radiohead" in the catalogue line's form.
func metaLine(item kit.Item) string {
	parts := []string{kindLabel(item.Kind)}
	if s := strings.TrimSpace(subtitleOf(item)); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " · ")
}

// artSource is the Go-served artwork the header shows, the medium size: the
// loader decodes it at the 56 DIP the header gives it.
func artSource(item kit.Item) string {
	var a services.Art
	switch t := item.Target.(type) {
	case services.Song:
		a = t.Art
	case services.Album:
		a = t.Art
	case services.Playlist:
		a = t.Art
	case services.Artist:
		a = t.Art
	case services.Station:
		a = t.Art
	case services.MusicVideo:
		a = t.Art
	}
	if a.Medium != "" {
		return a.Medium
	}
	return a.Large
}

// chip is an ID the header offers to copy.
type chip struct{ label, value string }

// chipsOf are the item's IDs: the catalogue's, the library's and, where the
// cached resource names one that differs, a playlist's global ID.
func (in *inspector) chipsOf() []chip {
	q := in.query
	var out []chip
	if in.item.Kind == kit.KindStation && q.CatalogID != "" {
		out = append(out, chip{i18n.T("Station"), q.CatalogID})
	} else if q.CatalogID != "" {
		out = append(out, chip{i18n.T("Catalogue"), q.CatalogID})
	}
	if q.LibraryID != "" {
		out = append(out, chip{i18n.T("Library"), q.LibraryID})
	}
	if g := in.report.GlobalID; g != "" {
		out = append(out, chip{i18n.T("Global"), g})
	}
	if len(out) == 0 && in.id() != "" {
		out = append(out, chip{"ID", in.id()})
	}
	return out
}
