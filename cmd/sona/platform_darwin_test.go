//go:build darwin

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestDialogFilters(t *testing.T) {
	json := app.FileFilter{Name: "Sona settings (JSON)", Patterns: []string{"*.json"}}
	for _, tc := range []struct {
		name string
		in   []app.FileFilter
		want []mygo.FileFilter
	}{
		{"none", nil, nil},
		{"an extension", []app.FileFilter{json}, []mygo.FileFilter{{Name: json.Name, Extensions: []string{"json"}}}},
		{
			"several",
			[]app.FileFilter{{Name: "Pictures", Patterns: []string{"*.png", "*.jpg"}}},
			[]mygo.FileFilter{{Name: "Pictures", Extensions: []string{"png", "jpg"}}},
		},
		{
			"all files",
			[]app.FileFilter{{Name: "Everything", Patterns: []string{"*"}}},
			[]mygo.FileFilter{{Name: "Everything", Extensions: []string{"*"}}},
		},
		// A panel filters by extension only: a name pattern has no meaning
		// there, and a filter left with nothing is not offered.
		{
			"a name pattern",
			[]app.FileFilter{{Name: "Sona", Patterns: []string{"sona*", "*.json"}}},
			[]mygo.FileFilter{{Name: "Sona", Extensions: []string{"json"}}},
		},
		{"nothing expressible", []app.FileFilter{{Name: "Sona", Patterns: []string{"sona*", "*.j?on", "*."}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dialogFilters(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("dialogFilters = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSavePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := savePath("/some/folder", "sona-settings.json"); got != "/some/folder/sona-settings.json" {
		t.Errorf("with a folder: %q", got)
	}
	if got := savePath("", ""); got != "" {
		t.Errorf("with nothing: %q", got)
	}
	// With no folder, the panel starts in Documents, or the home folder
	// without one, never in the current directory.
	if got := savePath("", "sona-settings.json"); got != filepath.Join(home, "sona-settings.json") {
		t.Errorf("without Documents: %q", got)
	}
	if err := os.Mkdir(filepath.Join(home, "Documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := savePath("", "sona-settings.json"); got != filepath.Join(home, "Documents", "sona-settings.json") {
		t.Errorf("with Documents: %q", got)
	}
}

// The panels attach to the window they are given as sheets, and with no
// window they are the application's.
func TestFilePanelsAttachToTheMainWindow(t *testing.T) {
	win := &mygo.Window{}
	var opened mygo.OpenDialogOptions
	var saved mygo.SaveDialogOptions
	files := macFiles{
		parent: func() *mygo.Window { return win },
		open:   func(o mygo.OpenDialogOptions) ([]string, error) { opened = o; return []string{"/tmp/a.json"}, nil },
		save:   func(o mygo.SaveDialogOptions) (string, error) { saved = o; return "/tmp/b.json", nil },
	}
	filters := []app.FileFilter{{Name: "Sona settings (JSON)", Patterns: []string{"*.json"}}}
	got, err := files.OpenFile(t.Context(), app.OpenFileOptions{Title: "Import Sona settings", AcceptLabel: "Import", Filters: filters})
	if err != nil || got != "/tmp/a.json" {
		t.Fatalf("OpenFile = %q, %v", got, err)
	}
	if opened.Parent != win || opened.Title != "Import Sona settings" || opened.ButtonLabel != "Import" || len(opened.Filters) != 1 {
		t.Errorf("open panel %+v", opened)
	}
	got, err = files.SaveFile(t.Context(), app.SaveFileOptions{Title: "Export Sona settings", AcceptLabel: "Export", Name: "sona-settings.json", Folder: "/tmp", Filters: filters})
	if err != nil || got != "/tmp/b.json" {
		t.Fatalf("SaveFile = %q, %v", got, err)
	}
	if saved.Parent != win || saved.DefaultPath != "/tmp/sona-settings.json" || saved.ButtonLabel != "Export" {
		t.Errorf("save panel %+v", saved)
	}

	// No main window (not made yet, hidden, or no parent given): unparented.
	for _, parent := range []func() *mygo.Window{nil, func() *mygo.Window { return nil }} {
		files.parent = parent
		if _, err := files.OpenFile(t.Context(), app.OpenFileOptions{}); err != nil || opened.Parent != nil {
			t.Errorf("with no window: parent %v, %v", opened.Parent, err)
		}
		if _, err := files.SaveFile(t.Context(), app.SaveFileOptions{}); err != nil || saved.Parent != nil {
			t.Errorf("with no window: parent %v, %v", saved.Parent, err)
		}
	}
}

// A cancelled panel is no file and no error.
func TestFilePanelCancelIsNoFile(t *testing.T) {
	files := macFiles{
		open: func(mygo.OpenDialogOptions) ([]string, error) { return nil, nil },
		save: func(mygo.SaveDialogOptions) (string, error) { return "", nil },
	}
	if got, err := files.OpenFile(t.Context(), app.OpenFileOptions{}); got != "" || err != nil {
		t.Errorf("OpenFile = %q, %v", got, err)
	}
	if got, err := files.SaveFile(t.Context(), app.SaveFileOptions{}); got != "" || err != nil {
		t.Errorf("SaveFile = %q, %v", got, err)
	}
}

func TestAskReturnsTheAnswerOrGivesUpWithItsContext(t *testing.T) {
	got, err := ask(context.Background(), func() (string, error) { return "/tmp/x.json", nil })
	if got != "/tmp/x.json" || err != nil {
		t.Fatalf("answer = %q, %v", got, err)
	}
	wantErr := errors.New("the panel failed")
	if _, err := ask(context.Background(), func() (string, error) { return "", wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	// A panel MyGo can't close: the caller isn't held by it once its
	// context ends.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release := make(chan struct{})
	defer close(release)
	if _, err := ask(ctx, func() (string, error) { <-release; return "", nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled ask = %v, want context.Canceled", err)
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
