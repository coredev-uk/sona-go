//go:build e2e

package e2e

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/e2e/automation"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// TestSmoke is the app end to end: it paints, Albums lists the fixture's
// albums, an album's play button starts it, the player bar shows it
// playing, Space pauses and resumes it, the queue panel lists it and a drag
// reorders it, and a frame can be captured.
func TestSmoke(t *testing.T) {
	a := startApp(t)
	c := a.client
	albums := fixture.Albums()
	// "Recently added" lists the album added last first.
	first, second := albums[len(albums)-1], albums[len(albums)-2]

	t.Run("first paint", func(t *testing.T) {
		until(t, startTimeout, "the window", func() bool {
			ws, err := c.Windows()
			return err == nil && len(ws) > 0 && ws[0].Visible
		})
		until(t, startTimeout, `"startup: first paint" in the log`, func() bool { return strings.Contains(a.log(), "startup: first paint") })
		t.Logf("first paint %v after the process started", time.Since(a.start).Round(time.Millisecond))
	})

	t.Run("Albums lists the fixture albums", func(t *testing.T) {
		tree, err := c.Wait(startTimeout, "the Albums page", func(tr *automation.Tree) bool {
			return tr.HasText("24 albums") && tr.HasText(first.Name) && tr.HasText(second.Name)
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range []string{"Albums", "Library", first.Artist} {
			if !tree.HasText(w) {
				t.Errorf("the page does not show %q", w)
			}
		}
		// The covers come from the fixture's server through the artwork cache.
		until(t, 15*time.Second, "the covers to be fetched", func() bool { return a.covers.served() > 0 })
	})

	t.Run("play an album", func(t *testing.T) {
		if err := c.Press(startTimeout, "Play "+first.Name); err != nil {
			t.Fatal(err)
		}
		// Only the "Player" landmark counts: the album's tile, with the same
		// artist and a song title that is another album's name, is in the
		// window too.
		var bar *automation.Tree
		_, err := c.Wait(startTimeout, "the player bar playing", func(tr *automation.Tree) bool {
			bar = tr.Under("", "Player")
			_, pausable := bar.Find("button", "Pause")
			return pausable && bar.HasText(first.Song(0))
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := first.Artist + " · " + first.Name; !bar.HasText(want) {
			t.Errorf("the bar does not show %q: %v", want, bar.Labels(""))
		}
	})

	t.Run("Space pauses and resumes", func(t *testing.T) {
		// The album's play button kept the focus from the click, and Space on
		// a focused button presses it: click the page's title (plain text,
		// nothing to focus) so Space reaches the shell's shortcut.
		tree, err := c.Tree()
		if err != nil {
			t.Fatal(err)
		}
		title, ok := tree.Find("text", "24 albums")
		if !ok {
			t.Fatal("no \"24 albums\" text to click")
		}
		x, y := title.Bounds.Centre()
		if err := c.Input(automation.Click(x, y)); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Play", "Pause"} {
			if err := c.Input(automation.Key("Space")); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Wait(10*time.Second, "the "+want+" button after Space", func(tr *automation.Tree) bool {
				_, ok := tr.Find("button", want)
				return ok
			}); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("reorder the queue", func(t *testing.T) {
		// The queue lists the album from the playing song on: drag its second
		// upcoming song below the third and the order follows.
		if first.SongCount() < 3 {
			t.Fatalf("%q has %d songs: the fixture's last album has three", first.Name, first.SongCount())
		}
		if err := c.Press(startTimeout, "Queue"); err != nil {
			t.Fatal(err)
		}
		a1, a2 := first.Song(1)+", "+first.Artist, first.Song(2)+", "+first.Artist
		row := func(tr *automation.Tree, label string) (automation.Node, bool) {
			if n, ok := tr.Find("listitem", label); ok {
				return n, true
			}
			return tr.Find("listitem", "Up next: "+label)
		}
		tree, err := c.Wait(startTimeout, "the queue's rows", func(tr *automation.Tree) bool {
			_, ok1 := row(tr, a1)
			_, ok2 := row(tr, a2)
			return ok1 && ok2
		})
		if err != nil {
			t.Fatal(err)
		}
		from, _ := row(tree, a1)
		to, _ := row(tree, a2)
		fx, fy := from.Bounds.Centre()
		_, ty := to.Bounds.Centre()
		ty += to.Bounds.Height / 3 // the lower half of the row below: the gap after it
		events := []automation.Event{automation.PointerMove(fx, fy), automation.PointerDown(fx, fy)}
		for i := 1; i <= 10; i++ {
			events = append(events, automation.PointerMove(fx, fy+(ty-fy)*float64(i)/10))
		}
		if err := c.Input(append(events, automation.PointerUp(fx, ty))...); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Wait(10*time.Second, "the dragged song after the other", func(tr *automation.Tree) bool {
			n1, ok1 := row(tr, a1)
			n2, ok2 := row(tr, a2)
			return ok1 && ok2 && n2.Bounds.Y < n1.Bounds.Y
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("capture", func(t *testing.T) {
		if err := c.Idle(5 * time.Second); err != nil {
			t.Log("idle:", err)
		}
		img, data, err := c.Capture()
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		if b.Dx() < 800 || b.Dy() < 600 {
			t.Errorf("the capture is %v, smaller than the window", b.Size())
		}
		if distinctColours(img) < 200 {
			t.Errorf("the capture has few colours: it is blank or covers are missing")
		}
		if err := os.WriteFile(filepath.Join(a.out, "smoke.png"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("quits on SIGTERM", func(t *testing.T) {
		a.quit()
		if !a.cmd.ProcessState.Success() {
			t.Errorf("the app ended with %v", a.cmd.ProcessState)
		}
	})
}

// distinctColours counts the colours of a coarse sample of the picture.
func distinctColours(img image.Image) int {
	seen := map[[3]uint32]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			r, g, bl, _ := img.At(x, y).RGBA()
			seen[[3]uint32{r >> 8, g >> 8, bl >> 8}] = true
		}
	}
	return len(seen)
}
