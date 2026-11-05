package kit

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The playing item's menu is the one its kind has everywhere else: a song's,
// a music video's, a station's; a station's ID is the stream's.
func TestPlayingTargetFollowsTheKindOfTheEntry(t *testing.T) {
	base := services.QueueItem{ID: "i.1", CatalogID: "1001", Name: "Airbag", Artist: "Radiohead", AlbumID: "al1", ArtistID: "ar1"}
	live, video := base, base
	live.Live, live.ID = true, "ra.1"
	video.Video, video.CatalogID = true, "mv.5"

	song, ok := PlayingTarget(&base)
	if !ok || song.Item.Kind != KindSong || song.Name != "Airbag" {
		t.Fatalf("song target %+v ok=%v", song, ok)
	}
	if s := song.Item.Song(); !s.Playable || s.AlbumID != "al1" || s.ArtistID != "ar1" || s.CatalogID != "1001" {
		t.Errorf("song %+v lacks what Go to album, Go to artist and the queue read", s)
	}
	st, _ := PlayingTarget(&live)
	if st.Item.Kind != KindStation || st.Item.Station().ID != "ra.1" {
		t.Errorf("live entry target %+v", st.Item)
	}
	mv, _ := PlayingTarget(&video)
	if mv.Item.Kind != KindMusicVideo || mv.Item.MusicVideo().CatalogID != "mv.5" {
		t.Errorf("video target %+v", mv.Item)
	}
	if _, ok := PlayingTarget(nil); ok {
		t.Error("a menu for nothing playing")
	}
}
