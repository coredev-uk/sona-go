package player

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Going up (the sung line advances), the lines the sung one leads (itself and
// the ones above) start together and the ones below follow, each a step
// later than the one before.
func TestTheCascadeWaitGrowsWithTheDistanceBehindTheSungLine(t *testing.T) {
	for _, c := range []struct {
		rank int
		want time.Duration
	}{
		{-3, 0},
		{-2, 0},
		{-1, 0},
		{0, 0},
		{1, 1 * cascadeStep},
		{2, 2 * cascadeStep},
		{3, 3 * cascadeStep},
		{4, 4 * cascadeStep},
	} {
		if got := cascadeDelay(c.rank, true, false); got != c.want {
			t.Errorf("rank %d going up waits %v, want %v", c.rank, got, c.want)
		}
	}
	if cascadeStep != 60*time.Millisecond {
		t.Errorf("a step is %v: the tests and the docs say 60 ms", cascadeStep)
	}
}

// The delays are capped, so that the far lines of a tall sheet start with the
// last of the steps and do not lag.
func TestTheCascadeWaitIsCapped(t *testing.T) {
	limit := cascadeSteps * cascadeStep
	for _, up := range []bool{true, false} {
		for rank := -40; rank <= 40; rank++ {
			if got := cascadeDelay(rank, up, false); got > limit {
				t.Errorf("rank %d (up %v) waits %v, past the cap %v", rank, up, got, limit)
			}
		}
	}
	if got := cascadeDelay(25, true, false); got != limit {
		t.Errorf("a line 25 places behind waits %v, want the cap %v", got, limit)
	}
	if got := cascadeDelay(-25, false, false); got != limit {
		t.Errorf("a line 25 places behind a seek back waits %v, want the cap %v", got, limit)
	}
	// Nothing comes after the last step: the whole cascade settles within
	// the cap, one glide and a margin for the frame that finishes it.
	if want := limit + theme.Current().Motion["climb"].Duration + cascadeMargin; cascadeSettle() != want {
		t.Errorf("the cascade settles in %v, want %v", cascadeSettle(), want)
	}
}

// On a seek back the way is mirrored: the lines above trail the sung one and
// the lines below go with it, so that the gap between two lines still only
// opens.
func TestTheCascadeGoesDownOnASeekBack(t *testing.T) {
	for _, c := range []struct {
		rank int
		want time.Duration
	}{
		{3, 0},
		{2, 0},
		{1, 0},
		{0, 0},
		{-1, 1 * cascadeStep},
		{-2, 2 * cascadeStep},
		{-3, 3 * cascadeStep},
		{-4, 4 * cascadeStep},
	} {
		if got := cascadeDelay(c.rank, false, false); got != c.want {
			t.Errorf("rank %d going down waits %v, want %v", c.rank, got, c.want)
		}
	}
	// Mirrored: a rank up and its opposite down wait the same.
	for rank := 0; rank <= 6; rank++ {
		if up, down := cascadeDelay(rank, true, false), cascadeDelay(-rank, false, false); up != down {
			t.Errorf("rank %d up waits %v but %d down %v", rank, up, -rank, down)
		}
	}
}

// Under reduced motion lines jump: there is no wait at all, whichever way.
func TestTheCascadeHasNoWaitUnderReducedMotion(t *testing.T) {
	for _, up := range []bool{true, false} {
		for rank := -8; rank <= 8; rank++ {
			if got := cascadeDelay(rank, up, true); got != 0 {
				t.Errorf("rank %d (up %v) waits %v under reduced motion", rank, up, got)
			}
			if got := cascadeAnim(rank, up, true).Delay; got != 0 {
				t.Errorf("rank %d (up %v) animates after %v under reduced motion", rank, up, got)
			}
		}
	}
}

// A line glides over the climb token's time, in the token's curve, after its
// wait; nothing makes the wait negative.
func TestACascadeAnimationIsTheClimbTokenAfterItsWait(t *testing.T) {
	m := theme.Current().Motion["climb"]
	if m.Duration < 500*time.Millisecond || m.Duration > 700*time.Millisecond {
		t.Fatalf("the climb token is %v: a glide is 500 to 700 ms", m.Duration)
	}
	a := cascadeAnim(2, true, false)
	if a.Duration != m.Duration || a.Delay != 2*cascadeStep || a.Ease == nil {
		t.Errorf("a line two places behind animates %+v, want %v after %v in the token's curve", a, m.Duration, 2*cascadeStep)
	}
	if a.Spring.Bounce != 0 {
		t.Errorf("the glide bounces: %+v", a.Spring)
	}
	if lead := cascadeAnim(0, true, false); lead.Delay != 0 || lead.Duration != m.Duration {
		t.Errorf("the sung line animates %+v: it goes first, at once", lead)
	}
}
