//go:build darwin

package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
)

func TestInAppBundle(t *testing.T) {
	for path, want := range map[string]bool{
		"/Applications/Sona.app/Contents/MacOS/sona":    true,
		"/tmp/Sona dev.app/Contents/MacOS/sona-dev":     true,
		"/Users/me/sona/bin/sona":                       false, // a source build
		"/Users/me/sona/bin/Sona.app/sona":              false,
		"/Applications/Sona.app/Contents/Resources/sna": false,
		"/Applications/Sona/Contents/MacOS/sona":        false, // no .app
		"/Applications/Sona.app/MacOS/sona":             false, // no Contents
	} {
		if got := inAppBundle(path); got != want {
			t.Errorf("inAppBundle(%q) = %v, want %v", path, got, want)
		}
	}
}

// The test binary is a bare executable: this is the answer outside Sona.app.
func TestRunsFromAppBundleOutsideTheBundle(t *testing.T) {
	if runsFromAppBundle() {
		t.Error("a test binary reports it runs from an app bundle")
	}
}

func TestKeeperHoldsOneAppNapAssertionAtATime(t *testing.T) {
	var reasons []string
	held := 0
	k := newMacKeeper(func() string { return "Playing music" })
	k.take = func(reason string) func() {
		reasons = append(reasons, reason)
		held++
		return func() { held-- }
	}
	k.Release()   // nothing held: harmless
	for range 2 { // a second Hold keeps the same assertion
		k.Hold()
	}
	if held != 1 || !slices.Equal(reasons, []string{"Playing music"}) {
		t.Fatalf("after Hold: held=%d reasons=%v", held, reasons)
	}
	for range 2 {
		k.Release()
	}
	if held != 0 {
		t.Fatalf("after Release: held=%d", held)
	}
	// The reason is read each time it is taken, so it follows the language.
	k.why = func() string { return "Musique en cours" }
	k.Hold()
	k.Close()
	if held != 0 || !slices.Equal(reasons, []string{"Playing music", "Musique en cours"}) {
		t.Fatalf("after Close: held=%d reasons=%v", held, reasons)
	}
}

// A bare binary isn't an app bundle, so macOS has no application of Sona's to
// make the handler: both calls say it can't be done, they don't pretend.
func TestLinkHandlerOutsideABundleIsUnsupported(t *testing.T) {
	h := macLinks{}
	if _, _, err := h.Default(); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("Default outside a bundle: %v, want ErrUnsupported", err)
	}
	if err := h.MakeDefault(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("MakeDefault outside a bundle: %v, want ErrUnsupported", err)
	}
}

func TestOpenedBy(t *testing.T) {
	const self = "dev.coredev.Sona"
	music := mygo.InstalledApp{Name: "Music", ID: "com.apple.Music", Path: "/System/Applications/Music.app"}
	sona := mygo.InstalledApp{Name: "Sona", ID: self}
	for _, tc := range []struct {
		name      string
		byScheme  func(scheme string) (mygo.InstalledApp, error)
		wantSona  bool
		wantOther string
	}{
		{"every scheme is Sona's", func(string) (mygo.InstalledApp, error) { return sona, nil }, true, ""},
		{"Music has them all", func(string) (mygo.InstalledApp, error) { return music, nil }, false, "Music"},
		{
			"the first other app is named",
			func(scheme string) (mygo.InstalledApp, error) {
				if scheme == app.LinkSchemes[0] {
					return sona, nil
				}
				return music, nil
			}, false, "Music",
		},
		{
			"none handles them",
			func(string) (mygo.InstalledApp, error) {
				return mygo.InstalledApp{}, errors.New("no application found")
			},
			false, "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotSona, gotOther := openedBy(self, tc.byScheme)
			if gotSona != tc.wantSona || gotOther != tc.wantOther {
				t.Errorf("openedBy = (%v, %q), want (%v, %q)", gotSona, gotOther, tc.wantSona, tc.wantOther)
			}
		})
	}
}

// DefaultApp is asked about each scheme's own URL.
func TestOpenedByAsksEachScheme(t *testing.T) {
	var asked []string
	openedBy("x", func(url string) (mygo.InstalledApp, error) {
		asked = append(asked, url)
		return mygo.InstalledApp{ID: "x"}, nil
	})
	if len(asked) != len(app.LinkSchemes) {
		t.Fatalf("asked %v, want one URL per scheme %v", asked, app.LinkSchemes)
	}
	for i, scheme := range app.LinkSchemes {
		if want := scheme + "://music.apple.com/"; asked[i] != want {
			t.Errorf("asked[%d] = %q, want %q", i, asked[i], want)
		}
	}
}
