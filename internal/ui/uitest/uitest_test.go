package uitest

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

func count(t *testing.T, h *Harness, query string) int {
	t.Helper()
	var n int
	if err := h.DB.R.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEnvSeedsTheFixtureLibrary(t *testing.T) {
	h := New(t)
	albums := fixture.Albums()
	if got := count(t, h, `SELECT count(*) FROM library_items WHERE kind = 'album'`); got != len(albums) {
		t.Errorf("%d albums seeded, want %d", got, len(albums))
	}
	songs := 0
	for _, a := range albums {
		songs += a.SongCount()
	}
	if got := count(t, h, `SELECT count(*) FROM library_items WHERE kind = 'song'`); got != songs {
		t.Errorf("%d songs seeded, want %d", got, songs)
	}
	if got := count(t, h, `SELECT count(*) FROM library_items WHERE kind = 'playlist'`); got != len(fixture.Playlists()) {
		t.Errorf("%d playlists seeded", got)
	}
	if got := count(t, h, `SELECT count(*) FROM search_fts`); got == 0 {
		t.Error("the search index is empty")
	}
	// Every album is out already, so every song plays.
	if got := count(t, h, `SELECT count(*) FROM albums WHERE release_date >= date('now')`); got != 0 {
		t.Errorf("%d albums are not released yet", got)
	}
	if Of(h.Env) != h {
		t.Error("Of does not find the Harness of its Env")
	}
}

// The services, the engine and the playback mirror share one clock: a song
// starts at 0:00 and Clock.Advance moves its position.
func TestAdvancingTheClockMovesAPlayingSongsPosition(t *testing.T) {
	h := New(t)
	ref := services.TrackRef{ID: fixture.SongLibraryID(0, 0), Source: services.SourceLibrary}
	if err := services.Service[services.PlaybackService](h.Core).PlayItems(t.Context(), []services.TrackRef{ref}, 0, services.PlayFrom{Kind: "album", ID: fixture.AlbumLibraryID(0)}); err != nil {
		t.Fatal(err)
	}
	// Wait for the song to play and the events of starting it to be over (the
	// coordinator publishes a snapshot per engine event), so none lands after
	// the clock moved.
	quiet := 0
	for deadline := time.Now().Add(5 * time.Second); quiet < 20; time.Sleep(5 * time.Millisecond) {
		if h.Main.Drain() == 0 && h.Env.Play.State().State == store.StatePlaying {
			quiet++
		} else {
			quiet = 0
		}
		if time.Now().After(deadline) {
			t.Fatalf("the mirror never settled on a playing song: %+v", h.Env.Play.State().State)
		}
	}
	if got := h.Env.Play.Position(); got != 0 {
		t.Errorf("a song just started is at %v", got)
	}
	h.Clock.Advance(30 * time.Second)
	if got := h.Env.Play.Position(); got != 30*time.Second {
		t.Errorf("30 s later the song is at %v", got)
	}
}

func TestEmptyAndBareEnvs(t *testing.T) {
	h := New(t, Empty())
	if got := count(t, h, `SELECT count(*) FROM library_items`); got != 0 {
		t.Errorf("an empty library holds %d items", got)
	}
	b := New(t, Bare())
	if b.Core != nil || b.DB != nil || b.Env.Core != nil {
		t.Error("a bare Env has services")
	}
	if b.Env.Bus == nil || b.Env.Play == nil || b.Env.Art == nil || b.Env.Post == nil {
		t.Error("a bare Env lacks the bus, the playback mirror, the artwork loader or the poster")
	}
}

func TestMainThreadRunsWhereTheTestDrains(t *testing.T) {
	m := &MainThread{}
	ran := make(chan struct{})
	go m.Post(func() { close(ran) })
	for !m.Pending() {
		time.Sleep(time.Millisecond)
	}
	select {
	case <-ran:
		t.Fatal("a post ran before Drain")
	default:
	}
	if n := m.Drain(); n != 1 {
		t.Errorf("Drain ran %d functions", n)
	}
	<-ran
}

func TestClockStandsStillUntilAdvanced(t *testing.T) {
	c := NewClock()
	if !c.Now().Equal(Epoch) || !c.Now().Equal(c.Now()) {
		t.Fatal("the clock moves by itself")
	}
	c.Advance(time.Minute)
	if got := c.Now().Sub(Epoch); got != time.Minute {
		t.Errorf("advanced by %v", got)
	}
}

func TestGoldenToleranceAbsorbsAntialiasingNotChanges(t *testing.T) {
	a, b := color.NRGBA{100, 100, 100, 255}, color.NRGBA{100 + blockTolerance, 100, 100, 255}
	if far(a, b) {
		t.Error("a difference at the tolerance counts")
	}
	if !far(a, color.NRGBA{100 + blockTolerance + 1, 100, 100, 255}) {
		t.Error("a difference past the tolerance does not count")
	}
	img := image.NewRGBA(image.Rect(5, 5, 9, 8)) // not at the origin
	if got := toNRGBA(img).Bounds(); got != image.Rect(0, 0, 4, 3) {
		t.Errorf("converted bounds %v", got)
	}
}

func TestGoldenComparesBlocks(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 100, 60))
	for i := 0; i < len(want.Pix); i += 4 {
		want.Pix[i], want.Pix[i+1], want.Pix[i+2], want.Pix[i+3] = 240, 240, 240, 255
	}
	got := image.NewNRGBA(want.Rect)
	copy(got.Pix, want.Pix)
	// A pixel of antialiasing in a block does not move its mean.
	got.SetNRGBA(5, 5, color.NRGBA{0, 0, 0, 255})
	if _, bad, total := compare(got, want); bad != 0 || total != 13*8 {
		t.Errorf("one dark pixel: %d of %d blocks differ", bad, total)
	}
	// A dark box four blocks wide does.
	for y := 16; y < 24; y++ {
		for x := 16; x < 48; x++ {
			got.SetNRGBA(x, y, color.NRGBA{20, 20, 20, 255})
		}
	}
	if _, bad, _ := compare(got, want); bad != 4 {
		t.Errorf("a box of four blocks: %d blocks differ", bad)
	}
}

func TestFixtureCoversAreDeterministicPNGsOfTheAskedSize(t *testing.T) {
	a, b := fixture.Cover(3, 120, 80), fixture.Cover(3, 120, 80)
	if !bytes.Equal(a, b) {
		t.Error("a cover changes between calls")
	}
	if bytes.Equal(a, fixture.Cover(4, 120, 80)) {
		t.Error("two seeds draw the same cover")
	}
	img, err := png.Decode(bytes.NewReader(a))
	if err != nil || img.Bounds().Dx() != 120 || img.Bounds().Dy() != 80 {
		t.Errorf("cover decodes to %v, %v", img.Bounds(), err)
	}
}

func TestFixtureCDNServesTemplatesAndNothingElse(t *testing.T) {
	cdn := &fixture.CDN{}
	c := &http.Client{Transport: cdn}
	url := "https://is1-ssl.mzstatic.com/image/thumb/Fixture/cover3/64x64bb.png"
	resp, err := c.Get(url)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("a cover: %v %v", resp, err)
	}
	_ = resp.Body.Close()
	resp, _ = c.Get("https://is1-ssl.mzstatic.com/image/thumb/Music/other/64x64bb.jpg")
	if resp.StatusCode != 404 {
		t.Errorf("another path answered %d", resp.StatusCode)
	}
	if cdn.Hits() != 1 {
		t.Errorf("%d hits", cdn.Hits())
	}
	srv := httptest.NewServer(fixture.Handler())
	defer srv.Close()
	r, err := http.Get(srv.URL + "/image/thumb/Fixture/playlist1/32x32bb.png")
	if err != nil || r.StatusCode != 200 {
		t.Errorf("the handler: %v %v", r, err)
	}
}
