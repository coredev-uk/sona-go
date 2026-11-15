//go:build darwin || windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
)

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
	t.Setenv("HOME", home)        // os.UserHomeDir on macOS
	t.Setenv("USERPROFILE", home) // and on Windows
	folder := filepath.Join(home, "some", "folder")
	if got := savePath(folder, "sona-settings.json"); got != filepath.Join(folder, "sona-settings.json") {
		t.Errorf("with a folder: %q", got)
	}
	if got := savePath("", ""); got != "" {
		t.Errorf("with nothing: %q", got)
	}
	// With no folder, the dialog starts in Documents, or the home folder
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

// The dialogs belong to the window they are given (a sheet on macOS, an
// owned dialog on Windows), and with no window they stand alone.
func TestFilePanelsAttachToTheMainWindow(t *testing.T) {
	win := &mygo.Window{}
	var opened mygo.OpenDialogOptions
	var saved mygo.SaveDialogOptions
	files := desktopFiles{
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
	folder := t.TempDir()
	got, err = files.SaveFile(t.Context(), app.SaveFileOptions{Title: "Export Sona settings", AcceptLabel: "Export", Name: "sona-settings.json", Folder: folder, Filters: filters})
	if err != nil || got != "/tmp/b.json" {
		t.Fatalf("SaveFile = %q, %v", got, err)
	}
	if saved.Parent != win || saved.DefaultPath != filepath.Join(folder, "sona-settings.json") || saved.ButtonLabel != "Export" {
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

// A cancelled dialog is no file and no error.
func TestFilePanelCancelIsNoFile(t *testing.T) {
	files := desktopFiles{
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
