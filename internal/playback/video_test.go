package playback

import "testing"

func TestVideoID(t *testing.T) {
	if got := VideoID("1440841363"); got != "mv.1440841363" {
		t.Fatalf("VideoID = %q", got)
	}
	for _, tc := range []struct {
		id      string
		video   bool
		catalog string
	}{
		{id: "mv.1440841363", video: true, catalog: "1440841363"},
		{id: VideoID("42"), video: true, catalog: "42"},
		{id: "mv.", video: true, catalog: ""},
		{id: "1440841363"},
		{id: "i.abc123"},
		{id: "MV.42"},
		{id: "xmv.42"},
		{id: ""},
	} {
		if got := IsVideo(tc.id); got != tc.video {
			t.Errorf("IsVideo(%q) = %v, want %v", tc.id, got, tc.video)
		}
		if got := VideoCatalogID(tc.id); got != tc.catalog {
			t.Errorf("VideoCatalogID(%q) = %q, want %q", tc.id, got, tc.catalog)
		}
	}
}
