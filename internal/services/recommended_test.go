package services

import (
	"encoding/json"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// A station or episode Apple recommends on Home shows what Apple's own pick
// shows (#372): the reason, the short editorial name, and the one-line
// description, not the tagline label. A station without notes keeps its
// own fields, and a pick of another kind is untouched.
func TestRecommendedStationTakesApplesEditorialFields(t *testing.T) {
	const body = `{
	"data": [
		{"id": "ra.1", "type": "stations", "meta": {"reason": {"stringForDisplay": "We Recommend"}}},
		{"id": "ra.2", "type": "stations", "meta": {"reason": {"stringForDisplay": "Made for You"}}},
		{"id": "pl.1", "type": "playlists", "meta": {"reason": {"stringForDisplay": "We Recommend"}}},
		{"id": "ra.3", "type": "stations"}
	],
	"resources": {
		"stations": {
			"ra.1": {"id": "ra.1", "type": "stations", "attributes": {
				"name": "Hear the very special episode and see some of the best moments.",
				"plainEditorialNotes": {"name": "His Majesty The King", "short": "King Charles joins Elton John for a “musical voyage”.", "tagline": "Only on Apple Music "}}},
			"ra.2": {"id": "ra.2", "type": "stations", "attributes": {"name": "Paul’s Station"}},
			"ra.3": {"id": "ra.3", "type": "stations", "attributes": {
				"name": "Episode",
				"plainEditorialNotes": {"name": "Editorial name", "short": "A line", "tagline": "Only on Apple Music"}}}
		},
		"playlists": {
			"pl.1": {"id": "pl.1", "type": "playlists", "attributes": {
				"name": "Today’s Hits", "curatorName": "Apple Music Hits",
				"plainEditorialNotes": {"name": "Today’s Hits", "short": "The biggest songs."}}}
		}
	}}`
	var resp applemusic.MapResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	items := mapper{}.items(resp.Resources, resp.Data)
	if len(items) != 4 {
		t.Fatalf("%d items: %+v", len(items), items)
	}
	for i, want := range []struct{ name, subtitle, caption string }{
		{"His Majesty The King", "King Charles joins Elton John for a “musical voyage”.", "We Recommend"},
		{"Paul’s Station", "", "Made for You"},
		{"Today’s Hits", "Apple Music Hits", "We Recommend"},
		// Outside a recommendation the station keeps its name and tagline.
		{"Episode", "Only on Apple Music", ""},
	} {
		it := items[i]
		if it.Name != want.name || it.Subtitle != want.subtitle || it.Caption != want.caption {
			t.Errorf("item %d = %q / %q / %q, want %q / %q / %q", i, it.Name, it.Subtitle, it.Caption, want.name, want.subtitle, want.caption)
		}
	}
}
