//go:build darwin || windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
)

// desktopFiles is the native file chooser for the settings export and
// import, through MyGo's Dialog: NSOpenPanel and NSSavePanel on macOS, the
// shell's IFileOpenDialog and IFileSaveDialog on Windows. It is a
// services.FileChooser. While Sona's main window is shown the dialog belongs
// to it: a sheet that drops from its title bar on macOS, a dialog owned by
// it (and modal to it) on Windows. With no main window shown (the mini
// player, a hidden window) it stands alone. MyGo can't close a dialog from
// code, so a cancelled ctx returns at once and leaves the dialog to the
// user, whose answer is then dropped.
type desktopFiles struct {
	// parent is the window the dialog attaches to (nil: none).
	parent func() *mygo.Window
	open   func(mygo.OpenDialogOptions) ([]string, error)
	save   func(mygo.SaveDialogOptions) (string, error)
}

// newDesktopFiles is desktopFiles over MyGo's dialogs, attached to the
// window parent returns.
func newDesktopFiles(parent func() *mygo.Window) desktopFiles {
	return desktopFiles{parent: parent, open: mygo.Dialog.Open, save: mygo.Dialog.Save}
}

// window is the window a dialog attaches to now, nil for none.
func (f desktopFiles) window() *mygo.Window {
	if f.parent == nil {
		return nil
	}
	return f.parent()
}

// OpenFile asks for one file; "" and no error when the user cancels.
func (f desktopFiles) OpenFile(ctx context.Context, o app.OpenFileOptions) (string, error) {
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
// user cancels. The dialog confirms replacing a file; it doesn't create one.
func (f desktopFiles) SaveFile(ctx context.Context, o app.SaveFileOptions) (string, error) {
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

// ask runs a blocking dialog off the caller's goroutine and gives up when
// ctx ends.
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

// savePath is the dialog's starting point: name in folder, or in Documents
// (the home folder where there is none) when no folder is given. A dialog
// started with a bare name would open in the current directory, which for an
// app is the root of the disk (macOS) or wherever it was started from.
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
// "sona*", which MyGo's file types can't express) is left out, and a filter
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
