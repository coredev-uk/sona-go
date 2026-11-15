package song

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the song page does
// today for each gesture and each kind. The rule being reviewed: choosing a
// result opens it; Play plays; a track list's row plays (double-click or
// Enter); a modifier never does something a visible cue does not hint at. Where
// today's behaviour departs from it, the assertion carries a "DEPARTS:"
// comment, to be found (and flipped) when the behaviour is fixed.

// iaGest is what each gesture on one control did, as fx words it.
type iaGest struct {
	click, dbl, enter, shiftEnter, space, right string
}

// iaWant fails for each gesture whose effect is not the one pinned.
func iaWant(t *testing.T, what string, got, want iaGest) {
	t.Helper()
	for _, c := range []struct{ gesture, got, want string }{
		{"click", got.click, want.click},
		{"double-click", got.dbl, want.dbl},
		{"Enter", got.enter, want.enter},
		{"Shift+Enter", got.shiftEnter, want.shiftEnter},
		{"Space", got.space, want.space},
		{"right-click", got.right, want.right},
	} {
		if c.got != c.want {
			t.Errorf("%s, %s: %s, want %s", what, c.gesture, c.got, c.want)
		}
	}
}

// iaJoin is several effects in the order they happened.
func iaJoin(fx ...string) string { return strings.Join(fx, "; ") }

// iaPage drives the gestures of the song page from a clean start and says what
// each did: a page opened, a play of songs through the queue, a menu opened.
type iaPage struct{ r *rig }

const iaPath = "/song/1440"

func (p iaPage) fx() string {
	for range 12 {
		p.r.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	if loc := p.r.route(); loc != iaPath {
		out = append(out, "nav "+loc)
	}
	for _, pl := range p.r.queue.played() {
		ids := make([]string, len(pl.refs))
		for i, ref := range pl.refs {
			ids[i] = ref.ID
		}
		out = append(out, "play ["+strings.Join(ids, " ")+"] start="+string(rune('0'+pl.start))+" from="+pl.from.Kind+":"+pl.from.ID)
	}
	if n := len(access.Read(p.r.t, p.r.tt.Tester).Roles(ui.RoleMenuItem)); n > 0 {
		out = append(out, "menu")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

// reset closes what a gesture opened, goes back to the page (which a page that
// was opened has rebuilt: it waits for it to be laid out again) and forgets
// what was played.
func (p iaPage) reset() {
	p.r.t.Helper()
	p.r.tt.Key(0, ui.KeyEscape)
	left := p.r.route() != iaPath
	p.r.router.Push(iaPath)
	p.clear()
	if left {
		p.r.tt.Until("the page", func() bool { return p.r.route() == iaPath && p.r.tt.HasText("Airbag") && p.r.tt.HasText("Radiohead") })
		p.top()
	}
	p.r.tt.Frame()
}

// clear forgets what a gesture did, without leaving the page.
func (p iaPage) clear() {
	p.r.queue.mu.Lock()
	p.r.queue.plays = nil
	p.r.queue.mu.Unlock()
}

// top scrolls the page to its top (Tab scrolls to the control it reaches).
func (p iaPage) top() {
	p.r.tt.Scroll(600, 400, 0, -10000)
	p.r.tt.Settle()
}

// tab presses Tab until the control called label has the focus, which the
// accessibility tree names.
func (p iaPage) tab(label string) {
	p.r.t.Helper()
	for range 80 {
		if tr := access.Read(p.r.t, p.r.tt.Tester); tr.Focus != nil && tr.Focus.Label == label {
			return
		}
		p.r.tt.Key(0, ui.KeyTab)
	}
	p.r.t.Fatalf("Tab never reached %q", label)
}

// gestures runs each gesture on the control called label.
func (p iaPage) gestures(label string) iaGest {
	p.r.t.Helper()
	at := func() (float32, float32) {
		r, ok := p.r.tt.Find(label)
		if !ok {
			p.r.t.Fatalf("%q is not on screen: %q", label, p.r.tt.Texts())
		}
		return r.X + r.W/2, r.Y + r.H/2
	}
	var g iaGest
	p.reset()
	x, y := at()
	p.r.tt.ClickAt(x, y)
	g.click = p.fx()
	p.reset()
	x, y = at()
	p.r.tt.ClickAt(x, y)
	p.r.tt.ClickAt(x, y)
	g.dbl = p.fx()
	// The keys, on one focus: Enter goes last, for it may open a page.
	p.reset()
	p.tab(label)
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
		into *string
	}{{ui.Shift, ui.KeyEnter, &g.shiftEnter}, {0, ui.KeySpace, &g.space}, {0, ui.KeyEnter, &g.enter}} {
		p.clear()
		p.r.tt.Key(k.mods, k.key)
		*k.into = p.fx()
	}
	p.reset()
	p.top()
	x, y = at()
	p.r.tt.RightClickAt(x, y)
	g.right = p.fx()
	p.reset()
	return g
}

// The head's Play plays this one song and nothing else: the queue becomes the
// song alone (not its album), labelled with its ID. There is no Shuffle. It
// never opens anything, and no modifier or right-click does anything.
func TestInteractionSongHeaderPlay(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	const play = "play [1440] start=0 from=:1440"
	iaWant(t, "Play", iaPage{r}.gestures("Play Airbag"), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
	if r.tt.HasText("Shuffle") {
		t.Error("the song page offers Shuffle")
	}
}

// A song that cannot play has its Play disabled: nothing plays on any gesture.
func TestInteractionSongHeaderPlayUnplayable(t *testing.T) {
	p := airbag()
	p.Song.Playable = false
	r := newRig(t, 1132, 1000, p)
	r.open()
	pg := iaPage{r}
	pg.reset()
	x, y := func() (float32, float32) { b, _ := r.tt.Find("Play Airbag"); return b.X + b.W/2, b.Y + b.H/2 }()
	r.tt.ClickAt(x, y)
	r.tt.ClickAt(x, y)
	if got := pg.fx(); got != "nothing" {
		t.Errorf("a double-click on a disabled Play did %s", got)
	}
}

// The byline's artist and the album under it are links to their pages that open
// on a click, a double-click (the router keeps one entry) and Enter, where Sona
// has the page; the album's link marks this song on the album's page. Space and
// Shift+Enter do nothing.
func TestInteractionSongBylineLinks(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	for _, c := range []struct{ label, loc string }{
		{"Radiohead", "/artist/a1"},
		{"OK Computer", "/album/al1?song=1440"},
	} {
		if info, ok := r.tt.Access(c.label); !ok || info.Role != ui.RoleLink {
			t.Errorf("%s: role %v (found %v), want a link", c.label, info.Role, ok)
		}
		to := "nav " + c.loc
		iaWant(t, c.label, iaPage{r}.gestures(c.label), iaGest{
			click: to, dbl: to, enter: to, shiftEnter: "nothing", space: "nothing", right: "nothing",
		})
	}
}
