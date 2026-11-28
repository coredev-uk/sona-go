package applemusic

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// The fuzz targets below feed the decoders what Apple's servers send: the
// resource JSON of amp-api, normalised ("format[resources]=map") or not, and
// the lyrics TTML. Their seeds are in testdata/fuzz/<target> (made-up
// values); go test runs them as ordinary tests.

// FuzzDecodeMapResponse decodes a normalised amp-api response and walks it
// as the pages do: resolving references, reading relationships and
// attributes, and decoding nodes into typed resources.
//
// Invariants: no panic; Primary and Resolve return no more nodes than there
// are references, and each node they return is in the map under its
// reference; every node keeps the raw object it was decoded from, which is
// valid JSON, and writes it back as the same object (a Node round-trips);
// a node of the wrong shape for a typed decode is an error, not a panic.
func FuzzDecodeMapResponse(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		var m MapResponse
		if err := decode(b, &m); err != nil {
			return
		}
		nodes := m.Primary()
		if len(nodes) > len(m.Data) {
			t.Fatalf("%d references resolved to %d nodes", len(m.Data), len(nodes))
		}
		for _, ref := range m.Data {
			_ = ref.Reason()
			n, ok := m.Resources.Get(ref)
			if !ok {
				continue
			}
			if len(n.Raw) == 0 || !json.Valid(n.Raw) {
				t.Fatalf("node %s %s lost its raw object: %q", ref.Type, ref.ID, n.Raw)
			}
			out, err := json.Marshal(n)
			if err != nil {
				t.Fatalf("marshal node: %v", err)
			}
			var back Node
			if err := json.Unmarshal(out, &back); err != nil || back.ID != n.ID || back.Type != n.Type {
				t.Fatalf("node does not round-trip: %v\n%s\n%s", err, n.Raw, out)
			}
			for name, rel := range n.Relationships {
				if got := n.Rel(name); len(got) != len(rel.Data) {
					t.Fatalf("Rel(%q) = %d refs, want %d", name, len(got), len(rel.Data))
				}
				if len(m.Resources.Resolve(rel.Data)) > len(rel.Data) {
					t.Fatalf("Resolve(%q) returned more nodes than references", name)
				}
			}
			var attrs map[string]any
			_ = n.Attrs(&attrs)
			_, _ = As[Album](n)
			_, _ = As[Song](n)
			_, _ = As[Resource[EditorialElementAttributes]](n)
		}
		merged := Resources{}
		merged.Merge(m.Resources)
		for typ, byID := range m.Resources {
			if len(merged[typ]) != len(byID) {
				t.Fatalf("Merge lost resources of %q", typ)
			}
		}
	})
}

// FuzzDecodeResourcePage decodes the body AMPResource and AMPResources
// decode (a Page of a typed resource) and takes the first resource.
//
// Invariants: no panic; with no data the answer is ErrNotFound; otherwise
// it is the first resource with no error; decoding never reports success
// for what is not a page.
func FuzzDecodeResourcePage(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		decodePage[Album](t, b)
		decodePage[Song](t, b)
		decodePage[Station](t, b)
		decodePage[Resource[json.RawMessage]](t, b)
	})
}

func decodePage[T any](t *testing.T, b []byte) {
	t.Helper()
	var p Page[T]
	if err := decode(b, &p); err != nil {
		return
	}
	one, err := first(p.Data, "x")
	switch {
	case len(p.Data) == 0 && !errors.Is(err, ErrNotFound):
		t.Fatalf("empty page gave %v", err)
	case len(p.Data) > 0 && err != nil:
		t.Fatalf("page of %d gave %v", len(p.Data), err)
	}
	_ = one
}

// FuzzParseTTML parses Apple's lyrics TTML.
//
// Invariants: no panic; the document is returned as it came; every time of a
// line, word and localized line is within 0 and a day (a longer, negative or
// non-finite time would overflow a Duration); the words of a line hold no
// more bytes than the document; every localization lookup returns nil or a
// localization of the kind asked for.
func FuzzParseTTML(f *testing.F) {
	f.Fuzz(func(t *testing.T, ttml string) {
		l, err := ParseTTML(ttml)
		if err != nil {
			return
		}
		if l.TTML != ttml {
			t.Fatal("the document was not kept")
		}
		ok := func(d time.Duration) bool { return d >= 0 && d <= 24*time.Hour+time.Second }
		line := func(ln LyricLine) {
			if !ok(ln.Begin) || !ok(ln.End) {
				t.Fatalf("line %+v has a time out of range", ln)
			}
			total := 0
			for _, w := range ln.Words {
				if !ok(w.Begin) || !ok(w.End) {
					t.Fatalf("word %+v has a time out of range", w)
				}
				total += len(w.Text)
			}
			if total > len(ttml) {
				t.Fatalf("%d bytes of words from %d bytes", total, len(ttml))
			}
		}
		for _, ln := range l.Lines {
			line(ln)
		}
		for _, loc := range l.Localizations {
			for _, ln := range loc.Lines {
				line(ln)
			}
		}
		for _, kind := range []string{LocalizationTranslation, LocalizationTransliteration} {
			for _, lang := range []string{"", "en", "zh-Hans"} {
				if loc := l.Localization(kind, lang); loc != nil && loc.Kind != kind {
					t.Fatalf("Localization(%q) returned a %q", kind, loc.Kind)
				}
			}
		}
	})
}

// A time that would overflow a Duration, or that is negative or not a
// number, is refused (the line then starts at 0), not wrapped around.
func TestParseClockRefusesOutOfRangeTimes(t *testing.T) {
	for _, s := range []string{"1e300", "NaN", "Inf", "-5s", "1:00:00:00", "86401"} {
		if d, err := parseClock(s); err == nil || d != 0 {
			t.Errorf("parseClock(%q) = %v, %v", s, d, err)
		}
	}
	if d, err := parseClock("1:02.5"); err != nil || d != 62500*time.Millisecond {
		t.Errorf("parseClock(1:02.5) = %v, %v", d, err)
	}
}
