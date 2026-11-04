package media

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"
)

// picture is a w×h I420 picture of random planes.
func picture(w, h int, seed uint64) []byte {
	b := make([]byte, I420Len(w, h))
	r := rand.New(rand.NewPCG(seed, 1))
	for i := range b {
		b[i] = byte(r.UintN(256))
	}
	return b
}

// natural is a w×h picture with the value ranges of real video (luma 16–235,
// chroma 64–192) rather than noise over every value, which clips half the
// channels: the clamps then branch as they do on a real picture.
func natural(w, h int, seed uint64) []byte {
	b := picture(w, h, seed)
	for i := range b {
		switch {
		case i < w*h:
			b[i] = 16 + b[i]%220
		default:
			b[i] = 64 + b[i]%129
		}
	}
	return b
}

// reference is the conversion in floating point, as the React player's
// i420ToRgba did.
func reference(w, h int, planes []byte) []byte {
	cw := (w + 1) / 2
	y := planes[:w*h]
	u := planes[w*h:]
	v := planes[w*h+cw*((h+1)/2):]
	out := make([]byte, 4*w*h)
	clamp := func(f float64) byte { return byte(math.Min(255, math.Max(0, math.Round(f)))) }
	for r := range h {
		for c := range w {
			l := 1.1643836 * (float64(y[r*w+c]) - 16)
			d := float64(u[(r>>1)*cw+c>>1]) - 128
			e := float64(v[(r>>1)*cw+c>>1]) - 128
			o := 4 * (r*w + c)
			out[o] = clamp(l + 1.7927411*e)
			out[o+1] = clamp(l - 0.2132486*d - 0.5329093*e)
			out[o+2] = clamp(l + 2.1124018*d)
			out[o+3] = 255
		}
	}
	return out
}

func TestI420ToRGBAMatchesTheShader(t *testing.T) {
	// Sizes: SD, a tall one that runs in bands, odd widths and heights.
	for _, sz := range [][2]int{{864, 486}, {64, 48}, {17, 9}, {1, 1}, {3, 1}, {5, 257}, {2, 300}, {640, 359}} {
		w, h := sz[0], sz[1]
		planes := picture(w, h, uint64(w*1000+h))
		got := I420ToRGBA(nil, w, h, planes)
		if got == nil || got.Rect.Dx() != w || got.Rect.Dy() != h {
			t.Fatalf("%dx%d: %v", w, h, got)
		}
		want := reference(w, h, planes)
		worst := 0
		for i := range want {
			d := int(got.Pix[i]) - int(want[i])
			if d < 0 {
				d = -d
			}
			worst = max(worst, d)
		}
		// 16.16 fixed point against doubles: a rounding step at most.
		if worst > 1 {
			t.Errorf("%dx%d: worst channel difference %d", w, h, worst)
		}
	}
}

func TestI420ToRGBAKnownColours(t *testing.T) {
	// 2×2 pictures, each plane filled with one value.
	for _, tc := range []struct {
		name    string
		y, u, v byte
		want    [3]byte
	}{
		{"black", 16, 128, 128, [3]byte{0, 0, 0}},
		{"white", 235, 128, 128, [3]byte{255, 255, 255}},
		{"below black clamps", 0, 128, 128, [3]byte{0, 0, 0}},
		{"above white clamps", 255, 128, 128, [3]byte{255, 255, 255}},
		{"red", 63, 102, 240, [3]byte{254, 0, 0}},
	} {
		planes := []byte{tc.y, tc.y, tc.y, tc.y, tc.u, tc.v}
		img := I420ToRGBA(nil, 2, 2, planes)
		for _, p := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
			o := img.PixOffset(p[0], p[1])
			got := [3]byte{img.Pix[o], img.Pix[o+1], img.Pix[o+2]}
			if img.Pix[o+3] != 255 {
				t.Errorf("%s: alpha %d", tc.name, img.Pix[o+3])
			}
			for i := range got {
				if d := int(got[i]) - int(tc.want[i]); d < -2 || d > 2 {
					t.Errorf("%s at %v: %v, want %v", tc.name, p, got, tc.want)
					break
				}
			}
		}
	}
}

func TestI420ToRGBAReusesAndRejects(t *testing.T) {
	planes := picture(32, 24, 1)
	a := I420ToRGBA(nil, 32, 24, planes)
	if b := I420ToRGBA(a, 32, 24, planes); b != a {
		t.Error("an image of the right size was not reused")
	}
	if b := I420ToRGBA(a, 16, 12, planes[:I420Len(16, 12)]); b == a || b.Rect.Dx() != 16 {
		t.Error("an image of another size was reused")
	}
	if I420ToRGBA(nil, 32, 24, planes[:len(planes)-1]) != nil {
		t.Error("a short picture converted")
	}
	if I420ToRGBA(nil, 0, 24, planes) != nil || I420ToRGBA(nil, 32, -1, planes) != nil {
		t.Error("an empty picture converted")
	}
	// A subimage's rows are not 4×width apart: it gets a new image.
	sub := a.SubImage(image.Rect(0, 0, 16, 12)).(*image.RGBA)
	if b := I420ToRGBA(sub, 16, 12, planes[:I420Len(16, 12)]); b == sub {
		t.Error("a strided subimage was written into")
	}
}

func TestI420Len(t *testing.T) {
	for _, tc := range []struct{ w, h, n int }{{864, 486, 864*486 + 2*432*243}, {5, 3, 15 + 12}, {1, 1, 3}} {
		if got := I420Len(tc.w, tc.h); got != tc.n {
			t.Errorf("I420Len(%d, %d) = %d, want %d", tc.w, tc.h, got, tc.n)
		}
	}
}

// BenchmarkI420ToRGBA is an SD music video picture, as the child sends it.
func BenchmarkI420ToRGBA(b *testing.B) {
	for _, sz := range []struct {
		name string
		w, h int
	}{{"SD 864x486", 864, 486}, {"SD 640x480", 640, 480}, {"HD 1280x720", 1280, 720}} {
		planes := natural(sz.w, sz.h, 7)
		dst := image.NewRGBA(image.Rect(0, 0, sz.w, sz.h))
		b.Run(sz.name, func(b *testing.B) {
			b.SetBytes(int64(len(planes)))
			for b.Loop() {
				I420ToRGBA(dst, sz.w, sz.h, planes)
			}
		})
		b.Run(sz.name+" one goroutine", func(b *testing.B) {
			b.SetBytes(int64(len(planes)))
			for b.Loop() {
				i420Rows(dst.Pix, planes[:sz.w*sz.h], planes[sz.w*sz.h:], planes[sz.w*sz.h+((sz.w+1)/2)*((sz.h+1)/2):], sz.w, 0, sz.h)
			}
		})
	}
}
