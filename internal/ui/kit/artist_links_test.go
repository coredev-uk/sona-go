package kit

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

var (
	twoArtists = []services.Artist{{ID: "1435848034", Name: "The Kid LAROI"}, {ID: "320569549", Name: "Justin Bieber"}}
	oneArtist  = []services.Artist{{ID: "3296287", Name: "Queen"}}
)

// artistApp is a view holding one credit.
type artistApp struct {
	artists []services.Artist
	text    string
	static  bool
	noOpen  bool
	opened  []string
}

func (a *artistApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Gap(12).AlignItems(ui.Start).Children(func() {
		o := ArtistLinksOpts{Static: a.static}
		if !a.noOpen {
			o.Open = func(id string) { a.opened = append(a.opened, id) }
		}
		ArtistLinks(c.Key("credit"), a.artists, a.text, theme.C("ink-muted"), o)
		ui.Button(c, "Elsewhere")
	})
}

func newArtistApp(t *testing.T, a *artistApp) *ui.Tester {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	return ui.NewTester(a.view, 480, 300)
}

// One artist is a plain link, as it always was: no menu, a click opens the page.
func TestArtistLinksOneArtistIsALink(t *testing.T) {
	a := &artistApp{artists: oneArtist, text: "Queen"}
	tt := newArtistApp(t, a)
	tr := access.Read(t, tt)
	if _, ok := tr.Find(ui.RoleLink, "Queen"); !ok {
		t.Fatalf("no link named Queen:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleMenuButton, "Queen"); ok {
		t.Error("a single artist has a menu button")
	}
	if err := tt.Click("Queen"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.opened, []string{"3296287"}) {
		t.Errorf("opened %v", a.opened)
	}
	if tt.HasText("Choose an artist") {
		t.Error("a menu opened for one artist")
	}
}

// Several artists show Apple's artist string, and choosing it opens a menu of
// them: each opens its own page.
func TestArtistLinksSeveralArtistsOpenAMenuThatNavigates(t *testing.T) {
	a := &artistApp{artists: twoArtists, text: "The Kid LAROI & Justin Bieber"}
	tt := newArtistApp(t, a)
	if !tt.HasText("The Kid LAROI & Justin Bieber") {
		t.Fatalf("the credit is not shown: %q", tt.Texts())
	}
	tr := access.Read(t, tt)
	btn, ok := tr.Find(ui.RoleMenuButton, "The Kid LAROI & Justin Bieber")
	if !ok || btn.HasPopup != ui.PopupMenu || !btn.Has(access.Focusable) || btn.Has(access.Expanded) {
		t.Fatalf("want a focusable menu button with a popup, collapsed:\n%s", tr.Dump())
	}
	if err := tt.Click("The Kid LAROI & Justin Bieber"); err != nil {
		t.Fatal(err)
	}
	tr = access.Read(t, tt)
	if n, ok := tr.Find(ui.RoleMenuButton, "The Kid LAROI & Justin Bieber"); !ok || !n.Has(access.Expanded) {
		t.Errorf("the button does not say it is expanded:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleMenu, "Choose an artist"); !ok {
		t.Fatalf("no menu named Choose an artist:\n%s", tr.Dump())
	}
	for _, name := range []string{"The Kid LAROI", "Justin Bieber"} {
		if _, ok := tr.Find(ui.RoleMenuItem, name); !ok {
			t.Errorf("no menu item %q:\n%s", name, tr.Dump())
		}
	}
	if len(a.opened) != 0 {
		t.Fatalf("opening the menu opened a page: %v", a.opened)
	}
	if err := tt.Click("Justin Bieber"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(50e6)
	tt.Frame()
	if !slices.Equal(a.opened, []string{"320569549"}) {
		t.Errorf("opened %v, want the second artist", a.opened)
	}
	if tt.HasText("Choose an artist") {
		t.Error("the menu stayed open")
	}
}

// Enter opens the menu on its first artist, the arrows move, Enter chooses and
// the focus goes back to the button; Escape closes without choosing.
func TestArtistLinksChooserKeyboard(t *testing.T) {
	a := &artistApp{artists: twoArtists, text: "The Kid LAROI & Justin Bieber"}
	tt := newArtistApp(t, a)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("The Kid LAROI & Justin Bieber") {
		t.Fatal("the button is not the first Tab stop")
	}
	tt.Key(0, ui.KeyEnter)
	if !tt.Focused("Choose an artist") {
		t.Fatal("the menu did not take the focus")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(50e6)
	if tt.HasText("Choose an artist") || len(a.opened) != 0 {
		t.Fatalf("Escape: menu shows %v, opened %v", tt.HasText("Choose an artist"), a.opened)
	}
	if !tt.Focused("The Kid LAROI & Justin Bieber") {
		t.Error("Escape did not give the focus back")
	}
	tt.Key(0, ui.KeySpace)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	tt.Frame()
	if !slices.Equal(a.opened, []string{"320569549"}) {
		t.Errorf("opened %v, want the second artist", a.opened)
	}
	if !tt.Focused("The Kid LAROI & Justin Bieber") {
		t.Error("the focus did not return to the button")
	}
}

// In a list the credit is no Tab stop (the list is one), yet it is on the tree
// as a menu button.
func TestArtistLinksStaticChooserIsNotATabStop(t *testing.T) {
	a := &artistApp{artists: twoArtists, text: "The Kid LAROI & Justin Bieber", static: true}
	tt := newArtistApp(t, a)
	tr := access.Read(t, tt)
	btn, ok := tr.Find(ui.RoleMenuButton, "The Kid LAROI & Justin Bieber")
	if !ok || btn.Has(access.Focusable) || btn.HasPopup != ui.PopupMenu {
		t.Fatalf("want an unfocusable menu button with a popup:\n%s", tr.Dump())
	}
	if err := tt.Click("The Kid LAROI & Justin Bieber"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Justin Bieber") {
		t.Error("clicking did not open the menu")
	}
}

// With no artists, or where the artist page is not ported, the credit is text:
// never a link to a "Various Artists" page.
func TestArtistLinksWithoutArtistsOrPageIsText(t *testing.T) {
	for name, a := range map[string]*artistApp{
		"no artists":   {text: "Various Artists"},
		"no page":      {artists: twoArtists, text: "The Kid LAROI & Justin Bieber", noOpen: true},
		"one, no page": {artists: oneArtist, text: "Queen", noOpen: true},
	} {
		tt := newArtistApp(t, a)
		tr := access.Read(t, tt)
		for _, role := range []ui.Role{ui.RoleLink, ui.RoleMenuButton} {
			if n, ok := tr.Find(role, a.text); ok {
				t.Errorf("%s: %q is a %v", name, a.text, n.Role)
			}
		}
		if !tt.HasText(a.text) {
			t.Errorf("%s: %q is not shown", name, a.text)
		}
	}
}

// --- SongArtists -----------------------------------------------------------------

type fakeSongArtists struct {
	mu    sync.Mutex
	calls [][]string
	by    map[string][]services.Artist
	err   error
}

func (f *fakeSongArtists) SongArtists(_ context.Context, ids []string) (map[string][]services.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, slices.Clone(ids))
	if f.err != nil {
		return nil, f.err
	}
	return f.by, nil
}

func (f *fakeSongArtists) requests() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func newSongArtists(f *fakeSongArtists) *SongArtists {
	s := NewSongArtists(f, func(fn func()) { fn() })
	s.wait = 0
	return s
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A library song knows its library artist alone: its catalogue copy's artists
// are looked up when it is drawn, in one batched call for every song asked in
// the frame, and once.
func TestSongArtistsLooksUpALibrarySongsCatalogueCopy(t *testing.T) {
	f := &fakeSongArtists{by: map[string][]services.Artist{"1574968888": twoArtists, "5": oneArtist}}
	s := newSongArtists(f)
	s.wait = 40 * time.Millisecond // the frame's asks gather
	lib := services.Song{ID: "i.1", Source: services.SourceLibrary, CatalogID: "1574968888", Artist: "The Kid LAROI & Justin Bieber", ArtistID: "r.7"}
	if got := s.OfSong(lib); len(got) != 1 || got[0].ID != "r.7" {
		t.Fatalf("before the lookup: %+v, want the library artist alone", got)
	}
	// Another song in the same frame joins the call.
	other := services.Song{ID: "i.2", CatalogID: "5", Artist: "Earth, Wind & Fire", ArtistID: "r.8"}
	s.OfSong(other)
	eventually(t, "the lookup", func() bool { return len(s.OfSong(lib)) == 2 })
	got := s.OfSong(lib)
	if got[0].ID != "1435848034" || got[1].Name != "Justin Bieber" {
		t.Errorf("artists %+v, want Apple's order", got)
	}
	if reqs := f.requests(); len(reqs) != 1 || len(reqs[0]) != 2 {
		t.Errorf("requests %v, want one batched call for both songs", reqs)
	}
	// A lookup that finds one artist leaves the song as it was.
	if got := s.OfSong(other); len(got) != 1 || got[0].ID != "r.8" {
		t.Errorf("a one-artist answer changed the song: %+v", got)
	}
	s.OfSong(lib)
	s.OfSong(other)
	if reqs := f.requests(); len(reqs) != 1 {
		t.Errorf("a known song was asked again: %v", reqs)
	}
}

// Only a song that can name several is looked up: a name with no ", " or " & "
// names one artist, and a song's own artists are never asked about.
func TestSongArtistsAsksOnlyWhereTheNameMayNameSeveral(t *testing.T) {
	f := &fakeSongArtists{by: map[string][]services.Artist{}}
	s := newSongArtists(f)
	s.OfSong(services.Song{CatalogID: "1", Artist: "Queen", ArtistID: "r.1"})
	s.OfSong(services.Song{CatalogID: "", Artist: "A & B", ArtistID: "r.2"})
	own := services.Song{CatalogID: "3", Artist: "A & B", ArtistID: "9", Artists: twoArtists}
	if got := s.OfSong(own); !slices.Equal(got, twoArtists) {
		t.Errorf("own artists %+v", got)
	}
	var nilSongArtists *SongArtists
	if got := nilSongArtists.OfSong(services.Song{CatalogID: "4", Artist: "A & B", ArtistID: "r.4"}); len(got) != 1 || got[0].ID != "r.4" {
		t.Errorf("without a resolver: %+v", got)
	}
	time.Sleep(50 * time.Millisecond)
	if reqs := f.requests(); len(reqs) != 0 {
		t.Errorf("requests %v, want none", reqs)
	}
	if got := s.Of("", "Various Artists", "", nil); got != nil {
		t.Errorf("no artist at all: %+v", got)
	}
}

// A failed lookup leaves the first artist, and is tried again only later.
func TestSongArtistsFailureKeepsTheFirstArtist(t *testing.T) {
	f := &fakeSongArtists{err: errors.New("offline")}
	s := newSongArtists(f)
	song := services.Song{CatalogID: "1", Artist: "A & B", ArtistID: "r.1"}
	s.OfSong(song)
	eventually(t, "the lookup", func() bool { return len(f.requests()) == 1 })
	time.Sleep(20 * time.Millisecond)
	for range 3 {
		if got := s.OfSong(song); len(got) != 1 || got[0].ID != "r.1" {
			t.Fatalf("after a failure: %+v", got)
		}
	}
	if reqs := f.requests(); len(reqs) != 1 {
		t.Errorf("a failed lookup was retried at once: %v", reqs)
	}
}
