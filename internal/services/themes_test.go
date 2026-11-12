package services

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
)

func themesEnv(t *testing.T) (*ThemesService, *recorder, string, *[]string) {
	t.Helper()
	paths := config.Paths{Config: t.TempDir()}
	rec := &recorder{}
	var opened []string
	e := &Env{Deps: Deps{
		Paths: paths, Emit: rec, Logger: discard(),
		OpenURL: func(u string) error { opened = append(opened, u); return nil },
	}}
	t.Cleanup(e.close)
	s := newThemesService(e)
	s.interval = 10 * time.Millisecond
	return s, rec, paths.ThemesDir(), &opened
}

func writeTheme(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func names(files []ThemeFile) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

func (r *recorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e == name {
			n++
		}
	}
	return n
}

func TestThemesFilesListsTheFolder(t *testing.T) {
	s, _, dir, _ := themesEnv(t)
	if files, err := s.Files(context.Background()); err != nil || len(files) != 0 {
		t.Fatalf("no folder: %v, %v", files, err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("listing created the folder")
	}
	writeTheme(t, dir, "b.json", []byte(`{"b":1}`))
	writeTheme(t, dir, "A.JSON", []byte(`{"a":1}`))
	writeTheme(t, dir, "notes.txt", []byte("not a theme"))
	writeTheme(t, dir, ".hidden.json", []byte("{}"))
	if err := os.Mkdir(filepath.Join(dir, "dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "b.json"), filepath.Join(dir, "link.json")); err != nil {
		t.Skip("no symlinks:", err)
	}
	files, err := s.Files(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(files), []string{"A.JSON", "b.json", "link.json"}; !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	if !bytes.Equal(files[0].Data, []byte(`{"a":1}`)) || files[0].Size != 7 || files[0].ModTime == 0 || files[0].TooLarge {
		t.Errorf("file = %+v", files[0])
	}
	if s.Folder() != dir {
		t.Errorf("Folder = %q, want %q", s.Folder(), dir)
	}
}

func TestThemesFilesCapsSizeAndCount(t *testing.T) {
	s, _, dir, _ := themesEnv(t)
	writeTheme(t, dir, "big.json", []byte(`{"pad":"`+strings.Repeat("x", MaxThemeFileBytes)+`"}`))
	writeTheme(t, dir, "edge.json", bytes.Repeat([]byte(" "), MaxThemeFileBytes))
	files, err := s.Files(context.Background())
	if err != nil || len(files) != 2 {
		t.Fatalf("%v, %v", files, err)
	}
	if !files[0].TooLarge || len(files[0].Data) != 0 || files[1].TooLarge || len(files[1].Data) != MaxThemeFileBytes {
		t.Errorf("big %+v, at the cap %d bytes too large %v", files[0].TooLarge, len(files[1].Data), files[1].TooLarge)
	}
	for i := range maxThemeFiles + 5 {
		writeTheme(t, dir, "n"+string(rune('a'+i/26))+string(rune('a'+i%26))+".json", []byte("{}"))
	}
	files, _ = s.Files(context.Background())
	if len(files) != maxThemeFiles {
		t.Errorf("%d files read, want the first %d", len(files), maxThemeFiles)
	}
}

func TestThemesFilesMarksAnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	s, _, dir, _ := themesEnv(t)
	writeTheme(t, dir, "locked.json", []byte("{}"))
	if err := os.Chmod(filepath.Join(dir, "locked.json"), 0); err != nil {
		t.Fatal(err)
	}
	files, err := s.Files(context.Background())
	if err != nil || len(files) != 1 || !files[0].Unreadable || len(files[0].Data) != 0 {
		t.Errorf("%+v, %v", files, err)
	}
}

func TestThemesOpenFolderCreatesItAndOpensIt(t *testing.T) {
	s, _, dir, opened := themesEnv(t)
	if err := s.OpenFolder(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if len(*opened) != 1 || (*opened)[0] != "file://"+dir {
		t.Errorf("opened %v", *opened)
	}
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestThemesPollEmitsWhenTheFolderChanges(t *testing.T) {
	s, rec, dir, _ := themesEnv(t)
	s.start()
	time.Sleep(50 * time.Millisecond)
	if n := rec.count(EventThemesChanged); n != 0 {
		t.Fatalf("an empty folder announced %d times", n)
	}
	writeTheme(t, dir, "mine.json", []byte(`{"v":1}`))
	eventually(t, "the new file", func() bool { return rec.count(EventThemesChanged) == 1 })
	rec.mu.Lock()
	files, _ := rec.data[len(rec.data)-1].([]ThemeFile)
	rec.mu.Unlock()
	if len(files) != 1 || files[0].Name != "mine.json" || string(files[0].Data) != `{"v":1}` {
		t.Fatalf("payload %+v", files)
	}
	// Nothing changes, nothing is announced.
	time.Sleep(80 * time.Millisecond)
	if n := rec.count(EventThemesChanged); n != 1 {
		t.Errorf("unchanged folder announced again: %d", n)
	}
	// An edit (same size, later time) is a change.
	path := filepath.Join(dir, "mine.json")
	if err := os.WriteFile(path, []byte(`{"v":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the edit", func() bool { return rec.count(EventThemesChanged) == 2 })
	// A file listed by the UI is the baseline: no echo for what it already holds.
	if _, err := s.Files(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if n := rec.count(EventThemesChanged); n != 2 {
		t.Errorf("announced a listed folder: %d", n)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the removal", func() bool { return rec.count(EventThemesChanged) == 3 })
	rec.mu.Lock()
	files, _ = rec.data[len(rec.data)-1].([]ThemeFile)
	rec.mu.Unlock()
	if len(files) != 0 {
		t.Errorf("removal payload %+v", files)
	}
}

func TestThemesPollStopsWhenTheAppCloses(t *testing.T) {
	s, rec, dir, _ := themesEnv(t)
	s.start()
	s.e.close()
	writeTheme(t, dir, "late.json", []byte("{}"))
	time.Sleep(80 * time.Millisecond)
	if n := rec.count(EventThemesChanged); n != 0 {
		t.Errorf("announced after close: %d", n)
	}
	s.start() // a start after close does nothing
}
