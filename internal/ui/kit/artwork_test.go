package kit

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// fakeArt stands in for the artwork cache: "/art/k…" paths name a template;
// Get answers with a solid red JPEG at the box asked for.
type fakeArt struct {
	mu   sync.Mutex
	gets [][2]int
	fail int           // answer this many Gets with an error
	gate chan struct{} // when set, Get waits for it
}

func (f *fakeArt) Template(p string) (string, bool) {
	if strings.HasPrefix(p, "/art/k") {
		return "https://is1-ssl.mzstatic.com/image/thumb/Music/x/{w}x{h}bb.jpg", true
	}
	return "", false
}

func (f *fakeArt) Get(ctx context.Context, _ string, w, h int) ([]byte, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	f.gets = append(f.gets, [2]int{w, h})
	failing := f.fail > 0
	f.fail--
	f.mu.Unlock()
	if failing {
		return nil, errors.New("artwork unavailable")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 220, 30, 30, 255
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (f *fakeArt) requests() [][2]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][2]int(nil), f.gets...)
}

// mainThread is a Poster whose funcs run where the test says, as the app's
// runs them on the main thread.
type mainThread struct{ q chan func() }

func newMainThread() *mainThread { return &mainThread{q: make(chan func(), 256)} }

func (m *mainThread) post(fn func()) { m.q <- fn }

// run runs what has been posted, waiting up to a few seconds for at least n
// funcs when n > 0.
func (m *mainThread) run(t *testing.T, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for ran := 0; ; {
		if ran >= n {
			select {
			case fn := <-m.q:
				fn()
				ran++
				continue
			default:
				return
			}
		}
		select {
		case fn := <-m.q:
			fn()
			ran++
		case <-deadline:
			t.Fatalf("waited for %d posts, got %d", n, ran)
		}
	}
}

type artApp struct {
	scale float32 // the loader's scale before the first frame (0: not told)
	l     *ArtLoader
	src   string
	size  float32
	opts  ArtOpts
}

func (a *artApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(10).Children(func() {
		Artwork(c.Key("a"), a.l, a.src, a.size, a.opts)
	})
}

func newArtApp(t *testing.T, src *fakeArt, art *artApp) (*ui.Tester, *mainThread) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	mt := newMainThread()
	art.l = newArtLoader(src, mt.post, 2)
	t.Cleanup(art.l.Close)
	if art.scale > 0 {
		art.l.SetScale(art.scale)
	}
	return ui.NewTester(art.view, 200, 200), mt
}

// at is the pixel DIPs from the window's corner (scale 1).
func at(tt *ui.Tester, x, y int) color.RGBA { return tt.Image().RGBAAt(x, y) }

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) < 12 && d(a.G, b.G) < 12 && d(a.B, b.B) < 12
}

func TestArtworkShowsThePlaceholderThenTheImage(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{})}
	a := &artApp{src: "/art/k1/300x300.jpg", size: 100}
	tt, mt := newArtApp(t, src, a)
	tt.Frame()
	hover := theme.C("hover")
	// The corner of the box is the --hover square; the glyph sits in the middle.
	if got := at(tt, 14, 14); !near(got, color.RGBA{hover.R, hover.G, hover.B, 255}) {
		t.Errorf("placeholder corner %v, want %v", got, hover)
	}
	glyph := 0
	for y := 30; y < 100; y++ {
		for x := 30; x < 100; x++ {
			if !near(at(tt, x, y), color.RGBA{hover.R, hover.G, hover.B, 255}) {
				glyph++
			}
		}
	}
	if glyph < 20 {
		t.Errorf("no glyph drawn in the middle: %d pixels differ from the square", glyph)
	}

	close(src.gate) // the loader's worker finishes and posts
	mt.run(t, 1)
	tt.Frame()
	if got := at(tt, 60, 60); !near(got, color.RGBA{220, 30, 30, 255}) {
		t.Errorf("image not shown: centre %v", got)
	}
	if got := at(tt, 14, 14); !near(got, color.RGBA{220, 30, 30, 255}) {
		t.Errorf("image does not fill the box: corner %v", got)
	}
	// Exactly one request, at the box's pixels.
	if got := src.requests(); len(got) != 1 || got[0] != [2]int{100, 100} {
		t.Errorf("requests %v, want one of 100×100", got)
	}
}

func TestArtworkDecodesAtTheExactDevicePixels(t *testing.T) {
	src := &fakeArt{}
	a := &artApp{src: "/art/k1/300x300.jpg", size: 100}
	tt, mt := newArtApp(t, src, a)
	tt.SetScale(2)
	for range 3 { // paint learns the scale, the redraw asks, the worker answers
		tt.Frame()
		mt.run(t, 0)
		time.Sleep(20 * time.Millisecond)
		mt.run(t, 0)
	}
	tt.Frame()
	cv := a.l.covers[artKey{"/art/k1/300x300.jpg", 200, 200}]
	if cv == nil || cv.bitmap() == nil {
		t.Fatalf("no 200×200 bitmap for a 100 DIP box at scale 2; requests %v", src.requests())
	}
	// The window is 400×400 device pixels: the cover fills its 200.
	if got := tt.Image().RGBAAt(2*10+100, 2*10+100); !near(got, color.RGBA{220, 30, 30, 255}) {
		t.Errorf("centre of the cover %v", got)
	}
}

func TestArtworkLearnsTheScaleFromPainting(t *testing.T) {
	src := &fakeArt{}
	a := &artApp{src: "/art/k1/300x300.jpg", size: 100}
	tt, mt := newArtApp(t, src, a)
	tt.SetScale(1.5)
	tt.Frame() // asks at scale 1, learns 1.5 from the painter and posts a redraw
	mt.run(t, 1)
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	reqs := src.requests()
	if len(reqs) != 2 || reqs[len(reqs)-1] != [2]int{150, 150} {
		t.Errorf("requests %v: want the exact 150×150 at the end", reqs)
	}
}

func TestArtworkEdgeIsALineOverTheEdgeOfThePicture(t *testing.T) {
	src := &fakeArt{}
	for _, edge := range []bool{false, true} {
		a := &artApp{src: "/art/k1/300x300.jpg", size: 100, opts: ArtOpts{Edge: edge}}
		tt, mt := newArtApp(t, src, a)
		tt.Frame()
		mt.run(t, 1)
		tt.Frame()
		red := color.RGBA{220, 30, 30, 255}
		if got := at(tt, 11, 60); !near(got, red) {
			t.Errorf("edge %v: the picture is inset: pixel 1 in is %v", edge, got)
		}
		got, want := at(tt, 10, 60), color.RGBA{220, 30, 30, 255}
		ink := theme.C("ink")
		if edge {
			// 6% ink over the picture's outermost pixel.
			want = color.RGBA{uint8(0.94*220 + 0.06*float64(ink.R)), uint8(0.94*30 + 0.06*float64(ink.G)), uint8(0.94*30 + 0.06*float64(ink.B)), 255}
		}
		if !near(got, want) {
			t.Errorf("edge %v: outermost pixel %v, want %v", edge, got, want)
		}
	}
}

func TestArtworkRequestsOnceForTwoViewsOfTheSameCover(t *testing.T) {
	src := &fakeArt{}
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 2)
	defer l.Close()
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Row(c).Gap(4).Children(func() {
			Artwork(c.Key(1), l, "/art/k1/64x64.jpg", 64, ArtOpts{Edge: true})
			Artwork(c.Key(2), l, "/art/k1/64x64.jpg", 64, ArtOpts{})
			Artwork(c.Key(3), l, "/art/k1/64x64.jpg", 32, ArtOpts{}) // another size: another decode
		})
	}, 300, 100)
	tt.Frame()
	mt.run(t, 2)
	tt.Frame()
	if got := len(src.requests()); got != 2 {
		t.Errorf("%d requests for two sizes of one cover: %v", got, src.requests())
	}
}

func TestArtworkKeepsThePlaceholderForNothingToLoad(t *testing.T) {
	src := &fakeArt{}
	for _, s := range []string{"", "/other/path.jpg", "not a url"} {
		a := &artApp{src: s, size: 50}
		tt, _ := newArtApp(t, src, a)
		tt.Frame()
		tt.Frame()
		hover := theme.C("hover")
		if got := at(tt, 12, 12); !near(got, color.RGBA{hover.R, hover.G, hover.B, 255}) {
			t.Errorf("src %q: corner %v", s, got)
		}
	}
	if got := src.requests(); len(got) != 0 {
		t.Errorf("requests for nothing: %v", got)
	}
}

func TestArtworkAcceptsATemplate(t *testing.T) {
	src := &fakeArt{}
	a := &artApp{src: "https://is1-ssl.mzstatic.com/image/thumb/Music/y/{w}x{h}bb.jpg", size: 40}
	tt, mt := newArtApp(t, src, a)
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	if got := src.requests(); len(got) != 1 || got[0] != [2]int{40, 40} {
		t.Errorf("requests %v", got)
	}
}

func TestArtworkRetriesAFailedCoverTwiceThenLeavesIt(t *testing.T) {
	src := &fakeArt{fail: 100}
	a := &artApp{src: "/art/k1/300x300.jpg", size: 50}
	tt, mt := newArtApp(t, src, a)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	a.l.now = func() time.Time { return now }

	step := func() {
		tt.Frame()
		mt.run(t, 0)
		time.Sleep(20 * time.Millisecond) // the worker's Get
		mt.run(t, 0)
		tt.Frame()
	}
	step()
	if n := len(src.requests()); n != 1 {
		t.Fatalf("first attempt: %d requests", n)
	}
	now = now.Add(4 * time.Second)
	step()
	if n := len(src.requests()); n != 1 {
		t.Errorf("asked again after 4 s: %d requests", n)
	}
	now = now.Add(2 * time.Second) // 6 s after the failure
	step()
	if n := len(src.requests()); n != 2 {
		t.Errorf("first retry: %d requests", n)
	}
	now = now.Add(31 * time.Second)
	step()
	if n := len(src.requests()); n != 3 {
		t.Errorf("second retry: %d requests", n)
	}
	now = now.Add(time.Hour)
	step()
	if n := len(src.requests()); n != 3 {
		t.Errorf("left alone after two retries: %d requests", n)
	}
	// The placeholder stayed throughout.
	hover := theme.C("hover")
	if got := at(tt, 12, 12); !near(got, color.RGBA{hover.R, hover.G, hover.B, 255}) {
		t.Errorf("corner %v", got)
	}
}

func TestArtworkRecoversWhenARetrySucceeds(t *testing.T) {
	src := &fakeArt{fail: 1}
	a := &artApp{src: "/art/k1/300x300.jpg", size: 50}
	tt, mt := newArtApp(t, src, a)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	a.l.now = func() time.Time { return now }
	tt.Frame()
	mt.run(t, 1)
	now = now.Add(6 * time.Second)
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	if got := at(tt, 35, 35); !near(got, color.RGBA{220, 30, 30, 255}) {
		t.Errorf("after the retry: %v", got)
	}
}

func TestArtworkIsNamedOnlyWhenItMatters(t *testing.T) {
	src := &fakeArt{}
	a := &artApp{src: "", size: 50, opts: ArtOpts{A11yLabel: "Album cover"}}
	tt, _ := newArtApp(t, src, a)
	tt.Frame()
	r, ok := tt.Find("Album cover")
	if !ok || r.W != 50 || r.H != 50 {
		t.Errorf("labelled cover %v, %v", r, ok)
	}
	a.opts.A11yLabel = ""
	tt.Frame()
	if _, ok := tt.Find("Album cover"); ok {
		t.Error("a decoration is named")
	}
}

func TestArtworkAspectAndShapes(t *testing.T) {
	src := &fakeArt{}
	a := &artApp{src: "/art/k1/x.jpg", size: 160, opts: ArtOpts{Aspect: 16.0 / 9, Edge: true, Placeholder: "list-music", A11yLabel: "Video"}}
	tt, mt := newArtApp(t, src, a)
	tt.Frame()
	if r, _ := tt.Find("Video"); r.W != 160 || r.H != 90 {
		t.Errorf("16:9 box %v", r)
	}
	mt.run(t, 1)
	if got := src.requests(); len(got) != 1 || got[0] != [2]int{160, 90} {
		t.Errorf("requests %v, want 160×90", got)
	}
	a.opts = ArtOpts{Round: true, A11yLabel: "Artist"}
	a.src = ""
	tt.Frame() // the round placeholder (user glyph) builds
	if r, _ := tt.Find("Artist"); r.W != 160 || r.H != 160 {
		t.Errorf("round box %v", r)
	}
}

func TestLoaderDropsTheLeastRecentlyAskedFor(t *testing.T) {
	l := newArtLoader(&fakeArt{}, nil, 0)
	defer l.Close()
	for i := range 20 {
		cv := &cover{key: artKey{src: "/art/k", pw: i + 1, ph: i + 1}, used: uint64(i + 1), state: coverReady}
		if i == 0 {
			cv.state = coverQueued // in flight: never dropped
		}
		l.covers[cv.key] = cv
	}
	l.evict()
	if len(l.covers) != 20-4 { // a quarter of the 19 that may go
		t.Errorf("%d covers left", len(l.covers))
	}
	if l.covers[artKey{"/art/k", 1, 1}] == nil {
		t.Error("a cover in flight was dropped")
	}
	if l.covers[artKey{"/art/k", 2, 2}] != nil || l.covers[artKey{"/art/k", 5, 5}] != nil {
		t.Error("the oldest were kept")
	}
	if l.covers[artKey{"/art/k", 6, 6}] == nil {
		t.Error("a newer cover was dropped")
	}
}

func TestCoverReleasesPixelsOnceDrawn(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	bmp, err := ui.DecodeBitmap(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cv := &cover{bmp: bmp, state: coverReady}
	for range 5 {
		cv.drawn()
	}
	if cv.shown != 5 {
		t.Errorf("counted %d frames", cv.shown)
	}
	var none *cover
	if none.bitmap() != nil || (&cover{state: coverQueued}).bitmap() != nil || cv.bitmap() != bmp {
		t.Error("bitmap() answers")
	}
}

func TestCoverKeepsItselfWhenTheLoaderIsFull(t *testing.T) {
	l := newArtLoader(&fakeArt{}, nil, 1)
	defer l.Close()
	fill := func(state coverState) {
		clear(l.covers)
		for i := range maxArt {
			k := artKey{src: "old", pw: i, ph: 1}
			l.covers[k] = &cover{key: k, state: state, used: uint64(i)}
		}
	}
	// Full of covers in flight: nothing can go, and the new one stays.
	fill(coverQueued)
	cv := l.cover("new", 10, 10)
	if l.covers[cv.key] != cv {
		t.Fatal("a cover asked for was dropped as it was inserted")
	}
	// Full of idle covers: the oldest quarter goes, never the new one.
	fill(coverFailed)
	cv = l.cover("new", 10, 10)
	if l.covers[cv.key] != cv {
		t.Fatal("the new cover evicted itself")
	}
	if n := len(l.covers); n > maxArt-maxArt/4+1 {
		t.Errorf("%d covers kept after eviction", n)
	}
	if _, old := l.covers[artKey{src: "old", pw: 0, ph: 1}]; old {
		t.Error("the least recently used cover survived")
	}
}
