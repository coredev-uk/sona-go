package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestReleaseTitle(t *testing.T) {
	for _, c := range []struct{ in, title, kind string }{
		{"Abbey - Single", "Abbey", "single"},
		{"Abbey - EP", "Abbey", "ep"},
		{"Abbey", "Abbey", ""},
		{" - EP", " - EP", ""},
		{"A - Single - EP", "A - Single", "ep"},
		{"Single", "Single", ""},
	} {
		if title, kind := ReleaseTitle(c.in); title != c.title || kind != c.kind {
			t.Errorf("ReleaseTitle(%q) = %q, %q; want %q, %q", c.in, title, kind, c.title, c.kind)
		}
	}
}

func TestParseHex(t *testing.T) {
	if c, ok := ParseHex("#0a0B0c"); !ok || c != ui.RGB(10, 11, 12) {
		t.Errorf("ParseHex = %v, %v", c, ok)
	}
	for _, s := range []string{"", "#fff", "0a0b0c", "#gg0000", "#0a0b0c0d"} {
		if _, ok := ParseHex(s); ok {
			t.Errorf("ParseHex(%q) ok", s)
		}
	}
}

func TestEntry(t *testing.T) {
	var m map[string]*int
	a := Entry(&m, "a")
	*a = 3
	if b := Entry(&m, "a"); b != a || *b != 3 || len(m) != 1 {
		t.Errorf("Entry made a second value: %v", m)
	}
}

func TestPageGutter(t *testing.T) {
	if PageGutter(0) != PageGutter(PageNarrow) || PageGutter(PageNarrow-1) >= PageGutter(PageNarrow) {
		t.Errorf("gutters: 0 %v, narrow-1 %v, narrow %v", PageGutter(0), PageGutter(PageNarrow-1), PageGutter(PageNarrow))
	}
}
