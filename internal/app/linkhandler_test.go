//go:build !windows

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeXDG installs a desktop entry for Sona (when installed) and an xdg-mime
// that keeps its defaults in files, like the real one's mimeapps.list; seed
// maps a MIME type to the desktop file that is its default already.
func fakeXDG(t *testing.T, installed bool, seed map[string]string) {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(filepath.Join(data, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if installed {
		if err := os.WriteFile(filepath.Join(data, "applications", desktopFile), []byte("[Desktop Entry]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	for mime, entry := range seed {
		if err := os.WriteFile(filepath.Join(state, filepath.Base(mime)), []byte(entry+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
state=` + state + `
case "$1" in
query) cat "$state/$(basename "$3")" 2>/dev/null; exit 0 ;;
default) printf '%s\n' "$2" > "$state/$(basename "$3")" ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "xdg-mime"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_DATA_DIRS", filepath.Join(root, "none"))
}

func TestXDGLinkHandlerMakesSonaTheDefaultForEveryScheme(t *testing.T) {
	fakeXDG(t, true, map[string]string{"x-scheme-handler/music": "org.gnome.Music.desktop"})
	var h XDGLinkHandler
	sona, other, err := h.Default()
	if err != nil || sona || other != "org.gnome.Music" {
		t.Fatalf("before: sona=%v other=%q err=%v, want another app's entry", sona, other, err)
	}
	if err := h.MakeDefault(context.Background()); err != nil {
		t.Fatal(err)
	}
	sona, other, err = h.Default()
	if err != nil || !sona || other != "" {
		t.Fatalf("after: sona=%v other=%q err=%v, want Sona", sona, other, err)
	}
}

func TestXDGLinkHandlerIsNotTheDefaultWhileOneSchemeIsAnotherApps(t *testing.T) {
	seed := map[string]string{}
	for _, s := range LinkSchemes {
		seed[mimeType(s)] = desktopFile
	}
	seed[mimeType("itmss")] = "rhythmbox.desktop"
	fakeXDG(t, true, seed)
	sona, other, err := XDGLinkHandler{}.Default()
	if err != nil || sona || other != "rhythmbox" {
		t.Fatalf("sona=%v other=%q err=%v, want not Sona, rhythmbox", sona, other, err)
	}
}

func TestXDGLinkHandlerNeedsAnInstalledDesktopEntry(t *testing.T) {
	fakeXDG(t, false, nil)
	var h XDGLinkHandler
	if _, _, err := h.Default(); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("Default without a desktop entry: %v, want ErrUnsupported", err)
	}
	if err := h.MakeDefault(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("MakeDefault without a desktop entry: %v, want ErrUnsupported", err)
	}
}
