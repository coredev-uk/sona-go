package config

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// v0.2.0 read a missing key as: every folder closed, every section open,
// translation and pronunciation off.
func TestWebViewPreferencesStartAsV020Did(t *testing.T) {
	s, err := LoadSettings(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sidebar.OpenFolders) != 0 || len(s.Sidebar.FoldedSections) != 0 || s.LyricsView != (LyricsView{}) {
		t.Errorf("defaults %+v %+v", s.Sidebar, s.LyricsView)
	}
}

func TestWebViewPreferencesSurviveAFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	in := Defaults()
	in.Sidebar = Sidebar{OpenFolders: []string{"f.a", "f.b"}, FoldedSections: []string{"apple"}}
	in.LyricsView = LyricsView{Translation: true}
	if err := SaveSettings(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestSidebarNormalisationSortsAndDropsEmptyAndRepeats(t *testing.T) {
	got := Sidebar{OpenFolders: []string{"b", "", "a", "b"}, FoldedSections: []string{""}}.normalized()
	if !slices.Equal(got.OpenFolders, []string{"a", "b"}) || got.FoldedSections != nil {
		t.Errorf("got %+v", got)
	}
}

// Both are this computer's view, as localStorage was: an export leaves them
// out and an import keeps the ones already here.
func TestWebViewPreferencesAreDeviceLocal(t *testing.T) {
	doc, err := Export(custom())
	if err != nil {
		t.Fatal(err)
	}
	here := Defaults()
	here.Sidebar = Sidebar{OpenFolders: []string{"mine"}}
	here.LyricsView = LyricsView{Pronunciation: true}
	im, err := ParseExport(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := here
	im.Apply(&got)
	if !reflect.DeepEqual(got.Sidebar, here.Sidebar) || got.LyricsView != here.LyricsView {
		t.Errorf("import changed them: %+v %+v", got.Sidebar, got.LyricsView)
	}
}
