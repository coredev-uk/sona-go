package home

import (
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// catalogueLine is the catalogue line of a featured item: what it is, then
// Apple's reason ("Album · New Release"); v0.2.0's useCatalogueLine.
func catalogueLine(it services.Item) string {
	var kind string
	switch it.Kind {
	case services.ItemAlbum:
		kind = i18n.T("Album")
	case services.ItemPlaylist:
		kind = i18n.T("Playlist")
	case services.ItemStation:
		kind = i18n.T("Station")
	case services.ItemArtist:
		kind = i18n.T("Artist")
	case services.ItemSong:
		kind = i18n.T("Song")
	case services.ItemMusicVideo:
		kind = i18n.T("Music video")
	}
	parts := make([]string, 0, 2)
	for _, p := range [...]string{kind, it.Caption} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
