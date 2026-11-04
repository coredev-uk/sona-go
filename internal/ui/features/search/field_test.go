package search

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// ampHost is the web player's API host, where the suggestions are asked.
const ampHost = "amp-api.music.apple.com"

// suggestionsJSON is Apple's answer for a term starting "blu": two
// completions and three top results (an artist, an album and a song).
const suggestionsJSON = `{"results":{"suggestions":[
	{"kind":"terms","searchTerm":"blue","displayTerm":"blue"},
	{"kind":"terms","searchTerm":"blue band","displayTerm":"blue band"},
	{"kind":"topResults","content":{"id":"9","type":"artists"}},
	{"kind":"topResults","content":{"id":"5","type":"albums"}},
	{"kind":"topResults","content":{"id":"1","type":"songs"}}]},
	"resources":{
	"artists":{"9":{"id":"9","type":"artists","attributes":{"name":"Blue Band"}}},
	"albums":{"5":{"id":"5","type":"albums","attributes":{"name":"Blue Hour","artistName":"Blue Band"}}},
	"songs":{"1":{"id":"1","type":"songs","attributes":{"name":"Blue Moon","artistName":"Blue Band","playParams":{"id":"1","kind":"song"}}}}}}`

// fieldPlay records what the field plays.
type fieldPlay struct {
	queueStub
	mu   sync.Mutex
	refs [][]services.TrackRef
}

func (p *fieldPlay) PlayItems(_ context.Context, refs []services.TrackRef, _ int, _ services.PlayFrom) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refs = append(p.refs, refs)
	return nil
}

func (p *fieldPlay) played() [][]services.TrackRef {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.refs)
}

// fieldRig is the top bar's search field over a real SearchService, a fake
// Apple Music API on the harness network and the seeded library.
type fieldRig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	s      *search
	api    *services.SearchService
	play   *fieldPlay
	tt     *uitest.Session

	mu    sync.Mutex
	asked []string // the terms the suggestions endpoint answered
	// toResults counts the frames that asked the page to focus its first result.
	toResults int
}

func (r *fieldRig) hintTerms() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.asked)
}

func (r *fieldRig) amp(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/v1/catalog/gb/search/suggestions" {
		http.NotFound(w, req)
		return
	}
	term := req.URL.Query().Get("term")
	r.mu.Lock()
	r.asked = append(r.asked, term)
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if strings.HasPrefix(term, "blu") {
		_, _ = w.Write([]byte(suggestionsJSON))
		return
	}
	_, _ = w.Write([]byte(`{"results":{"suggestions":[]},"resources":{}}`))
}

var fieldAlbums = []fixture.Album{
	{Name: "Blue Hour", Artist: "Al"},
	{Name: "Paper Planes", Artist: "Di"},
}

func newFieldRig(t *testing.T, start string, opts ...uitest.Option) *fieldRig {
	t.Helper()
	r := &fieldRig{t: t, play: &fieldPlay{}}
	opts = append([]uitest.Option{
		uitest.Albums(fieldAlbums...),
		uitest.Apple(http.NotFoundHandler()),
	}, opts...)
	r.h = uitest.New(t, opts...)
	r.h.Network.ServeHandler(ampHost, http.HandlerFunc(r.amp))
	r.router = ui.NewRouter(start)
	r.h.Env.Nav = r.router
	r.h.Env.HasRoute = func(p string) bool {
		return strings.HasPrefix(p, "/album/") || strings.HasPrefix(p, "/artist/") || strings.HasPrefix(p, "/playlist/") || strings.HasPrefix(p, "/song/")
	}
	r.api = services.Service[services.SearchService](r.h.Core)
	r.bindFocus("/", "Ctrl+F")
	useQueue(r.h.Env, r.play)
	r.s = newSearch(r.h.Env, r.api)
	r.tt = r.h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Height(48).FillWidth().Padding(0, 16).AlignItems(ui.Center).Children(func() {
				ui.Row(c).MinWidth(240).Basis(520).Shrink(1).Grow(0).Children(func() { r.s.field.build(c) })
			})
			// The page below takes the focus request as the real page does.
			if r.s.field.takeResultsFocus() {
				r.toResults++
			}
			ui.Box(c).Grow(1)
		})
	}, 900, 520)
	return r
}

// focusField clicks into the field.
func (r *fieldRig) focusField() {
	r.t.Helper()
	if err := r.tt.Click("Search"); err != nil {
		r.t.Fatal(err)
	}
}

// typeAndWait types text and lets the pause and the suggestions go by.
func (r *fieldRig) typeAndWait(text string, shows ...string) {
	r.t.Helper()
	r.tt.Type(text)
	r.tt.Advance(hintDelay + time.Millisecond)
	r.tt.Until("the suggestions", r.tt.Shows(shows...))
}

func (r *fieldRig) key(k ui.Key) {
	r.t.Helper()
	r.tt.Key(0, k)
}

// remember writes through the service as the field does, and has the recent
// list ask again (the list is never stale on its own).
func (r *fieldRig) rememberTerm(term, scope string) {
	r.t.Helper()
	if err := r.api.RememberTerm(context.Background(), term, scope); err != nil {
		r.t.Fatal(err)
	}
	r.s.refreshRecent()
}

func (r *fieldRig) rememberItem(it services.RecentItem) {
	r.t.Helper()
	if err := r.api.RememberItem(context.Background(), it); err != nil {
		r.t.Fatal(err)
	}
	r.s.refreshRecent()
}

// recent is what the service remembers, newest first, as "kind:name".
func (r *fieldRig) recent() []string {
	r.t.Helper()
	got, err := r.api.Recent(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	var out []string
	for _, e := range got {
		out = append(out, e.Kind+":"+e.Name)
	}
	return out
}

func (r *fieldRig) untilRecent(want ...string) {
	r.t.Helper()
	r.tt.Until("the remembered entries", func() bool {
		got := r.recent()
		for _, w := range want {
			if !slices.Contains(got, w) {
				return false
			}
		}
		return true
	})
}

func TestFieldPlaceholderFollowsTheScope(t *testing.T) {
	r := newFieldRig(t, "/search")
	// The placeholder is drawn by the input (no text of the tree): the
	// words follow the scope the route names.
	if got := fieldPlaceholder(scopeCatalog); got != "Search in Apple Music" {
		t.Errorf("catalog placeholder %q", got)
	}
	if got := fieldPlaceholder(scopeLibrary); got != "Search in library" {
		t.Errorf("library placeholder %q", got)
	}
	// Its pixels: the empty field draws something other than the bare box.
	r.tt.Frame()
	empty := r.tt.Image()
	r.router.Replace("/search?scope=library")
	r.tt.Frame()
	if lib := r.tt.Image(); lib.Bounds() != empty.Bounds() || string(lib.Pix) == string(empty.Pix) {
		t.Error("the library scope draws the same field as the catalog's: the placeholder did not change")
	}
}

// bindFocus sets the keys search.focus answers to (Env.Shortcuts' answer).
func (r *fieldRig) bindFocus(keys ...string) {
	r.h.Env.Shortcuts = func() []module.BoundShortcut {
		return []module.BoundShortcut{{Shortcut: module.Shortcut{ID: "search.focus"}, Keys: keys}}
	}
}

func TestFieldHintFollowsTheBoundKey(t *testing.T) {
	r := newFieldRig(t, "/search")
	r.bindFocus("Ctrl+K", "/")
	r.tt.Frame()
	if !r.tt.HasText("Ctrl") || !r.tt.HasText("K") {
		t.Errorf("the hint did not follow the rebinding: %q", r.tt.Texts())
	}
	r.bindFocus()
	r.tt.Frame()
	if r.tt.HasText("Ctrl") || r.tt.HasText("K") || r.tt.HasText("/") {
		t.Errorf("an unbound search.focus still has a hint: %q", r.tt.Texts())
	}
}

func TestFieldShowsTheChordUntilTyped(t *testing.T) {
	r := newFieldRig(t, "/search")
	r.tt.Frame()
	if !r.tt.HasText("/") {
		t.Errorf("the empty field lacks the chord hint: %q", r.tt.Texts())
	}
	if _, ok := r.tt.Find("Clear search"); ok {
		t.Error("the empty field has a Clear button")
	}
	r.focusField()
	r.tt.Type("hello")
	if _, ok := r.tt.Find("Clear search"); !ok {
		t.Fatalf("typing shows no Clear button: %q", r.tt.Texts())
	}
	if r.tt.HasText("/") {
		t.Error("the chord hint stays while typing")
	}
}

func TestFieldClearEmptiesAndKeepsTheFocus(t *testing.T) {
	r := newFieldRig(t, "/search")
	r.focusField()
	r.tt.Type("hello")
	if err := r.tt.Click("Clear search"); err != nil {
		t.Fatal(err)
	}
	if r.s.field.text != "" {
		t.Errorf("the field holds %q after Clear", r.s.field.text)
	}
	if !r.tt.Focused("Search") {
		t.Error("Clear moved the focus out of the field")
	}
	if r.s.field.active != -1 {
		t.Errorf("active = %d after Clear", r.s.field.active)
	}
}

func TestFieldFollowsTheRoute(t *testing.T) {
	r := newFieldRig(t, "/search?q=blue")
	r.tt.Frame()
	if got := r.s.field.text; got != "blue" {
		t.Fatalf("the field opens on %q, want the route's q", got)
	}
	// A draft belongs to its location: navigating hands the field back to
	// the route's q.
	r.focusField()
	r.tt.Type(" more")
	if got := r.s.field.text; got == "blue" {
		t.Fatalf("typing changed nothing: %q", got)
	}
	r.router.Push("/search?q=red")
	r.tt.Frame()
	if got := r.s.field.text; got != "red" {
		t.Errorf("after navigating the field holds %q, want red", got)
	}
	r.router.Push("/library/albums")
	r.tt.Frame()
	if got := r.s.field.text; got != "" {
		t.Errorf("off /search the field holds %q, want empty", got)
	}
}

func TestFieldCatalogHintsFollowTheTypingAfterAPause(t *testing.T) {
	r := newFieldRig(t, "/search")
	r.focusField()
	r.tt.Type("blu")
	r.tt.Advance(100 * time.Millisecond)
	if n := len(r.hintTerms()); n != 0 {
		t.Fatalf("the suggestions were asked %d times before the pause ended", n)
	}
	r.tt.Advance(60 * time.Millisecond)
	r.tt.Until("the suggestions", r.tt.Shows("Top results", "Blue Band", "Blue Hour", "Blue Moon"))
	if got := r.hintTerms(); !slices.Equal(got, []string{"blu"}) {
		t.Errorf("suggestions asked for %q, want [blu]", got)
	}
	// Completions first (rows named by the completion), then the items,
	// each with "Kind · subtitle".
	for _, want := range []string{"blue", "blue band", "Artist", "Album · Blue Band", "Song · Blue Band", "Top results"} {
		if !r.tt.HasText(want) {
			t.Errorf("the list lacks %q: %q", want, r.tt.Texts())
		}
	}
	// The same term again is cached: no second request.
	r.router.Push("/library/albums")
	r.tt.Frame()
	r.router.Push("/search")
	r.tt.Frame()
	r.tt.Type("blu")
	r.tt.Advance(time.Second)
	r.tt.Drain()
	if got := r.hintTerms(); len(got) != 1 {
		t.Errorf("a cached term was asked again: %q", got)
	}
}

func TestFieldTermRowSetsTheTypedPartApart(t *testing.T) {
	r := newFieldRig(t, "/search")
	r.focusField()
	r.typeAndWait("blu", "blue band")
	// "blu" is muted (its own text), the rest of the completion bold.
	if !r.tt.HasText("blu") || !r.tt.HasText("e band") {
		t.Errorf("the term row is not split into typed part and remainder: %q", r.tt.Texts())
	}
}

func TestFieldLibraryHintsComeFromTheLibrary(t *testing.T) {
	r := newFieldRig(t, "/search?scope=library")
	r.focusField()
	r.typeAndWait("blue", "In your library", "Blue Hour", "Album · Al")
	if r.tt.HasText("Top results") {
		t.Error("the library scope lists completions")
	}
	if got := r.hintTerms(); len(got) != 0 {
		t.Errorf("the library scope asked Apple: %q", got)
	}
	if r.tt.HasText("Paper Planes") {
		t.Error("a non-matching album is suggested")
	}
}

func TestFieldKeepsPreviousHintsWhileTheNextLoad(t *testing.T) {
	r := newFieldRig(t, "/search")
	r.focusField()
	r.typeAndWait("blu", "Blue Hour")
	// "blue" is a new term: until its answer arrives the old list stays.
	r.tt.Type("e")
	r.tt.Advance(hintDelay + time.Millisecond)
	if !r.tt.HasText("Blue Hour") {
		t.Error("the list emptied while the next suggestions loaded")
	}
}

func TestFieldListsRecentSearchesWhenEmpty(t *testing.T) {
	r := newFieldRig(t, "/search")
	for _, term := range []string{"alpha", "beta", "gamma"} {
		r.rememberTerm(term, scopeCatalog)
	}
	r.rememberTerm("libterm", scopeLibrary)
	r.rememberItem(services.RecentItem{Kind: services.ItemAlbum, ID: "5", Source: services.SourceCatalog, Name: "Blue Hour", Subtitle: "Blue Band"})
	r.focusField()
	r.tt.Until("the recent entries", r.tt.Shows("Recently searched", "alpha", "beta", "gamma", "Blue Hour"))
	if r.tt.HasText("libterm") {
		t.Error("a library term is listed under the catalog scope")
	}
	if !r.tt.HasText("Album · Blue Band") {
		t.Errorf("the remembered item lacks its kind and subtitle: %q", r.tt.Texts())
	}
	// The library scope lists its own terms.
	r.router.Replace("/search?scope=library")
	r.tt.Frame()
	r.tt.Until("the library's recents", r.tt.Shows("libterm"))
	if r.tt.HasText("alpha") {
		t.Error("a catalog term is listed under the library scope")
	}
}

func TestFieldListsAtMostSixRecentEntries(t *testing.T) {
	r := newFieldRig(t, "/search")
	for i := range 9 {
		r.rememberTerm("term "+string(rune('a'+i)), scopeCatalog)
	}
	r.focusField()
	// Nine are remembered; six are listed. Which six depends on the stored
	// times, which a fast loop can make equal: count, don't name them.
	shown := map[string]bool{}
	r.tt.Until("the recent entries", func() bool {
		clear(shown)
		for _, w := range r.tt.Texts() {
			if strings.HasPrefix(w, "term ") {
				shown[w] = true
			}
		}
		return r.tt.HasText("Recently searched") && len(shown) > 0
	})
	if len(shown) != maxRecent {
		t.Fatalf("recent terms shown %v, want %d", shown, maxRecent)
	}
	rows := 0
	for _, o := range r.s.field.options("", scopeCatalog, hintData{}, r.s.recent().Data) {
		if o.kind == optRecent {
			rows++
		}
	}
	if rows != maxRecent {
		t.Errorf("%d recent options, want %d", rows, maxRecent)
	}
}

func TestFieldEnterSearchesTheTypedText(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.tt.Type("  Blue  ")
	r.key(ui.KeyEnter)
	if got := r.router.Location(); got != "/search?q=Blue" {
		t.Fatalf("Enter went to %q, want /search?q=Blue", got)
	}
	r.untilRecent("term:Blue")
	// The route's q is the field's text now.
	r.tt.Frame()
	if got := r.s.field.text; got != "Blue" {
		t.Errorf("the field holds %q", got)
	}
}

func TestFieldEnterKeepsTheLibraryScope(t *testing.T) {
	r := newFieldRig(t, "/search?scope=library")
	r.focusField()
	r.tt.Type("blue")
	r.key(ui.KeyEnter)
	if got := r.router.Location(); got != "/search?q=blue&scope=library" {
		t.Fatalf("Enter went to %q", got)
	}
}

func TestFieldBlankEnterRemembersNothing(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.tt.Type("   ")
	r.key(ui.KeyEnter)
	if got := r.router.Location(); got != "/search" {
		t.Fatalf("a blank Enter went to %q, want /search", got)
	}
	r.tt.Drain()
	if got := r.recent(); len(got) != 0 {
		t.Errorf("a blank search remembered %q", got)
	}
}

func TestFieldEnterReplacesHistoryOnAnEmptySearchPage(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.router.Push("/search")
	r.tt.Frame()
	r.focusField()
	r.tt.Type("blue")
	r.key(ui.KeyEnter)
	if got := r.router.Location(); got != "/search?q=blue" {
		t.Fatalf("Enter went to %q", got)
	}
	r.router.Back()
	if got := r.router.Path(); got != "/library/albums" {
		t.Errorf("Back went to %q: the empty /search entry was kept", got)
	}

	// With a query already on the page the new search is a new entry.
	r.router.Push("/search?q=one")
	r.tt.Frame()
	r.focusField()
	r.tt.Type("two")
	r.key(ui.KeyEnter)
	if got := r.router.Location(); got != "/search?q=two" {
		t.Fatalf("Enter went to %q", got)
	}
	r.router.Back()
	if got := r.router.Location(); got != "/search?q=one" {
		t.Errorf("Back went to %q, want the earlier search", got)
	}
}

func TestFieldKeysMoveThroughTheList(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "Blue Hour")
	f := r.s.field
	// Nothing is active until a key moves there.
	if f.active != -1 {
		t.Fatalf("active = %d before any key", f.active)
	}
	r.key(ui.KeyDown)
	if f.active != 0 {
		t.Errorf("Down from the field: active = %d, want 0", f.active)
	}
	r.key(ui.KeyDown)
	if f.active != 1 {
		t.Errorf("Down: active = %d, want 1", f.active)
	}
	r.key(ui.KeyUp)
	r.key(ui.KeyUp)
	if f.active != -1 {
		t.Errorf("Up past the first returns to the field: active = %d", f.active)
	}
	r.key(ui.KeyUp)
	if f.active != 4 {
		t.Errorf("Up from the field enters at the last: active = %d, want 4", f.active)
	}
	r.key(ui.KeyDown)
	if f.active != -1 {
		t.Errorf("Down past the last returns to the field: active = %d", f.active)
	}
	if !r.tt.Focused("Search") {
		t.Error("the focus left the field while moving")
	}
}

func TestFieldEnterPicksTheActiveCompletion(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "blue band")
	r.key(ui.KeyDown)
	r.key(ui.KeyDown) // "blue band"
	r.key(ui.KeyEnter)
	if got := r.router.Location(); got != "/search?q=blue+band" {
		t.Fatalf("Enter on a completion went to %q", got)
	}
	r.untilRecent("term:blue band")
}

func TestFieldEscapeClosesTheListThenBlurs(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "Top results")
	r.key(ui.KeyEscape)
	if r.tt.HasText("Top results") {
		t.Error("Escape left the list open")
	}
	if !r.tt.Focused("Search") {
		t.Error("the first Escape took the focus away")
	}
	r.key(ui.KeyEscape)
	if r.tt.Focused("Search") {
		t.Error("Escape with the list closed did not blur the field")
	}
}

func TestFieldTabClosesTheList(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "Top results")
	r.key(ui.KeyTab)
	r.tt.Frame()
	if r.tt.HasText("Top results") {
		t.Error("Tab left the list open")
	}
}

func TestFieldDownWithTheListClosedOnSearchMovesIntoTheResults(t *testing.T) {
	r := newFieldRig(t, "/search?q=zzz")
	r.focusField()
	// The term has no suggestions, so the list is closed.
	before := r.toResults
	r.key(ui.KeyDown)
	if r.toResults != before+1 {
		t.Errorf("Down with the list closed asked %d times for the results' focus, want 1", r.toResults-before)
	}
	// Elsewhere Down opens the list instead.
	r2 := newFieldRig(t, "/library/albums")
	r2.focusField()
	r2.rememberTerm("alpha", scopeCatalog)
	r2.tt.Until("the recent entry", r2.tt.Shows("alpha"))
	r2.key(ui.KeyEscape)
	r2.key(ui.KeyDown)
	if r2.toResults != 0 {
		t.Error("Down off /search asked for the results' focus")
	}
	if r2.s.field.active != 0 {
		t.Errorf("Down off /search: active = %d, want the first row", r2.s.field.active)
	}
}

func TestFieldHoverMakesARowActiveAndClickPicksIt(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "blue band")
	box, ok := r.tt.Find("blue band")
	if !ok {
		t.Fatal("no row for the completion")
	}
	r.tt.Move(box.X+box.W/2, box.Y+box.H/2)
	r.tt.Move(box.X+box.W/2+1, box.Y+box.H/2)
	if got := r.s.field.active; got != 1 {
		t.Errorf("hovering the second row made %d active", got)
	}
	if !r.tt.Focused("Search") {
		t.Error("hovering took the focus from the field")
	}
	if err := r.tt.Click("blue band"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/search?q=blue+band" {
		t.Errorf("clicking a completion went to %q", got)
	}
}

func TestFieldPickingATermSearchesItInTheScope(t *testing.T) {
	r := newFieldRig(t, "/search?scope=library")
	r.focusField()
	r.typeAndWait("blue", "Blue Hour")
	// The library scope has no completions: an item. Picking the album
	// opens it (below); here the typed text keeps the scope on Enter.
	r.key(ui.KeyEnter)
	if got := r.router.Location(); got != "/search?q=blue&scope=library" {
		t.Errorf("Enter went to %q", got)
	}
	r.untilRecent("term:blue")
	entries, _ := r.api.Recent(context.Background())
	if len(entries) == 0 || entries[0].Scope != scopeLibrary {
		t.Errorf("the library search was remembered as %+v", entries)
	}
}

func TestFieldPickingAnItemOpensItsPageAndRemembersIt(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "Blue Hour")
	if err := r.tt.Click("Blue Hour"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/album/5" {
		t.Fatalf("picking the album went to %q", got)
	}
	if r.tt.Focused("Search") {
		t.Error("picking an item left the focus in the field")
	}
	r.untilRecent("album:Blue Hour")
	entries, _ := r.api.Recent(context.Background())
	var found bool
	for _, e := range entries {
		if e.Kind == services.ItemAlbum && e.ItemID == "5" && e.Subtitle == "Blue Band" && e.Source == services.SourceCatalog {
			found = true
		}
	}
	if !found {
		t.Errorf("the album was remembered as %+v", entries)
	}
}

func TestFieldPickingASongPlaysItInsteadOfNavigating(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "Blue Moon")
	if err := r.tt.Click("Blue Moon"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("playback", func() bool { return len(r.play.played()) == 1 })
	got := r.play.played()[0]
	if len(got) != 1 || got[0].ID != "1" {
		t.Errorf("played %+v, want the song", got)
	}
	if loc := r.router.Location(); loc != "/library/albums" {
		t.Errorf("playing a song navigated to %q", loc)
	}
	r.untilRecent("song:Blue Moon")
}

func TestFieldPickingAPageThatIsNotPortedOpensNothing(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.h.Env.HasRoute = func(string) bool { return false }
	r.focusField()
	r.typeAndWait("blu", "Blue Hour")
	if err := r.tt.Click("Blue Hour"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/library/albums" {
		t.Errorf("navigated to %q, a page the router lacks", got)
	}
}

func TestFieldPickingRecentEntries(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.rememberTerm("alpha", scopeCatalog)
	r.rememberItem(services.RecentItem{Kind: services.ItemAlbum, ID: "5", Source: services.SourceCatalog, Name: "Blue Hour", Subtitle: "Blue Band"})
	r.rememberItem(services.RecentItem{Kind: services.ItemSong, ID: "i.1", Source: services.SourceLibrary, Name: "Library Song", Subtitle: "Al"})
	r.rememberItem(services.RecentItem{Kind: services.ItemSong, ID: "77", Source: services.SourceCatalog, Name: "Catalog Song", Subtitle: "Al"})
	r.focusField()
	r.tt.Until("the recent entries", r.tt.Shows("alpha", "Blue Hour", "Catalog Song"))

	// A recent item opens its page (a catalog song its song page).
	if err := r.tt.Click("Catalog Song"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/song/77" {
		t.Fatalf("a recent song went to %q", got)
	}
	r.router.Replace("/library/albums")
	r.tt.Frame()
	r.focusField()
	r.tt.Until("the recent entries", r.tt.Shows("alpha"))
	if err := r.tt.Click("Blue Hour"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/album/5" {
		t.Fatalf("a recent album went to %q", got)
	}

	// A library song has no page: picking it opens nothing.
	r.router.Replace("/library/albums")
	r.tt.Frame()
	r.focusField()
	r.tt.Until("the recent entries", r.tt.Shows("Library Song"))
	if err := r.tt.Click("Library Song"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/library/albums" {
		t.Errorf("a recent library song went to %q", got)
	}

	// A recent term runs again.
	r.focusField()
	r.tt.Until("the recent entries", r.tt.Shows("alpha"))
	if err := r.tt.Click("alpha"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/search?q=alpha" {
		t.Errorf("a recent term went to %q", got)
	}
}

func TestFieldFocusShortcutFocusesAndSelectsTheText(t *testing.T) {
	r := newFieldRig(t, "/search?q=blue")
	r.tt.Frame()
	sc := r.s.focusShortcut()
	if sc.ID != "search.focus" || sc.Scope != "global" || sc.Group != "navigation" || !slices.Equal(sc.Keys, []string{"/", "Ctrl+F"}) || sc.Label() != "Focus the search field" {
		t.Errorf("shortcut = %+v", sc)
	}
	if r.tt.Focused("Search") {
		t.Fatal("the field has the focus before the shortcut")
	}
	// The shell's dispatcher runs it from any key handler, in a frame.
	sc.Run()
	r.tt.Frame()
	if !r.tt.Focused("Search") {
		t.Fatal("the shortcut did not focus the field")
	}
	// The text is selected: typing replaces it.
	r.tt.Type("red")
	if got := r.s.field.text; got != "red" {
		t.Errorf("typing after the shortcut gave %q, want the selection replaced", got)
	}
}

func TestFieldGoldens(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "field_light"
		var opts []uitest.Option
		if dark {
			name, opts = "field_dark", []uitest.Option{uitest.Dark()}
		}
		t.Run(name, func(t *testing.T) {
			r := newFieldRig(t, "/search", opts...)
			r.focusField()
			r.typeAndWait("blu", "Blue Hour", "Blue Moon")
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), name)
		})
	}
}
