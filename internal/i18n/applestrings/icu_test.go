package applestrings

import (
	"strings"
	"testing"
)

func mustSona(t *testing.T, id string) Message {
	t.Helper()
	m, ok := ParseSona(id)
	if !ok {
		t.Fatalf("ParseSona(%q) not ok", id)
	}
	return m
}

func TestParseSonaRefusesWhatOneKeyCannotHold(t *testing.T) {
	for _, id := range []string{
		"{0, plural, one {# album} other {# albums}}, {1, plural, one {# song} other {# songs}} in your library",
		"{n, plural, =0 {No songs} one {# song} other {# songs}}",
		"{kind, select, album {Album} other {Playlist}}",
		"{n, plural, one {# song}}",
		"Songs: {n, plural, one {#} other {#}}",
		"It'{s} quoted",
	} {
		if _, ok := ParseSona(id); ok {
			t.Errorf("ParseSona(%q) ok, want refused", id)
		}
	}
}

func TestToICUPlainRenamesPlaceholders(t *testing.T) {
	sona := mustSona(t, "More from {curatorName}")
	apple, _ := AppleMessage(Bundle{"K": "Plus de @@curator@@"}, "K", false)
	got, err := ToICU(apple, sona, map[string]string{"curator": "curatorName"})
	if err != nil || got != "Plus de {curatorName}" {
		t.Fatalf("ToICU = %q, %v", got, err)
	}
}

func TestToICUPluralKeepsAppleCategoriesAndUsesHash(t *testing.T) {
	sona := mustSona(t, "{n, plural, one {# song} other {# songs}}")
	b := Bundle{
		"K.one":   "@@count@@ песня",
		"K.few":   "@@count@@ песни",
		"K.many":  "@@count@@ песен",
		"K.other": "Песен: @@count@@",
	}
	apple, ok := AppleMessage(b, "K", true)
	if !ok {
		t.Fatal("plural key not found")
	}
	got, err := ToICU(apple, sona, map[string]string{"count": "n"})
	want := "{n, plural, one {# песня} few {# песни} many {# песен} other {Песен: #}}"
	if err != nil || got != want {
		t.Fatalf("ToICU = %q, %v; want %q", got, err, want)
	}
}

func TestToICUEscapesSyntaxCharacters(t *testing.T) {
	sona := mustSona(t, "{n, plural, one {# video} other {# videos}}")
	b := Bundle{"K.one": "@@count@@ {video} #1", "K.other": "@@count@@ video's"}
	apple, _ := AppleMessage(b, "K", true)
	got, err := ToICU(apple, sona, map[string]string{"count": "n"})
	want := "{n, plural, one {# '{'video'}' '#'1} other {# video''s}}"
	if err != nil || got != want {
		t.Fatalf("ToICU = %q, %v; want %q", got, err, want)
	}
	plain := mustSona(t, "Music videos")
	apple, _ = AppleMessage(Bundle{"P": "Muziekvideo's #1"}, "P", false)
	if got, _ := ToICU(apple, plain, nil); got != "Muziekvideo''s #1" {
		t.Fatalf("plain ToICU = %q", got)
	}
}

func TestToICURefusesUnknownPlaceholdersAndShapeMismatch(t *testing.T) {
	sona := mustSona(t, "Play {name}")
	apple, _ := AppleMessage(Bundle{"K": "@@content@@ von @@artist@@ abspielen"}, "K", false)
	if _, err := ToICU(apple, sona, map[string]string{"content": "name"}); err == nil || !strings.Contains(err.Error(), "artist") {
		t.Fatalf("err = %v, want unknown @@artist@@", err)
	}
	plural := mustSona(t, "{n, plural, one {# song} other {# songs}}")
	if _, err := ToICU(apple, plural, nil); err == nil {
		t.Fatal("plain Apple key accepted for a plural message")
	}
	if _, ok := AppleMessage(Bundle{"K.one": "1 Song"}, "K", true); ok {
		t.Fatal("plural without .other accepted")
	}
}

func TestToICURefusesDroppedArguments(t *testing.T) {
	sona := mustSona(t, "Play {name}")
	apple, _ := AppleMessage(Bundle{"K": "Lecture"}, "K", false)
	if _, err := ToICU(apple, sona, map[string]string{"title": "name"}); err == nil || !strings.Contains(err.Error(), "{name}") {
		t.Fatalf("err = %v, want {name} missing", err)
	}
	plural := mustSona(t, "{n, plural, one {# song} other {# songs}}")
	// A `one` form may spell the number out; `other` must show it.
	apple, _ = AppleMessage(Bundle{"K.one": "Un morceau", "K.other": "@@count@@ morceaux"}, "K", true)
	if got, err := ToICU(apple, plural, map[string]string{"count": "n"}); err != nil || got != "{n, plural, one {Un morceau} other {# morceaux}}" {
		t.Fatalf("ToICU = %q, %v", got, err)
	}
	apple, _ = AppleMessage(Bundle{"K.one": "@@count@@ morceau", "K.other": "Des morceaux"}, "K", true)
	if _, err := ToICU(apple, plural, map[string]string{"count": "n"}); err == nil {
		t.Fatal("other form without the count accepted")
	}
	named := mustSona(t, "{n, plural, one {# song by {artist}} other {# songs by {artist}}}")
	apple, _ = AppleMessage(Bundle{"K.one": "@@count@@ morceau", "K.other": "@@count@@ morceaux"}, "K", true)
	if _, err := ToICU(apple, named, map[string]string{"count": "n"}); err == nil || !strings.Contains(err.Error(), "{artist}") {
		t.Fatalf("err = %v, want {artist} missing", err)
	}
}
