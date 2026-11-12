package i18n

import "testing"

func TestMetaDropsEmptyParts(t *testing.T) {
	if got := Meta("", "Album", "", "2008"); got != "Album · 2008" {
		t.Errorf("Meta = %q", got)
	}
	if got := Meta("", ""); got != "" {
		t.Errorf("Meta of nothing = %q", got)
	}
}

func TestCatalogueLine(t *testing.T) {
	if got := CatalogueLine("Album", "2008", 1, 0); got != "Album · 2008 · 1 song" {
		t.Errorf("one song = %q", got)
	}
	if got := CatalogueLine("Playlist", "", 10, 42*60_000); got != "Playlist · 10 songs, 42 min" {
		t.Errorf("with time = %q", got)
	}
	if got := CatalogueLine("Album", "", 0, 0); got != "Album" {
		t.Errorf("bare = %q", got)
	}
}
