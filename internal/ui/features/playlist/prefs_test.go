package playlist

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func TestPrefsDefaults(t *testing.T) {
	p := newPrefs(config.Sidebar{}, nil)
	if p.FolderOpen("f.a") {
		t.Error("folders start closed")
	}
	if p.Collapsed("made-for-you") {
		t.Error("sections start open")
	}
}

func TestPrefsSaveGetsEveryChange(t *testing.T) {
	var saved []config.Sidebar
	p := newPrefs(config.Sidebar{}, func(s config.Sidebar) { saved = append(saved, s) })
	p.SetFolderOpen("f.a", true)
	p.SetFolderOpen("f.a", true) // no change, nothing saved
	p.SetFolderOpen("f.b", true)
	p.SetFolderOpen("f.b", false)
	p.SetCollapsed("apple", true)
	if len(saved) != 4 {
		t.Fatalf("saved %d times, want 4: %+v", len(saved), saved)
	}
	last := saved[3]
	if !slices.Equal(last.OpenFolders, []string{"f.a"}) || !slices.Equal(last.FoldedSections, []string{"apple"}) {
		t.Errorf("last save %+v", last)
	}
}

func TestPrefsRoundTripThroughTheSettings(t *testing.T) {
	var held config.Sidebar
	p := newPrefs(held, func(s config.Sidebar) { held = s })
	p.SetFolderOpen("f.a", true)
	p.SetCollapsed("apple", true)

	q := newPrefs(held, nil)
	if !q.FolderOpen("f.a") || q.FolderOpen("f.b") {
		t.Errorf("open folders: a=%v b=%v", q.FolderOpen("f.a"), q.FolderOpen("f.b"))
	}
	if !q.Collapsed("apple") || q.Collapsed("own") {
		t.Errorf("collapsed: apple=%v own=%v", q.Collapsed("apple"), q.Collapsed("own"))
	}
}

func TestSidebarSettingsAreNormalisedAndLocal(t *testing.T) {
	s := config.Settings{Sidebar: config.Sidebar{OpenFolders: []string{"b", "", "a", "b"}, FoldedSections: nil}}.Normalized()
	if !slices.Equal(s.Sidebar.OpenFolders, []string{"a", "b"}) || len(s.Sidebar.FoldedSections) != 0 {
		t.Errorf("normalised %+v", s.Sidebar)
	}
	doc, err := config.Export(config.Settings{Sidebar: config.Sidebar{OpenFolders: []string{"a"}}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(doc, []byte("openFolders")) {
		t.Errorf("the export holds the sidebar's state: %s", doc)
	}
}

// The app's prefs write through SystemService.SetSidebar: the state reaches
// the settings, and nothing else in them changes.
func TestSettingsPrefsSaveIntoTheSettings(t *testing.T) {
	h := uitest.New(t)
	sys := services.Service[services.SystemService](h.Core)
	before := sys.Settings()
	p := settingsPrefs(sys)
	p.SetFolderOpen("f.a", true)
	p.SetCollapsed("apple", true)
	deadline := time.Now().Add(5 * time.Second)
	for !slices.Equal(sys.Settings().Sidebar.OpenFolders, []string{"f.a"}) || !slices.Equal(sys.Settings().Sidebar.FoldedSections, []string{"apple"}) {
		if time.Now().After(deadline) {
			t.Fatalf("settings hold %+v", sys.Settings().Sidebar)
		}
		time.Sleep(5 * time.Millisecond)
	}
	want := before
	want.Sidebar = sys.Settings().Sidebar
	if got := sys.Settings(); !reflect.DeepEqual(got, want) {
		t.Errorf("another setting changed: %+v vs %+v", got, want)
	}
	q := settingsPrefs(sys)
	if !q.FolderOpen("f.a") || !q.Collapsed("apple") {
		t.Error("a new prefs did not read the saved state")
	}
}
