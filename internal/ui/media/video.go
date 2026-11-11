package media

import "github.com/egoist/mygo/ui"

// I420Len is the size in bytes of a w×h I420 picture: the luma plane, then
// the two chroma planes (U, then V) of ⌈w/2⌉×⌈h/2⌉, tightly packed, as the
// video surface (internal/playback/video) keeps it.
func I420Len(w, h int) int {
	return w*h + 2*((w+1)/2)*((h+1)/2)
}

// validI420 reports whether data holds a w×h I420 picture: ui.Bitmap.UpdateYUV
// panics on planes that are too short, so nothing reaches it that is not.
func validI420(w, h int, data []byte) bool {
	return w > 0 && h > 0 && len(data) >= I420Len(w, h)
}

// YUV is the picture as the image ui.Bitmap.UpdateYUV shows, with no
// conversion: BT.709, limited range (Y 16–235, Cb and Cr 16–240), what the
// CDM's decoder gives for SD music videos and what the React player's shader
// assumed. It holds data's planes as they are (it is not copied, and the child's
// frames are not changed once shown), and it is nil for a picture that
// UpdateYUV would panic on.
func (p Picture) YUV() *ui.YUVImage {
	if !validI420(p.Width, p.Height, p.Data) {
		return nil
	}
	w, h := p.Width, p.Height
	cw, ch := (w+1)/2, (h+1)/2
	return &ui.YUVImage{
		Format: ui.I420,
		Width:  w, Height: h,
		Planes: [3][]byte{
			p.Data[:w*h],
			p.Data[w*h : w*h+cw*ch],
			p.Data[w*h+cw*ch : w*h+2*cw*ch],
		},
		Strides: [3]int{w, cw, cw},
		Matrix:  ui.BT709,
	}
}
