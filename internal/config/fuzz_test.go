package config

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
)

// The fuzz targets below feed the settings import what a file picked by the
// person can hold (a settings export from anywhere). Their seeds are in
// testdata/fuzz/<target>; go test runs them as ordinary tests.

// FuzzParseExport reads a settings export and applies it.
//
// Invariants: no panic; a file that is refused is ErrNotExport,
// ErrNewerExport or ErrBadValue; a file that is accepted applies to settings
// that stay valid (Normalized is idempotent on them), never changes a local
// setting (localKeys), and writes, through Export and ParseExport again,
// a document that imports to the same settings.
func FuzzParseExport(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		im, err := ParseExport(data)
		if err != nil {
			if !errors.Is(err, ErrNotExport) && !errors.Is(err, ErrNewerExport) && !errors.Is(err, ErrBadValue) {
				t.Fatalf("error %v is not one an import names", err)
			}
			return
		}
		base := custom()
		s := base
		im.Apply(&s)
		checkSettings(t, base, s)
		// Apply must not write into the settings it was given: base keeps its maps.
		if !reflect.DeepEqual(base, custom()) {
			t.Fatal("Apply changed the settings it was given through a shared map or slice")
		}
	})
}

// FuzzExportRoundTrip exports settings made of arbitrary JSON and imports
// them on another computer.
//
// Invariants: no panic; Export succeeds for any settings; the document is
// one ParseExport accepts; the import gives the normalised settings with this
// computer's local settings; the document has no local key.
func FuzzExportRoundTrip(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		from := Defaults()
		if json.Unmarshal(data, &from) != nil {
			return
		}
		doc, err := Export(from)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		im, err := ParseExport(doc)
		if err != nil {
			t.Fatalf("ParseExport(Export(s)): %v\n%s", err, doc)
		}
		var top struct {
			Settings map[string]json.RawMessage `json:"settings"`
		}
		if err := json.Unmarshal(doc, &top); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"volume", "scrobbling", "window", "onboarding", "concerts", "sidebar", "lyricsView", "addons"} {
			if _, ok := top.Settings[k]; ok {
				t.Fatalf("export holds the local setting %q", k)
			}
		}
		base := custom()
		got := base
		im.Apply(&got)
		want := local(from.Normalized(), base)
		if got = got.Normalized(); !reflect.DeepEqual(got, want) {
			t.Fatalf("imported\n%+v\nwant\n%+v", got, want)
		}
	})
}

// checkSettings holds what applying an import to base must give: valid
// settings, the local settings of base, and a stable export.
func checkSettings(t *testing.T, base, applied Settings) {
	t.Helper()
	s := applied.Normalized()
	if !reflect.DeepEqual(s.Normalized(), s) {
		t.Fatalf("Normalized is not idempotent:\n%+v\n%+v", s, s.Normalized())
	}
	if !reflect.DeepEqual(local(Settings{}, s), local(Settings{}, base)) {
		t.Fatalf("an import changed a local setting:\n%+v\n%+v", local(Settings{}, s), local(Settings{}, base))
	}
	doc, err := Export(s)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	im, err := ParseExport(doc)
	if err != nil {
		t.Fatalf("ParseExport(Export(s)): %v", err)
	}
	again := base
	im.Apply(&again)
	if again = again.Normalized(); !reflect.DeepEqual(again, s) {
		t.Fatalf("export and import changed the settings:\n%+v\n%+v", s, again)
	}
	if ch := Changes(s, again); !slices.Equal(ch, []string{}) {
		t.Fatalf("Changes of equal settings: %v", ch)
	}
}
