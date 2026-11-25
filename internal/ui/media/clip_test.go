package media

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// gifDecoder is a decoder of a GIF of two 8×8 frames, red then blue, 100 ms
// each, that counts its closes.
func gifDecoder(t *testing.T, closed *atomic.Int32) ui.VideoDecoder {
	t.Helper()
	g := &gif.GIF{LoopCount: 0}
	for _, c := range []color.RGBA{{200, 30, 30, 255}, {30, 30, 200, 255}} {
		img := image.NewPaletted(image.Rect(0, 0, 8, 8), palette.Plan9)
		for i := range img.Pix {
			img.Pix[i] = uint8(img.Palette.Index(c))
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	d, err := ui.NewGIFDecoder(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return closing{d, closed}
}

type closing struct {
	ui.VideoDecoder
	closed *atomic.Int32
}

func (d closing) Close() error {
	d.closed.Add(1)
	if c, ok := d.VideoDecoder.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// clipView is a Tester showing k, 16×16, and how many times it built the view.
func clipView(k *Clip) (*ui.Tester, *int) {
	builds := new(int)
	tt := ui.NewTester(func(c *ui.Context) {
		*builds++
		ui.Box(c).Size(16, 16).Draw(func(pt *ui.Painter, r ui.Rect) { k.Paint(pt, r) })
	}, 16, 16)
	return tt, builds
}

// centre is whether the picture is red (true) or blue (false), and whether it
// is either.
func centre(tt *ui.Tester) (red, ok bool) {
	c := tt.Image().RGBAAt(8, 8)
	switch {
	case c.R > 150 && c.B < 60:
		return true, true
	case c.B > 150 && c.R < 60:
		return false, true
	}
	return false, false
}

// step runs a second of the Tester's time in refreshes, a moment of real time
// each for the decoder, and returns how many refreshes showed red and blue.
func step(tt *ui.Tester, d time.Duration) (red, blue int) {
	for range int(d / (16700 * time.Microsecond)) {
		tt.Advance(16700 * time.Microsecond)
		time.Sleep(time.Millisecond)
		if r, ok := centre(tt); ok {
			if r {
				red++
			} else {
				blue++
			}
		}
	}
	return red, blue
}

func TestClipPaintsFramesAtTheirOwnTimesWithoutBuilding(t *testing.T) {
	var closed atomic.Int32
	k, err := NewClip(gifDecoder(t, &closed))
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if !k.Ready() || !k.Playing() {
		t.Fatalf("ready %v, playing %v, want both from the start", k.Ready(), k.Playing())
	}
	if w, h := k.Size(); w != 8 || h != 8 {
		t.Errorf("size %d×%d, want 8×8", w, h)
	}
	tt, builds := clipView(k)
	if red, _ := centre(tt); !red {
		t.Fatal("the first frame is not on show")
	}
	built := *builds
	// A second is five red and five blue frames of 100 ms, looping, though
	// the GIF has two.
	red, blue := step(tt, time.Second)
	if red < 3 || blue < 3 {
		t.Errorf("a second showed red for %d refreshes and blue for %d: it does not loop", red, blue)
	}
	if n := *builds - built; n != 0 {
		t.Errorf("%d builds of the view in a second of video", n)
	}
}

func TestClipStandsStillWhilePausedAndGoesOnFromTheFrameItHeld(t *testing.T) {
	var closed atomic.Int32
	k, err := NewClip(gifDecoder(t, &closed))
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	tt, _ := clipView(k)
	step(tt, 250*time.Millisecond)
	k.Pause()
	if k.Playing() {
		t.Error("a paused clip plays")
	}
	tt.Frame()
	first, _ := centre(tt)
	red, blue := step(tt, time.Second)
	if (first && blue != 0) || (!first && red != 0) {
		t.Errorf("the picture moved while paused: red %d, blue %d", red, blue)
	}
	k.Play()
	tt.Frame()
	if red, blue = step(tt, time.Second); red < 3 || blue < 3 {
		t.Errorf("after Play a second showed red for %d refreshes and blue for %d", red, blue)
	}
}

func TestClipClosesItsDecoderOnceAndStops(t *testing.T) {
	var closed atomic.Int32
	k, err := NewClip(gifDecoder(t, &closed))
	if err != nil {
		t.Fatal(err)
	}
	tt, _ := clipView(k)
	step(tt, 100*time.Millisecond) // it decodes ahead
	k.Close()
	k.Close()
	if k.Playing() {
		t.Error("a closed clip plays")
	}
	for end := time.Now().Add(hangGuard); closed.Load() == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the decoder was not closed")
		}
	}
	time.Sleep(10 * time.Millisecond)
	if n := closed.Load(); n != 1 {
		t.Errorf("the decoder closed %d times, want once", n)
	}
}

// failingDecoder is a decoder whose frames run out in an error after one.
type failingDecoder struct{ n int }

func (d *failingDecoder) Next() (ui.VideoFrame, error) {
	d.n++
	if d.n > 1 {
		return ui.VideoFrame{}, io.ErrUnexpectedEOF
	}
	return ui.VideoFrame{Image: image.NewRGBA(image.Rect(0, 0, 4, 4)), Duration: 50 * time.Millisecond}, nil
}

func (d *failingDecoder) Rewind() error { return nil }

func TestClipReportsAFailedFrameAndKeepsThePicture(t *testing.T) {
	k, err := NewClip(&failingDecoder{})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	tt, _ := clipView(k)
	for range 40 {
		tt.Advance(16700 * time.Microsecond)
		time.Sleep(time.Millisecond)
	}
	if k.Err() == nil || k.Playing() {
		t.Errorf("err %v, playing %v after the decoder failed", k.Err(), k.Playing())
	}
}

func TestNewClipOfAVideoOfNoFramesFails(t *testing.T) {
	if _, err := NewClip(&emptyDecoder{}); err == nil {
		t.Error("a clip of no frames opened")
	}
}

type emptyDecoder struct{}

func (*emptyDecoder) Next() (ui.VideoFrame, error) { return ui.VideoFrame{}, io.EOF }
func (*emptyDecoder) Rewind() error                { return nil }
