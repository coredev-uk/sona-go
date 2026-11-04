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
// playing, Space pauses and resumes it, and a frame can be captured.
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
		tree, err := c.Wait(startTimeout, "the player bar playing", func(tr *automation.Tree) bool {
			_, pausable := tr.Find("button", "Pause")
			return pausable && tr.HasText(first.Song(0))
		})
		if err != nil {
			t.Fatal(err)
		}
		if !tree.HasText(first.Artist+" · "+first.Name) && !tree.HasText(first.Artist) {
			t.Errorf("the bar does not name the artist: %v", tree.Labels(""))
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
