package settings

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// fakeStorage is StorageService without a disk to count.
type fakeStorage struct {
	mu                    sync.Mutex
	usage                 services.StorageUsage
	usageErr              error
	clear                 services.ClearResult
	freed                 int64
	usages, clears, comps int
}

func (f *fakeStorage) Usage(context.Context) (services.StorageUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usages++
	return f.usage, f.usageErr
}

func (f *fakeStorage) Clear(context.Context) (services.ClearResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clears++
	return f.clear, nil
}

func (f *fakeStorage) Compact(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comps++
	return f.freed, nil
}

func (f *fakeStorage) get(n *int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *n
}

const mebi = 1 << 20

// storageRig opens Storage on a Linux desktop with no file chooser, whatever
// the machine the test runs on (the words of "Not available on … yet." name
// the platform).
func storageRig(t *testing.T, f *fakeStorage) *rig {
	t.Helper()
	return sectionRigWith(t, "storage", func(s *settings) {
		*stateOf[storageAPI](s, "seam:storage") = f
		store.Set(s.env.Queries, store.KeyCapabilities, services.Capabilities{OS: "linux"})
	})
}

func TestStorageShowsUsageAndRows(t *testing.T) {
	f := &fakeStorage{usage: services.StorageUsage{
		ArtworkBytes: 128 * mebi, CatalogBytes: 3 * mebi, CatalogEntries: 42, DatabaseBytes: 20 * mebi, DatabaseFreeBytes: 2 * mebi,
	}}
	r := storageRig(t, f)
	r.tt.Until("usage", r.tt.Shows("128 MB", "of 512 MB", "3 MB", "42 pages", "20 MB", "2 MB of it free space",
		"Up to 512 MB", "Clear caches", "Compact now"))
	for _, id := range []string{"art-cache", "clear-cache", "compact"} {
		if !r.s.seen[id] {
			t.Errorf("no row %q", id)
		}
	}
	if !r.s.seen["settings-file"] {
		t.Error("no settings file row (nor the unavailable one)")
	}
}

func TestArtCacheLimitSavesAndRefreshesUsage(t *testing.T) {
	f := &fakeStorage{}
	r := storageRig(t, f)
	r.tt.Until("rows", r.tt.Shows("Up to 512 MB"))
	before := f.get(&f.usages)
	if err := r.tt.Click("Up to 512 MB"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.Click("Up to 1 GB"); err != nil {
		t.Fatal(err)
	}
	r.saved("limit saved", func(s config.Settings) bool { return s.Storage.ArtCacheMiB == 1024 })
	r.tt.Until("usage refreshed", func() bool { return f.get(&f.usages) > before })
	r.tt.Until("new limit shown", r.tt.Shows("Up to 1 GB"))
}

func TestClearCachesCallsTheServiceAndToasts(t *testing.T) {
	f := &fakeStorage{clear: services.ClearResult{ArtworkBytes: 5 * mebi, CatalogEntries: 1}}
	r := storageRig(t, f)
	r.tt.Until("rows", r.tt.Shows("Clear caches"))
	if err := r.tt.Click("Clear caches"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("toast", r.tt.Shows("Cleared 5 MB of artwork and 1 cached page"))
	if f.get(&f.clears) != 1 {
		t.Errorf("Clear called %d times", f.clears)
	}
}

func TestCompactNowCallsTheServiceAndToasts(t *testing.T) {
	f := &fakeStorage{freed: 4 * mebi}
	r := storageRig(t, f)
	r.tt.Until("rows", r.tt.Shows("Compact now"))
	if err := r.tt.Click("Compact now"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("toast", r.tt.Shows("Gave back 4 MB to the disk"))
	f.freed = 0
	r.tt.Until("button back", r.tt.Shows("Compact now"))
	if err := r.tt.Click("Compact now"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("toast", r.tt.Shows("The database was already compact"))
	if f.get(&f.comps) != 2 {
		t.Errorf("Compact called %d times", f.comps)
	}
}

func TestStorageErrorBlockRetries(t *testing.T) {
	f := &fakeStorage{usageErr: errors.New("boom")}
	r := storageRig(t, f)
	r.tt.Until("error", r.tt.Shows("Couldn't read Sona's storage", "Try again"))
	n := f.get(&f.usages)
	f.mu.Lock()
	f.usageErr = nil
	f.mu.Unlock()
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("rows", r.tt.Shows("Clear caches"))
	if f.get(&f.usages) <= n {
		t.Error("Retry did not refetch the usage")
	}
}

func TestSettingsFileUnavailableWithoutAChooser(t *testing.T) {
	r := storageRig(t, &fakeStorage{})
	r.tt.Until("rows", r.tt.Shows("Settings file", "Not available on Linux yet."))
}

// fileSys is the SystemService with a file chooser's answers.
type fileSys struct {
	systemAPI
	exported string
	imp      services.SettingsImport
	err      error
	calls    []string
	mu       sync.Mutex
}

func (f *fileSys) note(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fileSys) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fileSys) ExportSettings(context.Context) (string, error) {
	f.note("export")
	return f.exported, f.err
}

func (f *fileSys) ImportSettings(context.Context) (services.SettingsImport, error) {
	f.note("import")
	return f.imp, f.err
}

func fileRig(t *testing.T, fs *fileSys) *rig {
	t.Helper()
	r := storageRig(t, &fakeStorage{})
	fs.systemAPI = r.s.sys
	r.s.sys = fs
	store.Set(r.h.Env.Queries, store.KeyCapabilities, services.Capabilities{OS: "linux", FileChooser: true})
	r.tt.Until("the file row", r.tt.Shows("Export…", "Import…"))
	return r
}

func TestSettingsFileExport(t *testing.T) {
	fs := &fileSys{exported: filepath.Join("/tmp", "sona-settings.json")}
	r := fileRig(t, fs)
	r.reveal("settings-file")
	if err := r.tt.Click("Export…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("toast", r.tt.Shows("Settings exported", "Saved as sona-settings.json."))
	// A cancelled chooser says nothing.
	fs.exported = ""
	r.tt.Until("buttons back", r.tt.Shows("Export…"))
	r.reveal("settings-file")
	if err := r.tt.Click("Export…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("export asked again", func() bool { return fs.count() == 2 })
}

func TestSettingsFileExportFailureToasts(t *testing.T) {
	fs := &fileSys{err: errors.New("disk full")}
	r := fileRig(t, fs)
	r.reveal("settings-file")
	if err := r.tt.Click("Export…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("toast", r.tt.Shows("Couldn't export your settings"))
}

func TestSettingsFileImportNamesRows(t *testing.T) {
	fs := &fileSys{imp: services.SettingsImport{Path: "/tmp/a.json", Changed: []string{"theme", "audio.equalizer.preset", "audio.equalizer.custom"}}}
	r := fileRig(t, fs)
	r.reveal("settings-file")
	if err := r.tt.Click("Import…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("toast", r.tt.Shows("Imported 2 settings", "Theme, Equaliser"))
	fs.imp = services.SettingsImport{Path: "/tmp/a.json"}
	r.tt.Until("buttons back", r.tt.Shows("Import…"))
	r.reveal("settings-file")
	if err := r.tt.Click("Import…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("toast", r.tt.Shows("Nothing to change", "The settings in a.json already match yours."))
}

// The cases of settings-file.test.ts.
func TestChangedRows(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{
			[]string{"audio.equalizer.custom", "audio.equalizer.enabled", "audio.equalizer.preset", "desktop.closeAction", "theme"},
			[]string{"Equaliser", "When the window is closed", "Theme"},
		},
		{[]string{"crossfade.seconds"}, []string{"Crossfade"}},
		{[]string{"desktop.resumeLastRoute", "desktop.startPage"}, []string{"Open on"}},
		{[]string{"future.setting"}, []string{"future.setting"}},
	}
	for _, c := range cases {
		if got := changedRows(c.in); !slices.Equal(got, c.want) {
			t.Errorf("changedRows(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
