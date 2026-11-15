package services

import "testing"

// An upload is a library song with no catalogue copy: the same answer for the
// song, its search item and its queue slot.
func TestIsUploadIsALibrarySongWithNoCatalogueCopy(t *testing.T) {
	for _, c := range []struct {
		name      string
		source    Source
		catalogID string
		want      bool
	}{
		{"upload", SourceLibrary, "", true},
		{"library song with a catalogue copy", SourceLibrary, "1440", false},
		{"catalogue song", SourceCatalog, "1440", false},
		{"catalogue song with no ID yet", SourceCatalog, "", false},
	} {
		s := Song{Source: c.source, CatalogID: c.catalogID}
		if got := s.IsUpload(); got != c.want {
			t.Errorf("%s: Song.IsUpload = %v", c.name, got)
		}
		if got := (Item{Kind: ItemSong, Song: &s}).IsUpload(); got != c.want {
			t.Errorf("%s: Item.IsUpload = %v", c.name, got)
		}
		if got := (QueueItem{Source: c.source, CatalogID: c.catalogID}).IsUpload(); got != c.want {
			t.Errorf("%s: QueueItem.IsUpload = %v", c.name, got)
		}
	}
	if (Item{Kind: ItemAlbum}).IsUpload() || (Item{Kind: ItemSong}).IsUpload() {
		t.Error("an item that is no song, or has none, is an upload")
	}
	if (QueueItem{Source: SourceLibrary, Video: true}).IsUpload() || (QueueItem{Source: SourceLibrary, Live: true}).IsUpload() {
		t.Error("a video or live stream is an upload")
	}
}
