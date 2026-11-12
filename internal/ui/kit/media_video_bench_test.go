package kit

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/media"
)

// BenchmarkVideoShow is the main thread's cost of showing one SD music video
// picture (864×486): what VideoFeed.show does for each frame, apart from the
// GPU's upload, which a headless bitmap has none of. With the CPU conversion
// the picture came to the main thread as RGBA, converted on the feed's
// goroutines (about half a millisecond of wall time, over several cores, in
// internal/ui/media's BenchmarkI420ToRGBA); now the planes come as the child
// sent them and the main thread interleaves the chroma for the GPU.
func BenchmarkVideoShow(b *testing.B) {
	const w, h = 864, 486
	p := media.Picture{Width: w, Height: h, Data: mediaPicture(w, h)}
	v := &VideoFeed{}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		p.Seq = uint64(i)
		v.show(p)
	}
}
