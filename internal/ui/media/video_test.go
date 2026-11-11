package media

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// picture is a w×h I420 picture whose every byte is its index mod 251, so a
// plane that starts in the wrong place shows.
func picture(w, h int) []byte {
	b := make([]byte, I420Len(w, h))
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestI420Len(t *testing.T) {
	for _, tc := range []struct{ w, h, n int }{{864, 486, 864*486 + 2*432*243}, {5, 3, 15 + 12}, {1, 1, 3}} {
		if got := I420Len(tc.w, tc.h); got != tc.n {
			t.Errorf("I420Len(%d, %d) = %d, want %d", tc.w, tc.h, got, tc.n)
		}
	}
}

// The picture the bitmap is given is the child's frame as it is: I420, BT.709,
// limited range, each plane where the surface keeps it.
func TestPictureYUV(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{864, 486}, {64, 48}, {5, 3}, {1, 1}} {
		data := picture(sz.w, sz.h)
		img := Picture{Width: sz.w, Height: sz.h, Data: data}.YUV()
		if img == nil {
			t.Fatalf("%d×%d: no image", sz.w, sz.h)
		}
		cw, ch := (sz.w+1)/2, (sz.h+1)/2
		if img.Format != ui.I420 || img.Matrix != ui.BT709 || img.FullRange {
			t.Errorf("%d×%d: %v, matrix %v, full range %v; want I420, BT709, limited", sz.w, sz.h, img.Format, img.Matrix, img.FullRange)
		}
		if img.Width != sz.w || img.Height != sz.h {
			t.Errorf("%d×%d: the image is %d×%d", sz.w, sz.h, img.Width, img.Height)
		}
		if img.Strides != [3]int{sz.w, cw, cw} {
			t.Errorf("%d×%d: strides %v", sz.w, sz.h, img.Strides)
		}
		for i, want := range []struct{ off, n int }{{0, sz.w * sz.h}, {sz.w * sz.h, cw * ch}, {sz.w*sz.h + cw*ch, cw * ch}} {
			if len(img.Planes[i]) != want.n || &img.Planes[i][0] != &data[want.off] {
				t.Errorf("%d×%d: plane %d is not the %d bytes at %d of the frame", sz.w, sz.h, i, want.n, want.off)
			}
		}
		// Whole pictures, odd sizes among them, are what UpdateYUV accepts.
		ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1))).UpdateYUV(img)
	}
}

// UpdateYUV panics on a plane too short, so a malformed frame has no image.
func TestPictureYUVRejectsMalformed(t *testing.T) {
	data := picture(32, 24)
	for name, p := range map[string]Picture{
		"short":    {Width: 32, Height: 24, Data: data[:len(data)-1]},
		"empty":    {Width: 32, Height: 24},
		"no width": {Width: 0, Height: 24, Data: data},
		"negative": {Width: 32, Height: -1, Data: data},
		"too big":  {Width: 64, Height: 48, Data: data},
	} {
		if p.YUV() != nil {
			t.Errorf("%s: a malformed picture has an image", name)
		}
	}
	// More bytes than the picture needs are fine: the planes are its first.
	if (Picture{Width: 32, Height: 24, Data: append(data, 0)}).YUV() == nil {
		t.Error("a frame with trailing bytes has no image")
	}
}
