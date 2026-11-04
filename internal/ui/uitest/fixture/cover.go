package fixture

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"sync"
)

// Cover is the generated cover of seed as a PNG of w×h pixels: a diagonal
// gradient between two hues that follow the seed, with one shape on it. It
// is a pure function of its arguments.
func Cover(seed, w, h int) []byte {
	w, h = max(w, 1), max(h, 1)
	c0, c1, ink := CoverColors(seed)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)*(0.3+0.4*float64((seed*7)%5)/4), float64(h)*(0.3+0.4*float64((seed*3)%5)/4)
	r := float64(min(w, h)) * (0.18 + 0.05*float64(seed%4))
	for y := range h {
		for x := range w {
			t := (float64(x)/float64(w) + float64(y)/float64(h)) / 2
			px := mix(c0, c1, t)
			var inside bool
			switch seed % 3 {
			case 0: // a disc
				inside = math.Hypot(float64(x)-cx, float64(y)-cy) < r
			case 1: // a bar across the lower third
				inside = float64(y) > float64(h)*0.62 && float64(y) < float64(h)*0.62+r*0.55
			default: // a ring
				d := math.Hypot(float64(x)-cx, float64(y)-cy)
				inside = d < r && d > r*0.6
			}
			if inside {
				px = mix(px, ink, 0.85)
			}
			img.SetNRGBA(x, y, px)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// CoverColors are the two gradient colours and the shape's colour of a
// seed's cover.
func CoverColors(seed int) (c0, c1, ink color.NRGBA) {
	hue := float64((seed*47 + 13) % 360)
	return hsl(hue, 0.55, 0.42), hsl(math.Mod(hue+40, 360), 0.6, 0.62), hsl(math.Mod(hue+180, 360), 0.5, 0.92)
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// hsl is the colour of hue (degrees), saturation and lightness (0..1).
func hsl(h, s, l float64) color.NRGBA {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return color.NRGBA{to(r), to(g), to(b), 255}
}

// coverPath matches the path of a Template filled in by the artwork cache:
// /image/thumb/Fixture/cover<n>/<w>x<h>bb.png (playlists: playlist<n>).
var coverPath = regexp.MustCompile(`^/image/thumb/Fixture/(?:cover|playlist)(\d+)/(\d+)x(\d+)bb\.png$`)

// CDN is Apple's artwork CDN as the fixture sees it: an http.RoundTripper
// answering the paths of Template and PlaylistTemplate with the generated
// cover, and every other request with 404. It counts what it served.
type CDN struct {
	mu   sync.Mutex
	hits int
}

// Hits is how many covers CDN has served.
func (c *CDN) Hits() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits
}

// RoundTrip implements http.RoundTripper.
func (c *CDN) RoundTrip(req *http.Request) (*http.Response, error) {
	if b, ok := ServeCover(req.URL.Path); ok {
		c.mu.Lock()
		c.hits++
		c.mu.Unlock()
		return &http.Response{
			Status: "200 OK", StatusCode: http.StatusOK, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Request: req,
			Header:        http.Header{"Content-Type": {"image/png"}, "Content-Length": {strconv.Itoa(len(b))}},
			Body:          io.NopCloser(bytes.NewReader(b)),
			ContentLength: int64(len(b)),
		}, nil
	}
	return &http.Response{
		Status: "404 Not Found", StatusCode: http.StatusNotFound, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Request: req,
		Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

// Handler serves covers over HTTP (the e2e smoke's local CDN).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := ServeCover(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(b)
	})
}

// ServeCover is the PNG a Template's filled path names.
func ServeCover(path string) ([]byte, bool) {
	m := coverPath.FindStringSubmatch(path)
	if m == nil {
		return nil, false
	}
	n, _ := strconv.Atoi(m[1])
	w, _ := strconv.Atoi(m[2])
	h, _ := strconv.Atoi(m[3])
	if w > 2048 || h > 2048 {
		return nil, false
	}
	if path[len("/image/thumb/Fixture/")] == 'p' {
		n += 1000 // playlists have colours of their own
	}
	return Cover(n, w, h), true
}
