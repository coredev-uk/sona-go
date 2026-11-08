package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// release is a fake GitHub: the manifest at releases/latest/download, the
// archive it names, and the releases list.
type release struct {
	srv      *httptest.Server
	pub      string
	archive  []byte
	manifest Manifest
	hits     map[string]int
}

func newRelease(t *testing.T, version string, sign func(digest []byte, priv ed25519.PrivateKey) string) *release {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	r := &release{pub: base64.StdEncoding.EncodeToString(pub), archive: []byte(strings.Repeat("sona-archive;", 1000)), hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits[req.URL.Path]++
		if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" || req.Header.Get("User-Agent") != userAgent {
			t.Errorf("request carries more than it should: %v", req.Header)
		}
		switch req.URL.Path {
		case "/coredev-uk/sona-go/releases/latest/download/update-darwin-arm64.json",
			"/coredev-uk/sona-go/releases/download/v" + version + "/update-darwin-arm64.json":
			_ = json.NewEncoder(w).Encode(r.manifest)
		case "/archive.tar.gz":
			_, _ = w.Write(r.archive)
		case "/repos/coredev-uk/sona-go/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"v0.1.0"},{"tag_name":"v` + version + `"},{"tag_name":"v9.9.9","draft":true}]`))
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	r.srv = srv
	digest := sha256.Sum256(r.archive)
	r.manifest = Manifest{Version: version, Notes: "Notes.", URL: srv.URL + "/archive.tar.gz", Size: int64(len(r.archive)), Signature: sign(digest[:], priv)}
	return r
}

func goodSig(d []byte, priv ed25519.PrivateKey) string { return Sign(priv, d) }

func (r *release) feed() Feed { return Feed{Web: r.srv.URL, API: r.srv.URL} }

func TestFeedFindsTheNewerRelease(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	m, raw, err := r.feed().Manifest(context.Background(), "darwin-arm64", false)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "0.3.0" || m.Notes != "Notes." || !Newer(m.Version, "0.2.0") || len(raw) == 0 {
		t.Fatalf("manifest = %+v", m)
	}
	if n := len(r.hits); n != 1 {
		t.Fatalf("one request expected, got %v", r.hits)
	}
}

func TestFeedPrereleaseReadsTheNewestTagsManifest(t *testing.T) {
	r := newRelease(t, "0.3.0-rc.2", goodSig)
	m, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", true)
	if err != nil || m.Version != "0.3.0-rc.2" {
		t.Fatalf("manifest %+v, err %v", m, err)
	}
}

func TestFeedWithoutAManifestIsNoRelease(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	_, _, err := r.feed().Manifest(context.Background(), "windows-amd64", false)
	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v", err)
	}
}

func TestFeedRefusesIncompleteOrInsecureManifests(t *testing.T) {
	for name, edit := range map[string]func(*Manifest){
		"no signature": func(m *Manifest) { m.Signature = "" },
		"no size":      func(m *Manifest) { m.Size = 0 },
		"plain http":   func(m *Manifest) { m.URL = "http://example.com/a.tar.gz" },
		"bad version":  func(m *Manifest) { m.Version = "latest" },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRelease(t, "0.3.0", goodSig)
			edit(&r.manifest)
			if _, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", false); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func source(r *release, key string) *Source {
	return &Source{Update: r.manifest, Raw: []byte("{}"), Key: key, Dir: ""}
}

func TestSourceServesAVerifiedArchive(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	var last int64
	s := source(r, r.pub)
	s.Progress = func(done, total int64) { last = done; _ = total }
	body, err := s.Open(context.Background(), r.manifest.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	if string(got) != string(r.archive) || last != int64(len(r.archive)) {
		t.Fatalf("served %d bytes, progress %d", len(got), last)
	}
	name := body.(*tempFile).Name()
	_ = body.Close()
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("the download stays after Close")
	}
}

func TestSourceRefusesABadSignature(t *testing.T) {
	// Signed by another key, and signed over other bytes.
	for name, sign := range map[string]func([]byte, ed25519.PrivateKey) string{
		"wrong key": func(d []byte, _ ed25519.PrivateKey) string {
			_, other, _ := ed25519.GenerateKey(nil)
			return Sign(other, d)
		},
		"wrong data": func(_ []byte, p ed25519.PrivateKey) string { return Sign(p, []byte("something else")) },
		"garbage":    func([]byte, ed25519.PrivateKey) string { return "!!" },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRelease(t, "0.3.0", sign)
			tmp := t.TempDir()
			s := source(r, r.pub)
			s.Dir = tmp
			if _, err := s.Open(context.Background(), r.manifest.URL); !errors.Is(err, ErrSignature) {
				t.Fatalf("err = %v, want ErrSignature", err)
			}
			if left, _ := os.ReadDir(tmp); len(left) != 0 {
				t.Fatalf("a refused download stays: %v", left)
			}
		})
	}
}

func TestSourceWithoutAKeyIsOff(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	if _, err := source(r, "").Open(context.Background(), r.manifest.URL); !errors.Is(err, ErrNoKey) {
		t.Fatalf("err = %v, want ErrNoKey", err)
	}
	if _, err := ParseKey("  "); !errors.Is(err, ErrNoKey) {
		t.Fatal("blank key is a key")
	}
}

func TestSourceRefusesOtherURLsAndWrongSizes(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	if _, err := source(r, r.pub).Open(context.Background(), r.srv.URL+"/elsewhere"); err == nil {
		t.Fatal("served a file the manifest doesn't name")
	}
	r.manifest.Size++
	if _, err := source(r, r.pub).Open(context.Background(), r.manifest.URL); err == nil {
		t.Fatal("accepted a short download")
	}
}

// TestSignatureFormatMatchesMyGo pins the format MyGo's updater checks: an
// Ed25519 signature, base64, over the archive's raw SHA-256 digest (mygo's
// update.Sign/Verify). A fixed seed and archive make the signature a
// constant, so a change to either side fails here.
func TestSignatureFormatMatchesMyGo(t *testing.T) {
	priv := ed25519.NewKeyFromSeed([]byte("sona-update-test-seed-0123456789"))
	digest := sha256.Sum256([]byte("archive"))
	sig := Sign(priv, digest[:])
	raw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil || len(raw) != ed25519.SignatureSize {
		t.Fatalf("signature %q: %v", sig, err)
	}
	if !ed25519.Verify(priv.Public().(ed25519.PublicKey), digest[:], raw) {
		t.Fatal("does not verify as plain Ed25519 over the digest")
	}
	const want = "VMf/lu5gFfnDxNn3JAO26VU9IlsNobvAg24aPsmbeYmq7SKgqzEIfItzjNzlHvK+meREOpFpBhfAfGw5xT56BQ=="
	if sig != want {
		t.Errorf("signature = %s", sig)
	}
	// MyGo's manifest field names.
	var m map[string]any
	b, _ := json.Marshal(Manifest{Version: "1", URL: "u", Size: 1, Signature: "s"})
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"version", "url", "size", "signature"} {
		if _, ok := m[k]; !ok {
			t.Errorf("manifest lacks %q", k)
		}
	}
}

func TestTargetsAreTheSelfUpdatingPlatforms(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "darwin-arm64", "windows": "windows-amd64", "linux": ""} {
		arch := map[string]string{"darwin": "arm64", "windows": "amd64", "linux": "amd64"}[goos]
		if got := Target(goos, arch); got != want {
			t.Errorf("Target(%s) = %q, want %q", goos, got, want)
		}
	}
	if Target("darwin", "amd64") != "" || Target("windows", "arm64") != "" {
		t.Error("unsupported architectures update themselves")
	}
}

func TestEmbeddedKeyIsAValidPublicKey(t *testing.T) {
	if _, err := ParseKey(PublicKey()); err != nil {
		t.Fatalf("update.pub: %v", err)
	}
}

func TestRollbackDecision(t *testing.T) {
	p := func(to string, starts int) *Pending { return &Pending{From: "0.2.0", To: to, Starts: starts} }
	for name, tc := range map[string]struct {
		p    *Pending
		want Action
	}{
		"nothing pending": {nil, Normal},
		"first start":     {p("0.3.0", 0), Counted},
		"third start":     {p("0.3.0", 2), Counted},
		"fourth start":    {p("0.3.0", 3), Restore},
		"another version": {p("0.4.0", 3), Normal},
	} {
		if got := Decide(tc.p, "0.3.0"); got != tc.want {
			t.Errorf("%s: Decide = %v, want %v", name, got, tc.want)
		}
	}
}

// layout builds an install of version v: a macOS bundle or a Windows
// directory, and its Guard.
func layout(t *testing.T, goos, v string) Guard {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	if err := os.Mkdir(state, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "Sona.app")
	exe := filepath.Join(root, "Contents", "MacOS", "sona")
	if goos == "windows" {
		root = filepath.Join(dir, "Sona")
		exe = filepath.Join(root, "sona.exe")
	}
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte(v), 0o755); err != nil {
		t.Fatal(err)
	}
	g, ok := NewGuard(state, exe, goos)
	if !ok || g.Root != root {
		t.Fatalf("guard = %+v, ok %v", g, ok)
	}
	return g
}

func exeOf(g Guard) string {
	if g.GOOS == "darwin" {
		return filepath.Join(g.Root, "Contents", "MacOS", "sona")
	}
	return filepath.Join(g.Root, "sona.exe")
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// install swaps the new version in as MyGo's updater does, leaving the
// snapshot of the old one.
func install(t *testing.T, g Guard, v string) {
	t.Helper()
	if err := g.Snapshot("0.2.0"); err != nil {
		t.Fatal(err)
	}
	if err := g.Mark("0.2.0", v); err != nil {
		t.Fatal(err)
	}
	// MyGo replaces files by renaming new ones in, never writing through.
	exe := exeOf(g)
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte(v), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestAnUpdateThatNeverPaintsIsRolledBack(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := layout(t, goos, "0.2.0")
			install(t, g, "0.3.0")
			prev := filepath.Join(g.Previous, strings.TrimPrefix(exeOf(g), g.Root))
			if read(t, prev) != "0.2.0" {
				t.Fatal("the snapshot isn't the old version")
			}
			for i := range MaxStarts {
				act, err := g.Begin("0.3.0")
				if err != nil || act != Counted {
					t.Fatalf("start %d: %v %v", i+1, act, err)
				}
			}
			act, _ := g.Begin("0.3.0")
			if act != Restore {
				t.Fatalf("start %d: %v, want Restore", MaxStarts+1, act)
			}
			if err := g.Rollback(); err != nil {
				t.Fatal(err)
			}
			if read(t, exeOf(g)) != "0.2.0" {
				t.Fatal("the old version isn't back")
			}
			if g.Pending() != nil {
				t.Fatal("the marker stays")
			}
			if r := g.LastRollback(); r == nil || r.From != "0.3.0" || r.To != "0.2.0" || r.Error != "" {
				t.Fatalf("report = %+v", r)
			}
			// The restored version starts normally.
			if act, _ := g.Begin("0.2.0"); act != Normal {
				t.Fatalf("after rollback: %v", act)
			}
		})
	}
}

func TestAnUpdateThatPaintsIsKept(t *testing.T) {
	g := layout(t, "darwin", "0.2.0")
	install(t, g, "0.3.0")
	if act, _ := g.Begin("0.3.0"); act != Counted {
		t.Fatal("not counted")
	}
	if err := g.Unmark(); err != nil { // the first paint
		t.Fatal(err)
	}
	if act, _ := g.Begin("0.3.0"); act != Normal {
		t.Fatal("still on trial after the first paint")
	}
	if read(t, exeOf(g)) != "0.3.0" {
		t.Fatal("the update was undone")
	}
}

func TestStaleMarkerIsDropped(t *testing.T) {
	g := layout(t, "windows", "0.2.0")
	if err := g.Mark("0.2.0", "0.3.0"); err != nil {
		t.Fatal(err)
	}
	if act, _ := g.Begin("0.2.0"); act != Normal || g.Pending() != nil {
		t.Fatal("a marker for a version that isn't running stays")
	}
}

func TestRollbackWithoutAPreviousVersionClearsTheMarker(t *testing.T) {
	g := layout(t, "darwin", "0.3.0")
	if err := g.Mark("0.2.0", "0.3.0"); err != nil {
		t.Fatal(err)
	}
	if err := g.Rollback(); err == nil {
		t.Fatal("restored nothing without error")
	}
	if g.Pending() != nil {
		t.Fatal("a failed restore would loop on every start")
	}
	if r := g.LastRollback(); r == nil || r.Error == "" {
		t.Fatalf("report = %+v", r)
	}
}

func TestGuardOnlyForBundlesAndWindows(t *testing.T) {
	if _, ok := NewGuard("s", "/usr/bin/sona", "darwin"); ok {
		t.Error("a bare executable on macOS")
	}
	if _, ok := NewGuard("s", "/usr/bin/sona", "linux"); ok {
		t.Error("Linux")
	}
}
