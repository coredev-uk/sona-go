package main

import (
	"strings"
	"testing"
)

func mustSona(t *testing.T, id string) message {
	t.Helper()
	m, ok := parseSona(id)
	if !ok {
		t.Fatalf("parseSona(%q) not ok", id)
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
		if _, ok := parseSona(id); ok {
			t.Errorf("parseSona(%q) ok, want refused", id)
		}
	}
}

func TestToICUPlainRenamesPlaceholders(t *testing.T) {
	sona := mustSona(t, "More from {curatorName}")
	apple, _ := appleMessage(appleBundle{"K": "Plus de @@curator@@"}, "K", false)
	got, err := toICU(apple, sona, map[string]string{"curator": "curatorName"})
	if err != nil || got != "Plus de {curatorName}" {
		t.Fatalf("toICU = %q, %v", got, err)
	}
}

func TestToICUPluralKeepsAppleCategoriesAndUsesHash(t *testing.T) {
	sona := mustSona(t, "{n, plural, one {# song} other {# songs}}")
	b := appleBundle{
		"K.one":   "@@count@@ песня",
		"K.few":   "@@count@@ песни",
		"K.many":  "@@count@@ песен",
		"K.other": "Песен: @@count@@",
	}
	apple, ok := appleMessage(b, "K", true)
	if !ok {
		t.Fatal("plural key not found")
	}
	got, err := toICU(apple, sona, map[string]string{"count": "n"})
	want := "{n, plural, one {# песня} few {# песни} many {# песен} other {Песен: #}}"
	if err != nil || got != want {
		t.Fatalf("toICU = %q, %v; want %q", got, err, want)
	}
}

func TestToICUEscapesSyntaxCharacters(t *testing.T) {
	sona := mustSona(t, "{n, plural, one {# video} other {# videos}}")
	b := appleBundle{"K.one": "@@count@@ {video} #1", "K.other": "@@count@@ video's"}
	apple, _ := appleMessage(b, "K", true)
	got, err := toICU(apple, sona, map[string]string{"count": "n"})
	want := "{n, plural, one {# '{'video'}' '#'1} other {# video''s}}"
	if err != nil || got != want {
		t.Fatalf("toICU = %q, %v; want %q", got, err, want)
	}
	plain := mustSona(t, "Music videos")
	apple, _ = appleMessage(appleBundle{"P": "Muziekvideo's #1"}, "P", false)
	if got, _ := toICU(apple, plain, nil); got != "Muziekvideo''s #1" {
		t.Fatalf("plain toICU = %q", got)
	}
}

func TestToICURefusesUnknownPlaceholdersAndShapeMismatch(t *testing.T) {
	sona := mustSona(t, "Play {name}")
	apple, _ := appleMessage(appleBundle{"K": "@@content@@ von @@artist@@ abspielen"}, "K", false)
	if _, err := toICU(apple, sona, map[string]string{"content": "name"}); err == nil || !strings.Contains(err.Error(), "artist") {
		t.Fatalf("err = %v, want unknown @@artist@@", err)
	}
	plural := mustSona(t, "{n, plural, one {# song} other {# songs}}")
	if _, err := toICU(apple, plural, nil); err == nil {
		t.Fatal("plain Apple key accepted for a plural message")
	}
	if _, ok := appleMessage(appleBundle{"K.one": "1 Song"}, "K", true); ok {
		t.Fatal("plural without .other accepted")
	}
}
