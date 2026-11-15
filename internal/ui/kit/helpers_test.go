package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// hangGuard is how long a test waits for state before it calls the wait hung.
// It guards against a wake-up that never comes and is not a speed limit: a
// shared runner under -race only makes the wait longer (docs/architecture.md,
// "Timing").
const hangGuard = 30 * time.Second

// waitFor polls ok until it holds, for the things a goroutine does that
// nothing posts or signals. Prefer a channel, a post (mainThread.until) or a
// fake clock where there is one.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(hangGuard); !ok(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

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
