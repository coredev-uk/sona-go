package measure

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"time"
)

// A tile's cover, until it has decoded, is a flat square with a glyph on
// it; a loaded cover is a picture. coversLoaded looks at the pixels of a
// frame the window captured: at the top left of each fully visible tile,
// inside its cover, a picture has far more distinct colours than the
// placeholder's glyph and its antialiasing.
const (
	// probeW and probeH are the sampled area in DIPs, and probeColours how
	// many distinct colours in it make a picture.
	probeW, probeH = 80, 80
	probeColours   = 100
	// gradientColours and flatShare: the lower bar a gradient cover passes.
	gradientColours = 20
	flatShare       = 0.6
	// loadedShare is how many of the visible covers must be pictures: a
	// cover that is itself a flat colour does not hold the probe back.
	loadedShare = 0.8
)

// coversLoaded tells whether the covers of the tiles in view are drawn,
// from a capture of the window.
func (s *scenario) coversLoaded() (bool, error) {
	nodes, err := s.Client.Tree()
	if err != nil {
		return false, err
	}
	tiles, page := Tiles(nodes)
	data, err := s.Client.Capture()
	if err != nil {
		return false, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	scale := float64(img.Bounds().Dx()) / s.win.Width
	seen, loaded := 0, 0
	for _, t := range tiles {
		b := t.Bounds
		// The tile's top left is the cover's: the sample is the 80 DIPs
		// from 8 DIPs in, clear of the cover's rounded corner.
		if b.Y < page.Bounds.Y || b.Y+b.Height > page.Bounds.Y+page.Bounds.Height {
			continue
		}
		seen++
		r := image.Rect(int((b.X+8)*scale), int((b.Y+8)*scale), int((b.X+8+probeW)*scale), int((b.Y+8+probeH)*scale)).Intersect(img.Bounds())
		if isPicture(img, r) {
			loaded++
		}
	}
	if seen == 0 {
		return false, errors.New("no album tile is fully in view")
	}
	return float64(loaded) >= loadedShare*float64(seen), nil
}

// isPicture tells whether r of img holds a picture rather than the
// placeholder: far more than probeColours distinct colours, or (a smooth
// gradient has few steps in 80 DIPs) some colours and no one colour over
// flatShare of the area, which a flat square with a small glyph on it has.
func isPicture(img image.Image, r image.Rectangle) bool {
	count := map[[3]uint32]int{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			count[[3]uint32{cr >> 8, cg >> 8, cb >> 8}]++
		}
	}
	if len(count) >= probeColours {
		return true
	}
	most := 0
	for _, n := range count {
		most = max(most, n)
	}
	area := r.Dx() * r.Dy()
	return len(count) >= gradientColours && area > 0 && float64(most) < flatShare*float64(area)
}

// waitCovers waits until the covers in view are drawn and returns the first
// frame's time and the time of the last frame drawn before that moment: the
// frames are what the start-up line is read from. ok is false after
// timeout, or when the window cannot be captured (the covers then count as
// on screen when the frames stop).
func (s *scenario) waitCovers(timeout time.Duration) (first, last time.Time, ok bool) {
	deadline := s.Now().Add(timeout)
	for {
		loaded, err := s.coversLoaded()
		if err != nil {
			s.logf("warning: cannot tell when the covers are drawn (%v): taking the end of the frames", err)
			return s.waitQuiet(s.Quiet, timeout)
		}
		now := s.Now()
		if loaded {
			if n := s.Frames.Count(); n > 0 {
				fs := s.Frames.Since(0)
				first, last = fs[0].At, fs[0].At
				for _, f := range fs {
					if !f.At.After(now) {
						last = f.At
					}
				}
			}
			return first, last, true
		}
		if now.After(deadline) {
			return first, last, false
		}
		s.Sleep(50 * time.Millisecond)
	}
}
