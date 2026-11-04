package artist

import (
	"image"
	"testing"
	"time"
)

// inkOf is how far the darkest pixel of the box showing text (a release row:
// its cover and title) is from the page's ground, 0 (as pale as the ground)
// to 1 (as dark as can be): it halves where the row is drawn at half opacity.
func (r *rig) inkOf(text string) float64 {
	r.t.Helper()
	b, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no %q on screen; texts %q", text, r.tt.Texts())
	}
	img := r.tt.Image()
	luma := func(x, y int) float64 {
		c := img.RGBAAt(x, y)
		return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
	}
	rect := image.Rect(int(b.X), int(b.Y), int(b.X+b.W), int(b.Y+b.H)).Intersect(img.Bounds())
	if rect.Empty() {
		r.t.Fatalf("%q is off screen: %v", text, b)
	}
	ground := luma(rect.Max.X-1, rect.Max.Y-1)
	darkest := ground
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			darkest = min(darkest, luma(x, y))
		}
	}
	return ground - darkest
}

// hover rests the pointer on the text for long enough for a tooltip.
func (r *rig) hover(text string) {
	r.t.Helper()
	b, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no %q on screen; texts %q", text, r.tt.Texts())
	}
	r.tt.Move(b.X+b.W/2, b.Y+b.H/2)
	r.tt.Advance(2 * time.Second)
}

const withheldReason = "Explicit content is restricted on this account"

// An explicit release on a restricted account is dimmed to half and says why
// on hover (v0.2.0's withheld row); a release that may be played is not.
func TestWithheldReleaseIsHalfDimAndExplainsWhyOnHover(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 1200, func(r *rig) {
		r.cat.page = radiohead()
		r.cat.page.Shelves[0].Albums[1].Explicit = true // Kid A
		r.auth.restricted = true
	})
	r.shows("OK Computer", "Kid A")
	r.tt.Settle()
	if r.tt.HasText(withheldReason) {
		t.Error("the reason shows before the pointer rests on the row")
	}
	played, withheld := r.inkOf("OK Computer"), r.inkOf("Kid A")
	if ratio := withheld / played; ratio < 0.45 || ratio > 0.55 {
		t.Errorf("the withheld row is drawn at %.2f of the playable one's ink (%.3f of %.3f), want half", ratio, withheld, played)
	}
	r.hover("Kid A")
	if !r.tt.HasText(withheldReason) {
		t.Errorf("no tooltip %q on the withheld release: %q", withheldReason, r.tt.Texts())
	}
	r.hover("OK Computer")
	if r.tt.HasText(withheldReason) {
		t.Error("a playable release says it is withheld")
	}
}

// The same explicit release on an account without the restriction is a row
// like any other.
func TestExplicitReleaseIsPlayableWhereNothingIsRestricted(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 1200, func(r *rig) {
		r.cat.page = radiohead()
		r.cat.page.Shelves[0].Albums[1].Explicit = true // Kid A
	})
	r.shows("OK Computer", "Kid A")
	r.tt.Settle()
	played, explicit := r.inkOf("OK Computer"), r.inkOf("Kid A")
	if ratio := explicit / played; ratio < 0.9 || ratio > 1.1 {
		t.Errorf("an explicit row is drawn at %.2f of a clean one's ink (%.3f of %.3f) on an account that allows it", ratio, explicit, played)
	}
	r.hover("Kid A")
	if r.tt.HasText(withheldReason) {
		t.Error("a release that plays says it is withheld")
	}
}
