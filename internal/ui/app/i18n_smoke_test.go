package app

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/i18n/po"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// smokePages are the main pages of the features that show text without a
// network or a signed-in account: the shell's own chrome is on each.
var smokePages = []string{
	"/",
	"/library/albums",
	"/library/songs",
	"/library/playlists",
	"/search",
	"/concerts",
	"/radio",
	"/settings",
	"/equalizer",
}

// smokeShell builds the shell with every real module over the synthetic
// library at path, showing language, and draws it until it is quiet. view
// sees each frame's context, for what a test reads from it.
//
// The shell chooses its language from the settings and Apple's translations,
// and neither reaches a pseudo locale or a language Sona has no text for: its
// pending choice is made stale so that it cannot overwrite the one the test
// sets.
func smokeShell(t *testing.T, language, path string, view func(*ui.Context)) (*App, *uitest.Session) {
	t.Helper()
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now, StartPath: path}, factories)
	t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
	a.firstPaint = nil // the network stays closed
	a.connect()
	a.locale.mu.Lock()
	a.locale.seq++
	a.locale.mu.Unlock()
	i18n.SetLocale(language)
	tt := h.Tester(t, func(c *ui.Context) {
		if view != nil {
			view(c)
		}
		a.view(c)
	}, 1132, 800)
	tt.Art = append(tt.Art, a.env.Art)
	tt.Drain()
	tt.Settle()
	return a, tt
}

// resetLanguage puts the language back to English once the test is over:
// it is process-wide.
func resetLanguage(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { i18n.SetLocale("") })
}

// sourceMessages are the msgids of the catalogue that carry no argument:
// text that, shown as is in a pseudo-localised run, was not looked up.
func sourceMessages(t *testing.T) map[string]bool {
	t.Helper()
	f, err := po.Read("../i18n/locales/en.po")
	if err != nil {
		t.Fatal(err)
	}
	var letters = regexp.MustCompile(`\p{L}{2}`)
	out := map[string]bool{}
	for _, e := range f.Entries {
		if !strings.ContainsAny(e.ID, "{}") && letters.MatchString(e.ID) {
			out[e.ID] = true
		}
	}
	return out
}

// en-XA turns every message into accented, bracketed, stretched English
// ("[Ĥöɱé ~~]"). On each main page of the shell and the features, every
// text the page shows that is a message of Sona's catalogue must therefore
// be the transformed one: the English msgid on screen is a string that
// skipped i18n.T. (Text from the library, as an album's title, is not a
// message, and is shown as it is.)
func TestPseudoLocaleLeavesNoEnglishOnTheMainPages(t *testing.T) {
	resetLanguage(t)
	source := sourceMessages(t)
	for _, path := range smokePages {
		t.Run(path, func(t *testing.T) {
			_, tt := smokeShell(t, i18n.PseudoLocale, path, nil)
			texts := tt.Texts()
			if len(texts) == 0 {
				t.Fatal("the page shows no text")
			}
			pseudo := 0
			for _, s := range texts {
				if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
					pseudo++
				}
				if source[s] {
					t.Errorf("%s shows %q in English: it is a message that skipped i18n.T (or was formatted before it)", path, s)
				}
			}
			if pseudo == 0 {
				t.Errorf("%s shows no pseudo-localised text: %q", path, texts)
			}
		})
	}
}

// One right-to-left language (Arabic) lays every main page out from the
// right: the window reads the direction its views build in (Context.Direction),
// the sidebar moves to the right edge and nothing panics on the way. The
// strings stay English, as no Arabic catalogue is embedded.
func TestRightToLeftLocaleBuildsTheMainPagesMirrored(t *testing.T) {
	resetLanguage(t)
	const width = 1132

	// The same page left to right, for where the sidebar sits.
	_, ltr := smokeShell(t, "ar", "/library/albums", nil)
	brand, ok := ltr.Find("Sona")
	if !ok {
		t.Fatalf("the sidebar's name is not shown: %q", ltr.Texts())
	}
	if brand.X >= width/2 {
		t.Fatalf("left to right, the sidebar is at x=%v, not the left", brand.X)
	}

	for _, path := range smokePages {
		t.Run(path, func(t *testing.T) {
			var seen []ui.Direction
			_, tt := smokeShell(t, "ar", path, func(c *ui.Context) {
				if d := c.Direction(); !slices.Contains(seen, d) {
					seen = append(seen, d)
				}
			})
			tt.SetLocale("ar")
			tt.SetDirection(ui.RightToLeft)
			tt.Drain()
			tt.Settle()
			if !slices.Contains(seen, ui.RightToLeft) {
				t.Fatalf("no view was built right to left (saw %v)", seen)
			}
			brand, ok := tt.Find("Sona")
			if !ok {
				t.Fatalf("the sidebar's name is not shown: %q", tt.Texts())
			}
			if brand.X < width/2 {
				t.Errorf("right to left, the sidebar is at x=%v, not the right", brand.X)
			}
		})
	}
}
