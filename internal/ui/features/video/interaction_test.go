package video

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the music video page does
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

// iaPage drives the gestures of the music video page from a clean start and
// says what each did: a page opened, a video or a song played, a link handed to
// Apple Music, a menu opened.
type iaPage struct{ p *pageRig }

const iaPath = "/music-video/mv.1"

func (g iaPage) fx() string {
	for range 12 {
		g.p.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	if path := g.p.router.Path(); path != iaPath {
		out = append(out, "nav "+path)
	}
	for i, path := range g.p.nav.Pushed {
		// The router keeps one entry for the same page pushed twice in a row; the
		// rig's navigation records every push.
		if i == 0 || g.p.nav.Pushed[i-1] != path {
			out = append(out, "nav "+path)
		}
	}
	for _, v := range g.p.queue.videos {
		out = append(out, "play video "+v[0].ID)
	}
	for i, s := range g.p.queue.songs {
		out = append(out, "play song "+s[0].ID+" from="+g.p.queue.from[i].Kind+":"+g.p.queue.from[i].ID)
	}
	g.p.cat.mu.Lock()
	for _, url := range g.p.cat.opened {
		out = append(out, "open "+url)
	}
	g.p.cat.mu.Unlock()
	if n := len(access.Read(g.p.t, g.p.tt.Tester).Roles(ui.RoleMenuItem)); n > 0 {
		out = append(out, "menu")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

// clear forgets what a gesture did, without leaving the page.
func (g iaPage) clear() {
	g.p.nav.Pushed = nil
	g.p.queue.videos, g.p.queue.songs, g.p.queue.from, g.p.queue.opened = nil, nil, nil, nil
	g.p.cat.set(func(f *fakeCat) { f.opened = nil })
}

// top scrolls the page to its top (Tab scrolls to the control it reaches).
func (g iaPage) top() {
	g.p.tt.Scroll(600, 400, 0, -10000)
	g.p.tt.Settle()
}

// reset closes what a gesture opened and forgets what it did. A page that was
// opened is built again: wait for it to be laid out, at its top.
func (g iaPage) reset() {
	g.p.t.Helper()
	g.p.tt.Key(0, ui.KeyEscape)
	g.p.env.Video.Close()
	left := g.p.router.Path() != iaPath
	g.p.router.Push(iaPath)
	g.clear()
	if left {
		g.p.tt.Until("the page", func() bool {
			return g.p.router.Path() == iaPath && g.p.tt.HasText("Sky Video") && g.p.tt.HasText("Notes")
		})
		g.top()
	}
	g.p.tt.Frame()
}

// tab presses Tab until the control called label has the focus, which the
// accessibility tree names.
func (g iaPage) tab(label string) {
	g.p.t.Helper()
	for range 80 {
		if tr := access.Read(g.p.t, g.p.tt.Tester); tr.Focus != nil && tr.Focus.Label == label {
			return
		}
		g.p.tt.Key(0, ui.KeyTab)
	}
	g.p.t.Fatalf("Tab never reached %q", label)
}

// gestures runs each gesture on the control called label.
func (g iaPage) gestures(label string) iaGest {
	g.p.t.Helper()
	at := func() (float32, float32) {
		r, ok := g.p.tt.Find(label)
		if !ok {
			g.p.t.Fatalf("%q is not on screen: %q", label, g.p.tt.Texts())
		}
		return r.X + r.W/2, r.Y + r.H/2
	}
	var out iaGest
	g.reset()
	x, y := at()
	g.p.tt.ClickAt(x, y)
	out.click = g.fx()
	g.reset()
	x, y = at()
	g.p.tt.ClickAt(x, y)
	g.p.tt.ClickAt(x, y)
	out.dbl = g.fx()
	// The keys, on one focus: Enter goes last, for it may open a page.
	g.reset()
	g.tab(label)
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
		into *string
	}{{ui.Shift, ui.KeyEnter, &out.shiftEnter}, {0, ui.KeySpace, &out.space}, {0, ui.KeyEnter, &out.enter}} {
		g.clear()
		g.p.tt.Key(k.mods, k.key)
		*k.into = g.fx()
	}
	g.reset()
	g.top()
	x, y = at()
	g.p.tt.RightClickAt(x, y)
	out.right = g.fx()
	g.reset()
	return out
}

// The page's Play plays this video (the video view opens once playback starts);
// "Play the song" plays the song the video is for, from its own ID, as the song
// page's Play does. Neither opens a page, and no modifier or right-click does
// anything.
func TestInteractionVideoPlayButtons(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.loaded()
	pg := iaPage{p}
	const video = "play video mv.1"
	iaWant(t, "Play", pg.gestures("Play Sky Video"), iaGest{
		click: video, dbl: iaJoin(video, video), enter: video, shiftEnter: "nothing", space: video, right: "nothing",
	})
	const song = "play song s.1 from=:s.1"
	iaWant(t, "Play the song", pg.gestures("Play the song"), iaGest{
		click: song, dbl: iaJoin(song, song), enter: song, shiftEnter: "nothing", space: song, right: "nothing",
	})
}

// "Watch in Apple Music" hands the video's link to the system's browser: it is
// the page's only way to watch where Sona cannot play videos, and then it leads
// (the Play button is not offered, the note says why).
func TestInteractionVideoWatchInAppleMusic(t *testing.T) {
	const url = "open https://music.apple.com/gb/music-video/sky-video/1"
	for _, c := range []struct {
		name   string
		plays  bool
		pageOf func() *pageRig
	}{
		{"where videos play", true, func() *pageRig { return newPageRig(t, false, 1132, 1253) }},
		{"where videos cannot play", false, func() *pageRig {
			p := newPageRig(t, false, 1132, 1253)
			p.caps.Video = false
			return p
		}},
	} {
		p := c.pageOf()
		p.loaded()
		if got := p.tt.HasText("Play Sky Video"); got != c.plays {
			t.Errorf("%s: Play offered %v, want %v", c.name, got, c.plays)
		}
		iaWant(t, "Watch in Apple Music "+c.name, iaPage{p}.gestures("Watch in Apple Music"), iaGest{
			click: url, dbl: iaJoin(url, url), enter: url, shiftEnter: "nothing", space: url, right: "nothing",
		})
	}
}

// The byline's artist is a link to their page (through the app's navigation)
// and the song line's name a link to the song's page (through the router): each
// opens on a click, a double-click and Enter; Space and Shift+Enter do nothing.
func TestInteractionVideoLinks(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.loaded()
	for _, c := range []struct{ label, loc string }{
		{"Mia Rivers", "/artist/ar.1"},
		{"Sky", "/song/s.1"},
	} {
		if info, ok := p.tt.Access(c.label); !ok || info.Role != ui.RoleLink {
			t.Errorf("%s: role %v (found %v), want a link", c.label, info.Role, ok)
		}
		to := "nav " + c.loc
		want := iaGest{click: to, dbl: to, enter: to, shiftEnter: "nothing", space: "nothing", right: "nothing"}
		iaWant(t, c.label, iaPage{p}.gestures(c.label), want)
	}
}
