package motion

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
)

// liveAlbum has motion artwork: Midnights (3am Edition).
const liveAlbum = "1650841512"

// TestLiveRendition fetches a real motion rendition from Apple through the
// cache and checks the file served is plain, starts at 0 and keeps every
// frame. Opt in with SONA_LIVE=1 (catalog requests need no user token).
func TestLiveRendition(t *testing.T) {
	if os.Getenv("SONA_LIVE") != "1" {
		t.Skip("live Apple Music test: set SONA_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	api := applemusic.New(nil, devtoken.NewWebPlayer(nil), nil)
	album, err := applemusic.AMPResource[applemusic.Album](ctx, api, "us", applemusic.TypeAlbums, liveAlbum,
		applemusic.AMPQuery{Extend: []string{"editorialVideo"}})
	if err != nil {
		t.Fatal(err)
	}
	master := album.Attributes.EditorialVideo.Square()
	if master == "" {
		t.Fatalf("album %s has no square motion artwork", liveAlbum)
	}
	c, err := New(Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	u := strings.Replace(c.URL(master), "/720.mp4", "/480.mp4", 1)
	w := get(c, u, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", u, w.Code, w.Body.String())
	}
	f := readFlat(t, w.Body.Bytes())
	if !slices.Equal(f.top, []string{"ftyp", "moov", "mdat"}) || f.mvex {
		t.Fatalf("boxes %v (mvex %v), want ftyp moov mdat", f.top, f.mvex)
	}
	var dur uint64
	for _, d := range f.durations {
		dur += uint64(d)
	}
	pts := f.pts()
	t.Logf("%d frames, %.2f s, ctts v%d, %d sync frames", len(f.durations), float64(dur)/float64(f.scale), f.cttsVersion, len(slices.DeleteFunc(slices.Clone(f.sync), func(s bool) bool { return !s })))
	if len(f.durations) < 30 || f.mediaDur != dur {
		t.Errorf("%d frames, mdhd duration %d, stts total %d", len(f.durations), f.mediaDur, dur)
	}
	if pts[0] != 0 || slices.Min(pts) != 0 || !f.sync[0] {
		t.Errorf("first frame presented at %d (sync %v), earliest at %d; want a keyframe at 0", pts[0], f.sync[0], slices.Min(pts))
	}
}
