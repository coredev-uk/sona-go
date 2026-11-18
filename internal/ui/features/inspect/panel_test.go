package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// appleSong is the cached response for the song: Apple's resource as Go's
// cache holds it.
const appleSong = `{"data":[{"id":"1440","type":"songs","href":"/v1/catalog/gb/songs/1440","attributes":{"name":"Airbag","artistName":"Radiohead",` +
	`"albumName":"OK Computer","durationInMillis":284000,"url":"https://music.apple.com/gb/song/airbag/1440","isrc":"GBAYE9700123","releaseDate":"1997-05-21",` +
	`"genreNames":["Alternative","Music"],"hasLyrics":true,"artwork":{"width":3000,"height":3000,"url":"https://is1-ssl.mzstatic.com/image/thumb/Music/v4/{w}x{h}bb.jpg","bgColor":"b0a98f"},` +
	`"playParams":{"id":"1440","kind":"song"}},"relationships":{"artists":{"data":[{"id":"657515","type":"artists"}]}}}]}`

// seedApple puts a cached Apple response for the song in Go's cache.
func (r *rig) seedApple(t *testing.T, body string) {
	t.Helper()
	if err := r.h.DB.Q.PutCache(context.Background(), dbq.PutCacheParams{
		Key: "gb|song-artists|1440", Storefront: "gb", Body: []byte(body), FetchedAt: 1, ExpiresAt: 1 << 50,
	}); err != nil {
		t.Fatal(err)
	}
}

// open chooses Inspect on the named item, with the tab and the view given.
func (r *rig) openOn(t *testing.T, name string, tab int) {
	t.Helper()
	r.run(t, r.items[name])
	r.in.tab = tab
	r.tt.Settle()
}

// rows are the keys of the tree on screen, in order, for the tab chosen.
func (r *rig) paths(t *testing.T, tree *jtree) []string {
	t.Helper()
	var out []string
	for i := range tree.state.Rows() {
		out = append(out, tree.state.Item(i).path)
	}
	return out
}

// The tree folds below the second level, and a click on an arrow or Left and
// Right open and close a row.
func TestTreeFoldsAndUnfolds(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, appleSong)
	r.openOn(t, "song", tabApple)
	tree := r.in.data.apple
	got := r.paths(t, tree)
	for _, want := range []string{"data", "data[0]", "data[0].id", "data[0].attributes", "data[0].relationships"} {
		if !slices.Contains(got, want) {
			t.Errorf("a payload's top levels are open: no %q in %v", want, got)
		}
	}
	if slices.Contains(got, "data[0].attributes.name") {
		t.Errorf("a level below the second is open: %v", got)
	}
	// Choose "attributes" with a click and open it with Right.
	if err := r.tt.Click("attributes"); err != nil {
		t.Fatal(err)
	}
	r.tt.Settle()
	if n := tree.chosenNode(); n == nil || n.path != "data[0].attributes" {
		t.Fatalf("the click chose %v", n)
	}
	r.tt.Key(0, ui.KeyRight)
	r.tt.Settle()
	if !slices.Contains(r.paths(t, tree), "data[0].attributes.name") {
		t.Errorf("Right did not open attributes: %v", r.paths(t, tree))
	}
	r.tt.Key(0, ui.KeyLeft)
	r.tt.Settle()
	if slices.Contains(r.paths(t, tree), "data[0].attributes.name") {
		t.Errorf("Left did not fold attributes: %v", r.paths(t, tree))
	}
	// A folded container says what it holds.
	if !r.tt.HasText("11 keys") && !r.tt.HasText("1 key") {
		t.Errorf("a folded row shows no count: %q", r.tt.Texts())
	}
}

// The filter keeps the rows whose key or value hold the words, with their
// ancestors; no match says so, and clearing it brings back the first levels.
func TestTreeFilter(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, appleSong)
	r.openOn(t, "song", tabApple)
	tree := r.in.data.apple
	all := r.paths(t, tree)

	tree.query = "gbaye"
	r.tt.Settle()
	got := r.paths(t, tree)
	want := []string{"data", "data[0]", "data[0].attributes", "data[0].attributes.isrc"}
	if !slices.Equal(got, want) {
		t.Errorf("filter gbaye: rows %v, want %v", got, want)
	}
	if !r.tt.HasText("1 match") {
		t.Errorf("no count of matches: %q", r.tt.Texts())
	}

	// A key matches, and every word must be in the key or the value of one row.
	tree.query = "artwork"
	r.tt.Settle()
	if got := r.paths(t, tree); !slices.Contains(got, "data[0].attributes.artwork") || slices.Contains(got, "data[0].attributes.isrc") {
		t.Errorf("filter artwork: %v", got)
	}
	tree.query = "name airbag"
	r.tt.Settle()
	if got := r.paths(t, tree); !slices.Contains(got, "data[0].attributes.name") || slices.Contains(got, "data[0].attributes.artistName") {
		t.Errorf("filter name airbag: %v", got)
	}

	tree.query = "no such thing"
	r.tt.Settle()
	if !r.tt.HasText("No matches") {
		t.Errorf("no empty state: %q", r.tt.Texts())
	}

	tree.query = ""
	r.tt.Settle()
	if got := r.paths(t, tree); !slices.Equal(got, all) {
		t.Errorf("clearing the filter: %v, want %v", got, all)
	}
}

// Typing in the filter field filters, and Ctrl+F puts the focus there.
func TestFilterFieldTypesAndControlFFocusesIt(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, appleSong)
	r.openOn(t, "song", tabApple)
	r.tt.Key(ui.Ctrl, ui.KeyF)
	r.tt.Settle()
	r.tt.Type("isrc")
	r.tt.Settle()
	if got := r.in.data.apple.query; got != "isrc" {
		t.Fatalf("the filter holds %q after typing", got)
	}
	if !slices.Contains(r.paths(t, r.in.data.apple), "data[0].attributes.isrc") {
		t.Errorf("rows: %v", r.paths(t, r.in.data.apple))
	}
}

// Enter copies the value of the chosen row, Ctrl+Shift+C its path, and the
// buttons a row shows beside it do the same.
func TestCopyValueAndPath(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, appleSong)
	r.openOn(t, "song", tabApple)
	tree := r.in.data.apple
	tree.query = "isrc"
	r.tt.Settle()
	if err := r.tt.Click("isrc"); err != nil {
		t.Fatal(err)
	}
	r.tt.Settle()
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Key(ui.Ctrl|ui.Shift, ui.KeyC)
	r.tt.Settle()
	got := r.h.Copied()
	if len(got) != 2 || got[0] != "GBAYE9700123" || got[1] != "data[0].attributes.isrc" {
		t.Fatalf("copied %q", got)
	}
	// The pointer finds the same two beside the row...
	r.tt.MustClick("Copy value")
	r.tt.MustClick("Copy path")
	got = r.h.Copied()
	if len(got) != 4 || got[2] != "GBAYE9700123" || got[3] != "data[0].attributes.isrc" {
		t.Errorf("the row's buttons copied %q", got)
	}
	// ...and the keyboard has them as buttons in the foot, which act on the row chosen.
	access.FocusByTab(t, r.tt.Tester, "Copy value")
	r.tt.Key(0, ui.KeyEnter)
	got = r.h.Copied()
	if len(got) != 5 || got[4] != "GBAYE9700123" {
		t.Errorf("the foot's Copy value copied %q", got)
	}
	if !r.tt.HasText("Copied") {
		t.Errorf("no toast: %q", r.tt.Texts())
	}
	// A container copies as JSON.
	tree.query = "genrenames"
	r.tt.Settle()
	if err := r.tt.Click("genreNames"); err != nil {
		t.Fatal(err)
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Settle()
	got = r.h.Copied()
	var names []string
	if err := json.Unmarshal([]byte(got[len(got)-1]), &names); err != nil || !slices.Equal(names, []string{"Alternative", "Music"}) {
		t.Errorf("an array copied %q: %v", got[len(got)-1], err)
	}
}

// A chip copies its ID and says which.
func TestChipsCopyTheirIDs(t *testing.T) {
	r := newRig(t, true)
	r.openOn(t, "libalbum", tabOverview)
	for _, want := range []string{"Catalogue", "Library"} {
		if !r.tt.HasText(want) {
			t.Fatalf("no %s chip: %q", want, r.tt.Texts())
		}
	}
	r.tt.MustClick("Copy Catalogue " + fixture.AlbumCatalogID(0))
	r.tt.MustClick("Copy Library " + fixture.AlbumLibraryID(0))
	got := r.h.Copied()
	if len(got) != 2 || got[0] != fixture.AlbumCatalogID(0) || got[1] != fixture.AlbumLibraryID(0) {
		t.Fatalf("copied %q", got)
	}
	if !r.tt.HasText("Copied " + fixture.AlbumLibraryID(0)) {
		t.Errorf("no toast: %q", r.tt.Texts())
	}
}

// A tab with nothing to show is disabled and says why; the arrows and a click
// pass it by.
func TestTabsWithNoDataAreDisabledWithAReason(t *testing.T) {
	cases := []struct {
		item string
		want map[int]string
	}{
		{"song", map[int]string{tabApple: "Not in Go's catalogue cache", tabLibrary: "Not in your library"}},
		{"station", map[int]string{tabApple: "Not in Go's catalogue cache", tabLibrary: "Not in your library", tabAddon: "Add-ons aren't offered this kind of item"}},
		{"libalbum", map[int]string{tabApple: "Not in Go's catalogue cache"}},
	}
	for _, tc := range cases {
		r := newRig(t, true)
		r.openOn(t, tc.item, tabOverview)
		reasons := r.in.reasons()
		for tab, want := range tc.want {
			if reasons[tab] != want {
				t.Errorf("%s: tab %d reason %q, want %q", tc.item, tab, reasons[tab], want)
			}
		}
		for tab, got := range reasons {
			if _, has := tc.want[tab]; !has && got != "" {
				t.Errorf("%s: tab %d is disabled: %q", tc.item, tab, got)
			}
		}
		tr := access.Read(t, r.tt.Tester)
		for tab, label := range tabLabels() {
			n, ok := tr.Find(ui.RoleTab, label)
			if !ok {
				t.Fatalf("%s: no %s tab:\n%s", tc.item, label, tr.Dump())
			}
			if _, off := tc.want[tab]; n.Has(access.Disabled) != off {
				t.Errorf("%s: %s tab disabled = %v, want %v", tc.item, label, n.Has(access.Disabled), off)
			}
		}
		if _, ok := tc.want[tabLibrary]; ok {
			_ = r.tt.Click("Library")
			r.tt.Settle()
			if r.in.tab != tabOverview {
				t.Errorf("%s: a click chose the disabled Library tab", tc.item)
			}
		}
	}
}

// The tooltip of a disabled tab says why.
func TestDisabledTabTooltipSaysWhy(t *testing.T) {
	r := newRig(t, true)
	r.openOn(t, "song", tabOverview)
	r.tt.Hover("Library")
	r.tt.Advance(1_000_000_000)
	if !r.tt.HasText("Not in your library") {
		t.Errorf("no reason on hover: %q", r.tt.Texts())
	}
}

// The Library tab is a ledger of the SQLite row, with a null as a null.
func TestLibraryTabIsALedgerOfTheRow(t *testing.T) {
	r := newRig(t, true)
	r.openOn(t, "libalbum", tabLibrary)
	if len(r.in.data.library) == 0 {
		t.Fatal("no library rows")
	}
	var labels []string
	for _, row := range r.in.data.library {
		labels = append(labels, row.label)
	}
	for _, want := range []string{"LibraryID", "Name", "ArtistName"} {
		if !slices.Contains(labels, want) {
			t.Errorf("no %s row in %v", want, labels)
		}
	}
	if !r.tt.HasText("Abbey Road") {
		t.Errorf("the row's name is not shown: %q", r.tt.Texts())
	}
	for _, row := range r.in.data.library {
		if strings.Contains(row.value, `"Valid"`) {
			t.Errorf("a null type shows as its fields: %+v", row)
		}
	}
}

// The ledger orders what Sona mapped into identity, text, flags and numbers,
// dates, durations, artwork and links, formats durations and dates beside their
// raw value, draws a colour as a swatch and reads a database null as one.
func TestLedgerOrderAndFormats(t *testing.T) {
	root := parseJSON(`{"url":"https://music.apple.com/gb/album/x/1","explicit":true,"name":"X","durationMs":3725000,"dateAdded":1700000000000,` +
		`"releaseDate":"2020-01-31","id":"9","color":"#336699","libraryId":{"String":"","Valid":false},"trackCount":12,"art":{"small":"/art/k/96x96.jpg"}}`)
	rows := ledgerOf(root, services.InspectArt{})
	var order []string
	by := map[string]ledgerRow{}
	for _, r := range rows {
		order = append(order, r.label)
		by[r.label] = r
	}
	want := []string{"id", "libraryId", "name", "color", "explicit", "trackCount", "dateAdded", "releaseDate", "durationMs", "art.small", "url"}
	if !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
	if r := by["durationMs"]; r.value != "1:02:05" || r.note != "3725000 ms" || r.copy != "3725000" {
		t.Errorf("duration %+v", r)
	}
	if r := by["dateAdded"]; r.kind != rowNumber || r.note != "1700000000000" || !strings.Contains(r.value, "2023-11-1") {
		t.Errorf("date %+v", r)
	}
	if r := by["color"]; r.kind != rowColour || r.value != "#336699" {
		t.Errorf("colour %+v", r)
	}
	if r := by["libraryId"]; r.kind != rowNull || r.value != "null" {
		t.Errorf("null pair %+v", r)
	}
	if r := by["url"]; r.kind != rowLink || r.url == "" {
		t.Errorf("url %+v", r)
	}
}

// An artwork template shows beside its URL at each size, with a link to open it.
func TestOverviewShowsTheArtworkTemplateAndItsURLs(t *testing.T) {
	r := newRig(t, true)
	r.openOn(t, "song", tabOverview)
	var tmpl, url string
	for _, row := range r.in.data.overview {
		switch row.label {
		case "art.template":
			tmpl = row.value
		case "art.large · 1000×1000":
			url = row.value
		}
	}
	if tmpl != songTemplate {
		t.Errorf("template %q", tmpl)
	}
	if want := "https://is1-ssl.mzstatic.com/image/thumb/Music/v4/ab/cd/ef/airbag.jpg/1000x1000bb.jpg"; url != want {
		t.Errorf("url %q, want %q", url, want)
	}
	r.in.view = 0
	last := len(r.in.data.overview) - 1
	r.in.lists[0].list.ScrollTo(last, ui.Start)
	r.tt.Settle()
	// The link opens on a click, from the row's own button as from its text.
	r.tt.Hover("art.large · 1000×1000")
	r.tt.MustClick("Open")
	if opened := r.h.Opened(); len(opened) != 1 || !strings.HasSuffix(opened[0], "/1000x1000bb.jpg") {
		t.Errorf("opened %q", opened)
	}
}

// Open in Apple Music opens the item's music.apple.com page, from the cached
// resource, on a click and never before.
func TestOpenInAppleMusicOpensThePageOnAClick(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, appleSong)
	r.openOn(t, "song", tabOverview)
	if len(r.h.Opened()) != 0 {
		t.Fatal("the panel opened a link on its own")
	}
	r.tt.MustClick("Open in Apple Music")
	if got := r.h.Opened(); len(got) != 1 || got[0] != "https://music.apple.com/gb/song/airbag/1440" {
		t.Errorf("opened %q", got)
	}
	// An item with no page has no button.
	r.openOn(t, "album", tabOverview)
	if r.tt.HasText("Open in Apple Music") {
		t.Error("an item with no music.apple.com page offers one")
	}
}

// The footer copies the tab shown as JSON, and everything as one document.
func TestFooterCopiesTheTabAndEverything(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, appleSong)
	r.openOn(t, "song", tabApple)
	r.tt.MustClick("Copy")
	r.tt.MustClick("Copy as JSON")
	got := r.h.Copied()
	if len(got) != 2 {
		t.Fatalf("copied %q", got)
	}
	var tab, all map[string]any
	if err := json.Unmarshal([]byte(got[0]), &tab); err != nil || tab["data"] == nil {
		t.Errorf("the tab copied %q: %v", got[0], err)
	}
	if err := json.Unmarshal([]byte(got[1]), &all); err != nil {
		t.Fatalf("everything copied %q: %v", got[1], err)
	}
	for _, k := range []string{"kind", "id", "mapped", "raw", "addon", "url", "artwork"} {
		if all[k] == nil {
			t.Errorf("everything lacks %q: %s", k, got[1])
		}
	}
}

// The add-on tab's JSON is what the protocol code produces for the item.
func TestAddonTabHoldsTheProtocolsJSON(t *testing.T) {
	r := newRig(t, true)
	cases := map[string]struct{ target, track string }{
		"song": {
			target: `{"kind":"song","id":"1440","name":"Airbag"}`,
			track: `{"id":"1440","title":"Airbag","artist":"Radiohead","album":"OK Computer","duration":284,"explicit":true,` +
				`"artwork":"https://is1-ssl.mzstatic.com/image/thumb/Music/v4/ab/cd/ef/airbag.jpg/600x600bb.jpg"}`,
		},
		"album":    {target: `{"kind":"album","id":"1700","name":"OK Computer"}`},
		"playlist": {target: `{"kind":"playlist","id":"pl.hero","name":"Today's Hits"}`},
	}
	for name, w := range cases {
		r.openOn(t, name, tabAddon)
		var doc struct {
			Invoke     *struct{ Target json.RawMessage } `json:"invoke"`
			NowPlaying *struct{ Track json.RawMessage }  `json:"now-playing"`
		}
		r.tt.MustClick("Copy")
		copied := r.h.Copied()
		if err := json.Unmarshal([]byte(copied[len(copied)-1]), &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		same := func(what string, got json.RawMessage, want string) {
			t.Helper()
			if want == "" {
				if len(got) != 0 {
					t.Errorf("%s: %s = %s, want none", name, what, got)
				}
				return
			}
			var a, b any
			_ = json.Unmarshal(got, &a)
			_ = json.Unmarshal([]byte(want), &b)
			x, _ := json.Marshal(a)
			y, _ := json.Marshal(b)
			if string(x) != string(y) {
				t.Errorf("%s: %s = %s, want %s", name, what, got, want)
			}
		}
		var target, track json.RawMessage
		if doc.Invoke != nil {
			target = doc.Invoke.Target
		}
		if doc.NowPlaying != nil {
			track = doc.NowPlaying.Track
		}
		same("invoke target", target, w.target)
		same("now-playing track", track, w.track)
		if !r.tt.HasText("Add-ons never see private listens") && !strings.Contains(strings.Join(r.tt.Texts(), " "), "never see private listens") {
			t.Errorf("%s: the tab does not say add-ons never see private listens: %q", name, r.tt.Texts())
		}
	}
}

// Nothing the panel shows or copies holds a token or a request header.
func TestNoTokenOrHeaderInThePanelOrItsCopies(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, strings.Replace(appleSong, `"isrc":"GBAYE9700123"`, `"isrc":"`+secret+`"`, 1))
	for _, name := range []string{"song", "album", "playlist", "station", "libalbum"} {
		r.openOn(t, name, tabOverview)
		texts := []string{r.in.report.Mapped, r.in.report.Raw, r.in.report.Row, r.in.report.RawKey, r.in.asJSON(), r.in.addonJSON(), strings.Join(r.tt.Texts(), "\n")}
		for _, tab := range []int{tabApple, tabAddon} {
			r.in.tab = tab
			r.tt.Settle()
			texts = append(texts, strings.Join(r.tt.Texts(), "\n"))
		}
		for _, text := range texts {
			if strings.Contains(text, secret) {
				t.Errorf("%s: a token reached the panel", name)
			}
			for _, header := range []string{"Authorization", "Bearer", "Media-User-Token", "Music-User-Token", "Origin:"} {
				if strings.Contains(text, header) {
					t.Errorf("%s: %q reached the panel", name, header)
				}
			}
		}
	}
}

// A report that could not be read says so inside the tab, with Retry.
func TestErrorShowsInsideTheTabWithRetry(t *testing.T) {
	r := newRig(t, true)
	calls := 0
	r.in.inspect = func(context.Context, services.InspectQuery) (services.InspectReport, error) {
		calls++
		if calls == 1 {
			return services.InspectReport{}, errors.New("boom")
		}
		return services.InspectReport{Mapped: `{"id":"1440","name":"Airbag"}`}, nil
	}
	r.run(t, r.items["song"])
	r.tt.Settle()
	if !r.tt.HasText("Couldn't read this item") || !r.tt.HasText("Retry") {
		t.Fatalf("no error with Retry: %q", r.tt.Texts())
	}
	r.tt.MustClick("Retry")
	r.tt.Until("loaded", func() bool { r.tt.Drain(); return !r.in.loading })
	if r.tt.HasText("Couldn't read this item") || !r.tt.HasText("Airbag") {
		t.Errorf("Retry did not load: %q", r.tt.Texts())
	}
}

// While the report loads, the layout is there with flat bars.
func TestLoadingShowsTheLayoutWithBars(t *testing.T) {
	r := newRig(t, true)
	release := make(chan struct{})
	r.in.inspect = func(context.Context, services.InspectQuery) (services.InspectReport, error) {
		<-release
		return services.InspectReport{Mapped: `{"id":"1440"}`}, nil
	}
	for _, def := range r.h.Env.Actions.For(kit.KindSong) {
		if def.ID == ActionID+"song" {
			def.Run(r.h.Env.Actions.Context([]kit.Item{r.items["song"]}, nil, nil))
		}
	}
	r.tt.Settle()
	for _, want := range []string{"Airbag", "Overview", "Copy as JSON"} {
		if !r.tt.HasText(want) {
			t.Errorf("the loading panel lacks %q: %q", want, r.tt.Texts())
		}
	}
	if tr := access.Read(t, r.tt.Tester); len(tr.FindAll(ui.RoleStatus, "Loading")) == 0 {
		t.Errorf("loading is not announced:\n%s", tr.Dump())
	}
	close(release)
	r.tt.Until("loaded", func() bool { r.tt.Drain(); return !r.in.loading })
}

// The JSON token colours are palette tokens every theme validates as text, one
// each, and distinct.
func TestTokenColoursAreValidatedPaletteTokens(t *testing.T) {
	seen := map[string]string{}
	for kind, token := range tokenNames {
		if other, dup := seen[token]; dup {
			t.Errorf("%s and %s share %s", kind, other, token)
		}
		seen[token] = kind
		if !slices.Contains(theme.TextTokens(), token) {
			t.Errorf("%s uses %s, which no theme validates as text", kind, token)
		}
	}
	for _, mode := range []theme.Mode{theme.ModeLight, theme.ModeDark} {
		theme.SetMode(mode)
		for _, k := range []jkind{jString, jNumber, jBool, jNull} {
			if tokenColour(k, false) == tokenColour(jObject, true) {
				t.Errorf("a %v value has the key's colour", k)
			}
		}
	}
	theme.SetMode(theme.ModeSystem)
}

// The panel is a dialog with tabs and a tree, one Tab stop in the tree, and
// nothing nested in a control.
func TestAccessibilityWalk(t *testing.T) {
	r := newRig(t, true)
	r.seedApple(t, appleSong)
	r.openOn(t, "song", tabApple)
	tr := access.Read(t, r.tt.Tester)
	for _, want := range []struct {
		role  ui.Role
		label string
	}{
		{ui.RoleDialog, "Inspect Airbag"},
		{ui.RoleTabList, "Inspector sections"},
		{ui.RoleTab, "Apple"},
		{ui.RoleTabPanel, "Apple"},
		{ui.RoleTree, "Apple's resource"},
		{ui.RoleTextField, "Filter Apple's resource"},
		{ui.RoleButton, "Copy as JSON"},
		{ui.RoleButton, "Copy Catalogue 1440"},
	} {
		if _, ok := tr.Find(want.role, want.label); !ok {
			t.Errorf("no %v %q:\n%s", want.role, want.label, tr.Dump())
		}
	}
	if items := tr.Roles(ui.RoleTreeItem); len(items) == 0 {
		t.Errorf("the tree has no items:\n%s", tr.Dump())
	}
	// A chosen row shows its buttons. They are for the pointer: a button inside a
	// row cannot be reached by assistive technology (a row is a leaf of the
	// tree), so the foot has the same actions, as buttons, for the row chosen.
	// The arrow of a row is MyGo's own mouse affordance: assistive technology
	// folds with the item's expanded state.
	if err := r.tt.Click("attributes"); err != nil {
		t.Fatal(err)
	}
	r.tt.Settle()
	pointerOnly := []string{"", "Copy value", "Copy path", "Open"}
	for _, n := range access.NestedControls(t, r.tt.Tester) {
		if !strings.Contains(n.Opener, "treeitem") || !slices.Contains(pointerOnly, strings.TrimPrefix(n.Inner, "not on the accessibility tree: ")) {
			t.Errorf("a control nested in a control: %v", n)
		}
	}
	for _, label := range []string{"Copy value", "Copy path"} {
		if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleButton, label); !ok {
			t.Errorf("the foot has no %q button for assistive technology", label)
		}
	}
	stops := access.StopLabels(access.TabWalk(t, r.tt.Tester, 30))
	access.InOrder(t, stops, "Copy Catalogue 1440", "Open in Apple Music", "Apple", "Filter Apple's resource")
	access.InOrder(t, stops, "Copy value", "Copy path", "Copy", "Copy as JSON", "Close")
	if !slices.ContainsFunc(stops, func(s string) bool { return strings.HasPrefix(s, "attributes") }) {
		t.Errorf("the tree is not one Tab stop: %q", stops)
	}
}

// The panel as it looks, light and dark.
func TestPanelGoldens(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts []uitest.Option
	}{{"light", nil}, {"dark", []uitest.Option{uitest.Dark()}}} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, true, tc.opts...)
			r.seedApple(t, appleSong)
			r.run(t, r.items["song"])
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), "inspect-overview-"+tc.name)
			r.in.tab = tabApple
			r.in.data.apple.query = ""
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), "inspect-apple-"+tc.name)
		})
	}
}
