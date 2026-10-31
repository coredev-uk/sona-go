package cdm

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func at(tm time.Time) func() time.Time { return func() time.Time { return tm } }

// runCheck runs one update check with o's defaults filled in.
func runCheck(t *testing.T, o Options) (pointer, bool, error) {
	t.Helper()
	o, err := o.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	return check(t.Context(), o, nil)
}

// installVersion publishes version and installs it with a check.
func installVersion(t *testing.T, f *fakeServer, o Options, version string) pointer {
	t.Helper()
	f.publish(o.Platform, version, makeCRX(t, o.Platform, version))
	ptr, changed, err := runCheck(t, o)
	if err != nil {
		t.Fatalf("install %s: %v", version, err)
	}
	if !changed || ptr.Current != version {
		t.Fatalf("install %s: changed %v, current %s", version, changed, ptr.Current)
	}
	return ptr
}

// wantInstalled checks that exactly versions are installed under dir,
// with nothing left over from an install.
func wantInstalled(t *testing.T, dir string, versions ...string) {
	t.Helper()
	want := []string{lockFile}
	if len(versions) > 0 {
		want = append(want, pointerFile)
	}
	want = append(want, versions...)
	slices.Sort(want)
	if got := dirNames(t, dir); !slices.Equal(got, want) {
		t.Errorf("install directory holds %q, want %q", got, want)
	}
}

func TestEnsureInstallsTheRelease(t *testing.T) {
	f := newFakeServer(t)
	dir := filepath.Join(t.TempDir(), "widevine")
	o := testOptions(t, f, dir)
	o.Now = at(t0)
	pub := f.publish(o.Platform, "4.10.3112.0", makeCRX(t, o.Platform, "4.10.3112.0"))

	inst, err := Ensure(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dir, "4.10.3112.0", "linux_x64", "libwidevinecdm.so")
	if inst.Version != "4.10.3112.0" || inst.Path != wantPath {
		t.Fatalf("Ensure = %+v, want %s", inst, wantPath)
	}
	wantInstalled(t, dir, "4.10.3112.0")
	if got := dirNames(t, filepath.Dir(inst.Path)); !slices.Equal(got, []string{"libwidevinecdm.so", "libwidevinecdm.so.sig", manifestFile}) {
		t.Errorf("version directory holds %q", got)
	}
	if v := LibraryVersion(inst.Path); v != inst.Version {
		t.Errorf("LibraryVersion = %q", v)
	}
	if err := checkLibrary(inst.Path, o.Platform); err != nil {
		t.Error(err)
	}
	ptr := readPointer(t, dir)
	if ptr.Platform != "linux_x64" || ptr.Current != "4.10.3112.0" || ptr.Previous != "" || !ptr.CheckedAt.Equal(t0) || ptr.Failures != 0 {
		t.Errorf("pointer = %+v", ptr)
	}
	if f.count(pub.pkg) != 1 || f.count(pub.mirror) != 0 {
		t.Errorf("requests: %v", f.hits)
	}
}

func TestEnsureRefusesBadPackages(t *testing.T) {
	p := linuxAMD64(t)
	const v = "1.2.3.4"
	good := packageEntries(p, v)
	// republish replaces the manifest entry's fields after publishing.
	republish := func(f *fakeServer, crx []byte, change func(e map[string]any)) {
		pub := f.publish(p, v, crx)
		change(pub.entry)
		f.serveManifest(v, map[string]any{p.Mozilla: pub.entry})
	}
	arm := platform(t, "linux", "arm64")
	tests := []struct {
		name  string
		setup func(t *testing.T, f *fakeServer) Options
		code  string
	}{
		{"hash mismatch", func(t *testing.T, f *fakeServer) Options {
			republish(f, makeCRX(t, p, v), func(e map[string]any) {
				sum := sha512.Sum512([]byte("another package"))
				e["hashValue"] = hex.EncodeToString(sum[:])
			})
			return Options{}
		}, CodeHashMismatch},
		{"shorter than the manifest says", func(t *testing.T, f *fakeServer) Options {
			republish(f, makeCRX(t, p, v), func(e map[string]any) { e["filesize"] = e["filesize"].(int) + 10 })
			return Options{}
		}, CodeHashMismatch},
		{"longer than the manifest says", func(t *testing.T, f *fakeServer) Options {
			republish(f, makeCRX(t, p, v), func(e map[string]any) { e["filesize"] = e["filesize"].(int) - 10 })
			return Options{}
		}, CodeTooLarge},
		{"streams more than the manifest says", func(t *testing.T, f *fakeServer) Options {
			crx := makeCRX(t, p, v)
			pub := f.publish(p, v, crx)
			stream := func(w http.ResponseWriter, _ *http.Request) {
				w.(http.Flusher).Flush() // no Content-Length
				_, _ = w.Write(crx)
				_, _ = w.Write([]byte("trailing"))
			}
			f.handle(pub.pkg, stream)
			f.handle(pub.mirror, stream)
			return Options{}
		}, CodeTooLarge},
		{"signed by another component key", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, buildCRX3(t, buildZip(t, good), signing{component: otherKey()}))
			return Options{}
		}, CodeSignature},
		{"no publisher proof", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, buildCRX3(t, buildZip(t, good), signing{noPublisher: true}))
			return Options{}
		}, CodeSignature},
		{"genuine but for another trust", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, makeCRX(t, p, v))
			return Options{Trust: Widevine}
		}, CodeSignature},
		{"package of another version", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, makeCRX(t, p, "1.2.3.5"))
			return Options{}
		}, CodeArchive},
		{"another platform's library", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, buildCRX3(t, buildZip(t, replaced(good, zipEntry{name: libEntry(p), body: fakeLibrary(arm)})), signing{}))
			return Options{}
		}, CodeArchive},
		{"no component manifest", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, buildCRX3(t, buildZip(t, without(good, manifestFile)), signing{}))
			return Options{}
		}, CodeArchive},
		{"zip-slip entry", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, buildCRX3(t, buildZip(t, append(slices.Clone(good), zipEntry{name: "../../evil", body: []byte("x")})), signing{}))
			return Options{}
		}, CodeArchive},
		{"oversize library", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, buildCRX3(t, buildZip(t, replaced(good, zipEntry{name: libEntry(p), body: fakeLibrary(p), declared: maxLibraryBytes + 1})), signing{}))
			return Options{}
		}, CodeTooLarge},
		{"redirect off Google's hosts", func(t *testing.T, f *fakeServer) Options {
			pub := f.publish(p, v, makeCRX(t, p, v))
			redirect := func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://evil.test/p.crx3", http.StatusFound)
			}
			f.handle(pub.pkg, redirect)
			f.handle(pub.mirror, redirect)
			return Options{}
		}, CodeUntrustedURL},
		{"every URL fails", func(t *testing.T, f *fakeServer) Options {
			pub := f.publish(p, v, makeCRX(t, p, v))
			f.status(pub.pkg, http.StatusInternalServerError)
			f.status(pub.mirror, http.StatusNotFound)
			return Options{}
		}, CodeNetwork},
		{"manifest lacks the platform", func(t *testing.T, f *fakeServer) Options {
			f.publish(p, v, makeCRX(t, p, v))
			return Options{Platform: arm}
		}, CodeUnsupportedPlatform},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(t)
			dir := t.TempDir()
			o := testOptions(t, f, dir)
			over := tt.setup(t, f)
			if over.Trust != (Trust{}) {
				o.Trust = over.Trust
			}
			if over.Platform.GOOS != "" {
				o.Platform = over.Platform
			}
			_, err := Ensure(t.Context(), o)
			wantCode(t, err, tt.code)
			wantInstalled(t, dir)
			if f.count("evil.test/p.crx3") != 0 {
				t.Error("followed a redirect off Google's hosts")
			}
		})
	}
}

func TestEnsureFallsBackToTheMirror(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	pub := f.publish(o.Platform, "1.2.3.4", makeCRX(t, o.Platform, "1.2.3.4"))
	f.status(pub.pkg, http.StatusInternalServerError)
	inst, err := Ensure(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Version != "1.2.3.4" || f.count(pub.pkg) != 1 || f.count(pub.mirror) != 1 {
		t.Errorf("Ensure = %+v; requests %v", inst, f.hits)
	}
	wantInstalled(t, dir, "1.2.3.4")
}

func TestCheckUpdatesKeepingOnePrevious(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	installVersion(t, f, o, "1.0.0.0")
	installVersion(t, f, o, "2.0.0.0")
	// Leftovers of an interrupted install and a stray old version.
	for _, name := range []string{stagingPrefix + "123", "0.9.0.0"} {
		if err := os.MkdirAll(filepath.Join(dir, name, "linux_x64"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, partPrefix+"456.crx3"), []byte("part"), 0o600); err != nil {
		t.Fatal(err)
	}
	ptr := installVersion(t, f, o, "3.0.0.0")
	if ptr.Previous != "2.0.0.0" {
		t.Errorf("previous = %q", ptr.Previous)
	}
	wantInstalled(t, dir, "2.0.0.0", "3.0.0.0")
	if got := readPointer(t, dir); got.Current != "3.0.0.0" || got.Previous != "2.0.0.0" {
		t.Errorf("pointer = %+v", got)
	}
}

func TestCheckNeverDowngrades(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	o.Now = at(t0)
	installVersion(t, f, o, "2.0.0.0")
	downloads := f.packageRequests()
	for i, v := range []string{"1.9.9.9", "2.0.0.0", "2.0"} {
		f.publish(o.Platform, v, makeCRX(t, o.Platform, v))
		now := t0.Add(time.Duration(i+1) * time.Hour)
		o.Now = at(now)
		ptr, changed, err := runCheck(t, o)
		if err != nil || changed || ptr.Current != "2.0.0.0" {
			t.Fatalf("manifest %s: changed %v, current %s, %v", v, changed, ptr.Current, err)
		}
		if got := readPointer(t, dir); got.Current != "2.0.0.0" || !got.CheckedAt.Equal(now) {
			t.Errorf("manifest %s: pointer %+v", v, got)
		}
	}
	if n := f.packageRequests(); n != downloads {
		t.Errorf("downloaded %d more packages", n-downloads)
	}
	wantInstalled(t, dir, "2.0.0.0")
}

func TestCheckSkipsARejectedVersion(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	ptr := installVersion(t, f, o, "1.0.0.0")
	ptr.Rejected = map[string]string{"2.0.0.0": CodeLoad}
	if err := (store{root: dir, p: o.Platform}).write(ptr); err != nil {
		t.Fatal(err)
	}
	pub := f.publish(o.Platform, "2.0.0.0", makeCRX(t, o.Platform, "2.0.0.0"))
	got, changed, err := runCheck(t, o)
	if err != nil || changed || got.Current != "1.0.0.0" {
		t.Fatalf("check: changed %v, current %s, %v", changed, got.Current, err)
	}
	if f.count(pub.pkg)+f.count(pub.mirror) != 0 {
		t.Error("downloaded a rejected version")
	}
	wantInstalled(t, dir, "1.0.0.0")
}

func TestCheckLoadFailureKeepsTheWorkingVersion(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	installVersion(t, f, o, "1.0.0.0")
	f.publish(o.Platform, "2.0.0.0", makeCRX(t, o.Platform, "2.0.0.0"))
	var checked string
	o.LoadCheck = func(ctx context.Context, lib string) error {
		if _, err := os.Stat(lib); err != nil {
			t.Errorf("LoadCheck: %v", err)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("LoadCheck without a deadline")
		}
		checked = lib
		return errors.New("SIGSEGV in the probe")
	}
	_, changed, err := runCheck(t, o)
	wantCode(t, err, CodeLoad)
	if changed {
		t.Error("changed")
	}
	if want := filepath.Join(dir, "2.0.0.0", "linux_x64", "libwidevinecdm.so"); checked != want {
		t.Errorf("LoadCheck(%s), want %s", checked, want)
	}
	wantInstalled(t, dir, "1.0.0.0")
	ptr := readPointer(t, dir)
	if ptr.Current != "1.0.0.0" || ptr.Rejected["2.0.0.0"] != CodeLoad {
		t.Errorf("pointer = %+v", ptr)
	}

	// It is not fetched again.
	downloads := f.packageRequests()
	o.LoadCheck = nil
	if _, changed, err := runCheck(t, o); err != nil || changed {
		t.Fatalf("second check: changed %v, %v", changed, err)
	}
	if f.packageRequests() != downloads {
		t.Error("downloaded the rejected version again")
	}
}

func TestCheckLoadFailureWithNothingInstalled(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	f.publish(o.Platform, "1.0.0.0", makeCRX(t, o.Platform, "1.0.0.0"))
	o.LoadCheck = func(context.Context, string) error { return errors.New("no") }
	_, err := Ensure(t.Context(), o)
	wantCode(t, err, CodeLoad)
	wantInstalled(t, dir)
}

func TestEnsureUsesTheInstalledCDMWithoutTheNetwork(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	f.publish(o.Platform, "1.0.0.0", makeCRX(t, o.Platform, "1.0.0.0"))
	first, err := Ensure(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	f.publish(o.Platform, "2.0.0.0", makeCRX(t, o.Platform, "2.0.0.0"))
	requests := f.total()
	o.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("request to %s", r.URL)
		return nil, errors.New("offline")
	})}
	again, err := Ensure(t.Context(), o)
	if err != nil || again != first {
		t.Fatalf("Ensure = %+v, %v; want %+v", again, err, first)
	}
	if f.total() != requests {
		t.Error("Ensure made requests")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEnsureReinstallsOverABadPointer(t *testing.T) {
	tests := []struct {
		name    string
		pointer string
	}{
		{"corrupt", "{\"platform\": \"linux_x6"},
		{"empty", ""},
		{"another platform", `{"platform":"win_x64","current":"1.0.0.0"}`},
		{"a climbing version", `{"platform":"linux_x64","current":"../../1.0.0.0"}`},
		{"a version not installed", `{"platform":"linux_x64","current":"7.0.0.0"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(t)
			dir := t.TempDir()
			o := testOptions(t, f, dir)
			installVersion(t, f, o, "1.0.0.0")
			if err := os.WriteFile(filepath.Join(dir, pointerFile), []byte(tt.pointer), 0o600); err != nil {
				t.Fatal(err)
			}
			downloads := f.packageRequests()
			inst, err := Ensure(t.Context(), o)
			if err != nil || inst.Version != "1.0.0.0" {
				t.Fatalf("Ensure = %+v, %v", inst, err)
			}
			if f.packageRequests() != downloads+1 {
				t.Error("did not download again")
			}
			if ptr := readPointer(t, dir); ptr.Platform != "linux_x64" || ptr.Current != "1.0.0.0" {
				t.Errorf("pointer = %+v", ptr)
			}
			wantInstalled(t, dir, "1.0.0.0")
		})
	}
}

func TestStorePointerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := store{root: dir, p: linuxAMD64(t)}
	if ptr, err := s.read(); err != nil || ptr.Platform != "linux_x64" || ptr.Current != "" {
		t.Fatalf("read of nothing = %+v, %v", ptr, err)
	}
	want := pointer{Current: "2.0.0.0", Previous: "1.0.0.0", CheckedAt: t0, Failures: 2, Rejected: map[string]string{"3.0.0.0": CodeLoad}}
	for range 3 {
		if err := s.write(want); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Platform != "linux_x64" || got.Current != want.Current || got.Previous != want.Previous ||
		!got.CheckedAt.Equal(t0) || got.Failures != 2 || got.Rejected["3.0.0.0"] != CodeLoad {
		t.Errorf("read = %+v", got)
	}
	if names := dirNames(t, dir); !slices.Equal(names, []string{pointerFile}) {
		t.Errorf("directory holds %q", names)
	}
	// Another platform's view of the same directory sees nothing.
	if ptr, err := (store{root: dir, p: platform(t, "darwin", "arm64")}).read(); err != nil || ptr.Current != "" || ptr.Platform != "mac_arm64" {
		t.Errorf("foreign read = %+v, %v", ptr, err)
	}
}

func TestEnsureConcurrentCallsInstallOnce(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	o := testOptions(t, f, dir)
	f.publish(o.Platform, "1.0.0.0", makeCRX(t, o.Platform, "1.0.0.0"))
	const n = 6
	var (
		wg      sync.WaitGroup
		results [n]Installed
		errs    [n]error
	)
	for i := range n {
		wg.Go(func() { results[i], errs[i] = Ensure(t.Context(), o) })
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil || results[i] != results[0] || results[i].Version != "1.0.0.0" {
			t.Errorf("Ensure %d = %+v, %v", i, results[i], errs[i])
		}
	}
	if got := f.packageRequests(); got != 1 {
		t.Errorf("%d package downloads, want 1", got)
	}
	wantInstalled(t, dir, "1.0.0.0")
}

func TestEnsureNeedsADirectory(t *testing.T) {
	if _, err := Ensure(t.Context(), Options{}); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("Ensure without Dir = %v", err)
	}
}

func TestEnsureReportsAnUnwritableDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Ensure(t.Context(), Options{Dir: filepath.Join(file, "widevine"), Platform: linuxAMD64(t)})
	wantCode(t, err, CodeDisk)
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{nil, ""},
		{fail(CodeArchive, "x"), CodeArchive},
		{&Error{Code: CodeLoad, Err: errors.New("x")}, CodeLoad},
		{context.DeadlineExceeded, CodeNetwork},
		{os.ErrPermission, CodeDisk},
		{errors.Join(errors.New("a"), fail(CodeSignature, "b")), CodeSignature},
	}
	for i, tt := range tests {
		if got := CodeOf(tt.err); got != tt.code {
			t.Errorf("%d: CodeOf(%v) = %q, want %q", i, tt.err, got, tt.code)
		}
	}
	if !strings.HasPrefix(fail(CodeDisk, "boom %d", 1).Error(), "cdm: boom 1") {
		t.Error("Error() lacks the prefix")
	}
	if _, err := PlatformFor("plan9", "amd64"); CodeOf(err) != CodeUnsupportedPlatform {
		t.Errorf("PlatformFor(plan9) = %v", err)
	}
	if p := platform(t, "windows", "arm64"); p.String() != "windows/arm64" || p.Lib != "widevinecdm.dll" {
		t.Errorf("platform = %+v", p)
	}
}
