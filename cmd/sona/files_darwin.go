//go:build darwin

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
)

// macFiles is the native file chooser (NSOpenPanel and NSSavePanel through
// MyGo's Dialog) for the settings export and import. It is a
// services.FileChooser. A panel is a sheet of Sona's main window while that
// is shown, so it drops from the window's title bar and moves with it; with
// no main window shown (the mini player, a hidden window) it is application
// modal. MyGo can't close a panel from code, so a cancelled ctx returns at
// once and leaves the panel to the user, whose answer is then dropped.
type macFiles struct {
	// parent is the window the panel attaches to (nil: none).
	parent func() *mygo.Window
	open   func(mygo.OpenDialogOptions) ([]string, error)
	save   func(mygo.SaveDialogOptions) (string, error)
}

// newMacFiles is macFiles over MyGo's dialogs, attached to the window parent
// returns.
func newMacFiles(parent func() *mygo.Window) macFiles {
	return macFiles{parent: parent, open: mygo.Dialog.Open, save: mygo.Dialog.Save}
}

// window is the window a panel attaches to now, nil for none.
func (f macFiles) window() *mygo.Window {
	if f.parent == nil {
		return nil
	}
	return f.parent()
}

// OpenFile asks for one file; "" and no error when the user cancels.
func (f macFiles) OpenFile(ctx context.Context, o app.OpenFileOptions) (string, error) {
	parent := f.window()
	paths, err := ask(ctx, func() ([]string, error) {
		return f.open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       o.Title,
			ButtonLabel: o.AcceptLabel,
			DefaultPath: o.Folder,
			Filters:     dialogFilters(o.Filters),
		})
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// SaveFile asks where to save a file named o.Name; "" and no error when the
// user cancels. The panel confirms replacing a file; it doesn't create one.
func (f macFiles) SaveFile(ctx context.Context, o app.SaveFileOptions) (string, error) {
	parent := f.window()
	return ask(ctx, func() (string, error) {
		return f.save(mygo.SaveDialogOptions{
			Parent:      parent,
			Title:       o.Title,
			ButtonLabel: o.AcceptLabel,
			DefaultPath: savePath(o.Folder, o.Name),
			Filters:     dialogFilters(o.Filters),
		})
	})
}

// ask runs a blocking panel off the caller's goroutine and gives up when ctx
// ends.
func ask[T any](ctx context.Context, show func() (T, error)) (T, error) {
	type answer struct {
		v   T
		err error
	}
	ch := make(chan answer, 1)
	go func() {
		v, err := show()
		ch <- answer{v, err}
	}()
	select {
	case a := <-ch:
		return a.v, a.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// savePath is the panel's starting point: name in folder, or in Documents
// (the home folder where there is none) when no folder is given. A panel
// started with a bare name would open in the current directory, which for an
// app is the root of the disk.
func savePath(folder, name string) string {
	if name == "" {
		return folder
	}
	if folder == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return name
		}
		folder = home
		if docs := filepath.Join(home, "Documents"); isDir(docs) {
			folder = docs
		}
	}
	return filepath.Join(folder, name)
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// dialogFilters turns glob filters ("*.json") into MyGo's file types
// (extensions). A glob that is not "*.<extension>" (a name pattern like
// "sona*", which a macOS panel can't express) is left out, and a filter
// with no glob left is too; "*" and "*.*" lift the restriction.
func dialogFilters(in []app.FileFilter) []mygo.FileFilter {
	var out []mygo.FileFilter
	for _, f := range in {
		var exts []string
		for _, p := range f.Patterns {
			switch ext, ok := strings.CutPrefix(p, "*."); {
			case p == "*" || p == "*.*":
				exts = append(exts, "*")
			case ok && ext != "" && !strings.ContainsAny(ext, "*?[]"):
				exts = append(exts, ext)
			}
		}
		if len(exts) > 0 {
			out = append(out, mygo.FileFilter{Name: f.Name, Extensions: exts})
		}
	}
	return out
}
