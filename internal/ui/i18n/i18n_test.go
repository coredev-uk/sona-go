package i18n

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n/locales"
)

// reset puts the package back to English with no layers.
func reset(t *testing.T) {
	t.Helper()
	clean := func() {
		mu.Lock()
		shown, regional, appleLocale, appleMessages = SourceLocale, nil, "", nil
		communities = map[string]map[string]string{}
		post = nil
		warned.Range(func(k, _ any) bool { warned.Delete(k); return true })
		rebuild()
		mu.Unlock()
	}
	clean()
	t.Cleanup(clean)
}

func appleOf(locale string, cov float64, msgs map[string]string) services.AppleTranslations {
	byID := map[string]string{}
	for k, v := range msgs {
		byID[k] = v
	}
	return services.AppleTranslations{Locale: locale, Coverage: cov, Messages: byID, Version: "translations~x.js", FetchedAt: 1}
}

func id(msg string) string { return applestrings.MessageID(msg, "") }

func TestTPlainNamedPositionalAndPlural(t *testing.T) {
	reset(t)
	if got := T("Albums"); got != "Albums" {
		t.Errorf("plain: %q", got)
	}
	if got := T("Add music by {name} to collect it here.", "name", "Björk"); got != "Add music by Björk to collect it here." {
		t.Errorf("named: %q", got)
	}
	// Lingui's positional placeholders: "0" or 0.
	if got := T("{0} by {1}", "0", "Army of Me", "1", "Björk"); got != "Army of Me by Björk" {
		t.Errorf("positional by string: %q", got)
	}
	if got := T("{0} by {1}", 0, "Army of Me", 1, "Björk"); got != "Army of Me by Björk" {
		t.Errorf("positional by int: %q", got)
	}
	const albums = "{0, plural, one {# album} other {# albums}}"
	for n, want := range map[int]string{0: "0 albums", 1: "1 album", 2: "2 albums", 1234: "1,234 albums"} {
		if got := T(albums, 0, n); got != want {
			t.Errorf("plural %d: %q, want %q", n, got, want)
		}
	}
	const two = "{0, plural, one {# album} other {# albums}}, {1, plural, one {# song} other {# songs}} in your library"
	if got := T(two, 0, 1, 1, 12); got != "1 album, 12 songs in your library" {
		t.Errorf("two plurals: %q", got)
	}
	// An apostrophe is text; a missing argument stays as written.
	if got := T("The account name or password isn't right."); got != "The account name or password isn't right." {
		t.Errorf("apostrophe: %q", got)
	}
	if got := T("Hello {name}"); got != "Hello {name}" {
		t.Errorf("missing argument: %q", got)
	}
}

func TestTArgumentMistakesPanicInTests(t *testing.T) {
	reset(t)
	for name, fn := range map[string]func(){
		"odd pairs":        func() { T("x {a}", "a") },
		"a name of a type": func() { T("x {a}", 1.5, "b") },
		"a broken message": func() { T("x {a") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			fn()
		}()
	}
}

func TestTCSeparatesContexts(t *testing.T) {
	reset(t)
	catalogue := `msgid ""
msgstr ""
"Language: de\n"

msgctxt "verb"
msgid "Play"
msgstr "Abspielen"

msgctxt "noun"
msgid "Play"
msgstr "Schauspiel"

msgid "Play"
msgstr "Spielen"
`
	if err := LoadCommunity("de", []byte(catalogue)); err != nil {
		t.Fatal(err)
	}
	if T("Play") != "Play" || TC("verb", "Play") != "Play" {
		t.Error("English shows before a locale with the catalogue is shown")
	}
	SetLocale("de")
	for _, tc := range []struct{ got, want string }{
		{TC("verb", "Play"), "Abspielen"},
		{TC("noun", "Play"), "Schauspiel"},
		{T("Play"), "Spielen"},
		{TC("other", "Play"), "Play"}, // no such context: English
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestLayersAppleThenCommunityThenEnglish(t *testing.T) {
	reset(t)
	community := "msgid \"\"\nmsgstr \"\"\n\nmsgid \"Albums\"\nmsgstr \"Alben (Gemeinschaft)\"\n\nmsgid \"Songs\"\nmsgstr \"Titel (Gemeinschaft)\"\n\nmsgid \"Fuzzy\"\n#, fuzzy\nmsgstr \"nein\"\n"
	if err := LoadCommunity("fr", []byte(community)); err != nil {
		t.Fatal(err)
	}
	SetApple(appleOf("fr-FR", 0.95, map[string]string{
		id("Add"):                "Ajouter",
		id("About {artistName}"): "À propos de {artistName}",
		id("Albums"):             "Albums",
		id("{0, plural, one {# song} other {# songs}}"): "{0, plural, one {# morceau} other {# morceaux}}",
		id("Broken"): "{broken",
	}))

	// Apple's layer is for fr-FR: English is shown until that locale is.
	if T("Add") != "Add" {
		t.Fatal("a layer shows before its locale")
	}
	var logs bytes.Buffer
	SetLogger(slog.New(slog.NewTextHandler(&logs, nil)))
	defer SetLogger(slog.Default())

	SetLocale("fr-FR")
	if Locale() != "fr-FR" {
		t.Fatalf("locale %q", Locale())
	}
	if got := T("Add"); got != "Ajouter" {
		t.Errorf("Apple's: %q", got)
	}
	if got := T("About {artistName}", "artistName", "Björk"); got != "À propos de Björk" {
		t.Errorf("Apple's with an argument: %q", got)
	}
	if got := T("Albums"); got != "Albums" {
		t.Errorf("Apple wins over the community's %q", got)
	}
	if got := T("Songs"); got != "Titel (Gemeinschaft)" {
		t.Errorf("community fills in what Apple lacks: %q", got)
	}
	if got := T("Fuzzy"); got != "Fuzzy" {
		t.Errorf("fuzzy translations are not shown: %q", got)
	}
	if got := T("Playlists"); got != "Playlists" {
		t.Errorf("English fills in: %q", got)
	}
	if got := T("{0, plural, one {# song} other {# songs}}", 0, 2); got != "2 morceaux" {
		t.Errorf("Apple's plural by French rules: %q", got)
	}
	if got := T("{0, plural, one {# song} other {# songs}}", 0, 0); got != "0 morceau" {
		t.Errorf("French has one for 0: %q", got)
	}
	// A message Apple sent that does not parse is left out, and logged
	// once: the English shows (a broken message of our own would panic
	// under test).
	if got := T("Broken"); got != "Broken" {
		t.Errorf("Apple's broken message: %q", got)
	}
	if got := T("Broken"); got != "Broken" || strings.Count(logs.String(), "does not parse") != 1 {
		t.Errorf("%q, logged:\n%s", got, logs.String())
	}

	// A sibling locale follows the same layers; English leaves them.
	SetLocale("fr-CA")
	if T("Add") != "Ajouter" {
		t.Error("a sibling locale lost Apple's layer")
	}
	SetLocale("de")
	if T("Add") != "Add" || T("Songs") != "Songs" {
		t.Error("another language shows fr's layers")
	}
	SetLocale("en")
	if T("Add") != "Add" {
		t.Error("English shows a translation")
	}
	SetLocale("")
	if Locale() != "en" {
		t.Errorf("empty locale is %q", Locale())
	}

	// Apple's update of the shown locale shows at once.
	SetLocale("fr-FR")
	SetApple(appleOf("fr-FR", 0.95, map[string]string{id("Add"): "Ajouter à"}))
	if got := T("Add"); got != "Ajouter à" {
		t.Errorf("after Apple's update: %q", got)
	}
}

func TestPseudoLocale(t *testing.T) {
	reset(t)
	SetLocale(PseudoLocale)
	got := T("Albums")
	if got == "Albums" || !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("pseudo-locale shows %q", got)
	}
	if !strings.Contains(T("{0, plural, one {# album} other {# albums}}", 0, 3), "3") {
		t.Error("pseudo-locale lost the number")
	}
	SetLocale("en")
	if T("Albums") != "Albums" {
		t.Error("English after the pseudo-locale")
	}
}

// The app always sets the system's languages (SetRegional): the pseudo
// locale must still transform, as on an en-US or en-GB machine.
func TestPseudoLocaleShowsOnAnEnglishSystem(t *testing.T) {
	reset(t)
	SetRegional("en-GB", "en-US")
	SetLocale(PseudoLocale)
	if got := T("Albums"); got == "Albums" || !strings.HasPrefix(got, "[") {
		t.Errorf("pseudo-locale on an en-GB system shows %q", got)
	}
	if got := T("{0, plural, one {# album} other {# albums}}", 0, 3); got == "3 albums" || !strings.Contains(got, "3") {
		t.Errorf("pseudo-locale plural shows %q", got)
	}
}

func TestSetLocaleRedrawsThroughThePoster(t *testing.T) {
	reset(t)
	posts := 0
	SetPoster(func(fn func()) { posts++; fn() })
	SetLocale("fr")
	SetApple(appleOf("fr", 1, nil))
	SetRegional("en-GB")
	if posts != 3 {
		t.Errorf("%d redraws for three changes", posts)
	}
}

func TestTIsSafeFromManyGoroutines(t *testing.T) {
	reset(t)
	SetApple(appleOf("fr-FR", 1, map[string]string{id("Add"): "Ajouter"}))
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 200 {
				if i == 0 && j%50 == 0 {
					SetLocale([]string{"fr-FR", "en"}[j/50%2])
				}
				got := T("Add")
				if got != "Add" && got != "Ajouter" {
					t.Errorf("got %q", got)
				}
				_ = T("{0, plural, one {# album} other {# albums}}", 0, j)
			}
		}()
	}
	wg.Wait()
}

func TestFormatters(t *testing.T) {
	reset(t)
	day := time.Date(2026, 11, 3, 21, 30, 0, 0, time.UTC)
	if got := FormatNumber(1234.5); got != "1,234.5" {
		t.Errorf("en number: %q", got)
	}
	if got := FormatDate(day, DateMedium); got != "Nov 3, 2026" {
		t.Errorf("en date: %q", got)
	}
	if got := FormatDate(day, DateFull); got != "Tuesday, November 3, 2026" {
		t.Errorf("en full date: %q", got)
	}
	if got := FormatClock(day); got != "9:30 PM" {
		t.Errorf("en clock: %q", got)
	}

	// An English UI on a British system keeps its date and clock.
	SetRegional("en-GB", "de-DE")
	if got := FormatDate(day, DateMedium); got != "3 Nov 2026" {
		t.Errorf("en-GB date: %q", got)
	}
	if got := FormatClock(day); got != "21:30" {
		t.Errorf("en-GB clock: %q", got)
	}
	if !slices.Equal(Locales(), []string{"en-GB", "en"}) {
		t.Errorf("locales %v", Locales())
	}
	SetRegional()

	SetLocale("de")
	if got := FormatNumber(1234.5); got != "1.234,5" {
		t.Errorf("de number: %q", got)
	}
	if got := FormatDate(day, DateLong); got != "3. November 2026" {
		t.Errorf("de date: %q", got)
	}
}

func TestFormatBytes(t *testing.T) {
	for n, want := range map[int64]string{-5: "0 B", 0: "0 B", 42: "42 B", 1023: "1,023 B", 1024: "1 kB", 1536: "1.5 kB", 12 * 1024: "12 kB", 12_700_000: "12 MB", 3 << 30: "3 GB"} {
		if got := FormatBytes(n); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                    "0:00",
		0:                               "0:00",
		999 * time.Millisecond:          "0:00",
		5 * time.Second:                 "0:05",
		65 * time.Second:                "1:05",
		59*time.Minute + 59*time.Second: "59:59",
		time.Hour:                       "1:00:00",
		3*time.Hour + 7*time.Minute + 9*time.Second: "3:07:09",
	} {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestList(t *testing.T) {
	reset(t)
	abc := []string{"Björk", "Radiohead", "Portishead"}
	for _, tc := range []struct {
		locale string
		items  []string
		want   string
	}{
		{"en", nil, ""},
		{"en", abc[:1], "Björk"},
		{"en", abc[:2], "Björk and Radiohead"},
		{"en", abc, "Björk, Radiohead, and Portishead"},
		{"en-GB", abc, "Björk, Radiohead and Portishead"},
		{"fr", abc, "Björk, Radiohead et Portishead"},
		{"de-DE", abc[:2], "Björk und Radiohead"},
		{"ja", abc, "Björk、Radiohead、Portishead"},
		{"xx", abc, "Björk, Radiohead, and Portishead"}, // no data: English's
	} {
		SetLocale(tc.locale)
		if got := List(tc.items); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.locale, got, tc.want)
		}
	}
	SetLocale("en")
	if got := ListOr(abc); got != "Björk, Radiohead, or Portishead" {
		t.Errorf("or: %q", got)
	}
	if got := ListOr(abc[:2]); got != "Björk or Radiohead" {
		t.Errorf("or two: %q", got)
	}
}

func TestCollator(t *testing.T) {
	reset(t)
	names := []string{"z", "Z", "ä", "a", "B", "b"}
	sorted := slices.Clone(names)
	SetLocale("de")
	c := Collator()
	slices.SortFunc(sorted, c.CompareString)
	if want := []string{"a", "ä", "b", "B", "z", "Z"}; !slices.Equal(sorted, want) {
		t.Errorf("de order %q, want %q", sorted, want)
	}
	SetLocale("sv") // ä sorts after z
	sorted = slices.Clone(names)
	c = Collator()
	slices.SortFunc(sorted, c.CompareString)
	if want := []string{"a", "b", "B", "z", "Z", "ä"}; !slices.Equal(sorted, want) {
		t.Errorf("sv order %q, want %q", sorted, want)
	}
}

// codesIn reads every ErrorCode constant of services/errors.go.
func codesIn(t *testing.T) []services.ErrorCode {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../../services/errors.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var codes []services.ErrorCode
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "ErrorCode" {
				continue
			}
			for _, v := range vs.Values {
				s, err := strconv.Unquote(v.(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				codes = append(codes, services.ErrorCode(s))
			}
		}
	}
	return codes
}

func TestErrorTextWordsEveryServicesCode(t *testing.T) {
	reset(t)
	codes := codesIn(t)
	if len(codes) < 40 {
		t.Fatalf("found %d error codes", len(codes))
	}
	generic := ErrorText(services.CodeUnknown)
	if generic != "Something went wrong." || ErrorText("") != generic || ErrorText("not_a_code") != generic {
		t.Errorf("generic text %q", generic)
	}
	var en po.File
	f, err := po.Parse(locales.SourceCatalogue)
	if err != nil {
		t.Fatal(err)
	}
	en = *f
	known := map[string]bool{}
	for _, e := range en.Entries {
		known[e.ID] = true
	}
	seen := map[string]services.ErrorCode{}
	for _, code := range codes {
		text := ErrorText(code)
		switch {
		case text == "":
			t.Errorf("%s has no text", code)
		case code != services.CodeUnknown && text == generic:
			t.Errorf("%s falls through to the generic text", code)
		case !known[text]:
			t.Errorf("%s: %q is not a message of en.po", code, text)
		}
		if other, dup := seen[text]; dup {
			t.Errorf("%s and %s share %q", code, other, text)
		}
		seen[text] = code
	}
}

func TestErrorTextIsTranslated(t *testing.T) {
	reset(t)
	SetApple(appleOf("fr-FR", 1, map[string]string{id("Enter a name."): "Saisissez un nom."}))
	SetLocale("fr-FR")
	if got := ErrorText(services.CodeNameRequired); got != "Saisissez un nom." {
		t.Errorf("translated: %q", got)
	}
	if got := ErrorText(services.CodeNotFound); !strings.HasPrefix(got, "It couldn't be found") {
		t.Errorf("English fills in: %q", got)
	}
}

func TestErrorMessage(t *testing.T) {
	reset(t)
	if ErrorMessage(nil) != "" {
		t.Error("nil")
	}
	if got := ErrorMessage(services.ErrNameRequired); got != "Enter a name." {
		t.Errorf("a service error is worded by its code: %q", got)
	}
}
