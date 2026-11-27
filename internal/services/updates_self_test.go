package services

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/update"
)

// fakeUpdater is MyGo's updater: it asks the source for the archive, as
// mygo does, and "swaps" by replacing the executable. failWith, when set, is
// the error of an install that gets past the download and fails unpacking;
// beforeSwap runs after the download, when MyGo would unpack and swap.
type fakeUpdater struct {
	enabled    bool
	exe        string
	mu         sync.Mutex
	installed  bool
	failWith   error
	beforeSwap func()
	relaunch   chan struct{}
}

func (f *fakeUpdater) Enabled() bool { return f.enabled }

func (f *fakeUpdater) Install(ctx context.Context, src *update.Source) error {
	body, err := src.Open(ctx, src.Update.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	if _, err := io.Copy(io.Discard, body); err != nil {
		return err
	}
	if f.beforeSwap != nil {
		f.beforeSwap()
	}
	if f.failWith != nil {
		return f.failWith
	}
	if err := os.Remove(f.exe); err != nil {
		return err
	}
	f.mu.Lock()
	f.installed = true
	f.mu.Unlock()
	return os.WriteFile(f.exe, []byte(src.Update.Version), 0o755)
}

func (f *fakeUpdater) Relaunch() { close(f.relaunch) }

// selfRig is an UpdatesService on a fake release server that updates
// itself, with an installed app in a temp dir.
type selfRig struct {
	s       *UpdatesService
	e       *Env
	rec     *recorder
	up      *fakeUpdater
	g       update.Guard
	hits    map[string]int
	hitsMu  sync.Mutex
	archive []byte
}

type selfOpts struct {
	version, latest string
	key             bool
	badSig          bool
	claims          string // the version the manifest claims, when it is not the one signed
}

func newSelfRig(t *testing.T, o selfOpts) *selfRig {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(nil)
	r := &selfRig{hits: map[string]int{}, archive: []byte("the new Sona.app")}
	digest := sha256.Sum256(r.archive)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hitsMu.Lock()
		r.hits[req.URL.Path]++
		r.hitsMu.Unlock()
		switch req.URL.Path {
		case "/" + update.Repo + "/releases/latest/download/update-darwin-arm64.json":
			sig := update.Sign(priv, digest[:])
			if o.badSig {
				_, other, _ := ed25519.GenerateKey(nil)
				sig = update.Sign(other, digest[:])
			}
			m := update.Manifest{
				Version: o.latest, Notes: "## [" + o.latest + "](https://example.test/compare) (2026-01-02)\n\nWhat's new.", Date: "2026-01-02T03:04:05Z",
				URL: srv.URL + "/a.tar.gz", Size: int64(len(r.archive)), Signature: sig,
				SHA256: hex.EncodeToString(digest[:]), StatementSignature: update.SignStatement(priv, "darwin-arm64", o.latest, digest[:]),
			}
			if o.claims != "" {
				m.Version = o.claims
			}
			_ = json.NewEncoder(w).Encode(m)
		case "/a.tar.gz":
			_, _ = w.Write(r.archive)
		default:
			// The GitHub API, for installs that don't update themselves.
			_, _ = w.Write([]byte(`{"tag_name":"v` + o.latest + `","html_url":"https://github.com/coredev-uk/sona-go/releases/tag/v` + o.latest + `"}`))
		}
	}))
	t.Cleanup(srv.Close)

	// The installed app and its state folder.
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	exe := filepath.Join(dir, "Sona.app", "Contents", "MacOS", "sona")
	state := filepath.Join(dir, "state")
	for _, d := range []string{filepath.Dir(exe), state} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exe, []byte(o.version), 0o755); err != nil {
		t.Fatal(err)
	}
	r.g, _ = update.NewGuard(state, exe, "darwin")

	st, err := config.OpenStore(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.rec = &recorder{}
	r.up = &fakeUpdater{enabled: true, exe: exe, relaunch: make(chan struct{})}
	r.e = &Env{Deps: Deps{
		Settings: st, Gate: NewGate(time.Hour, nil), Emit: r.rec, Logger: discard(), Now: time.Now, Version: o.version,
		Paths: config.Paths{State: state}, SelfUpdater: r.up,
	}}
	r.s = newUpdatesService(r.e)
	r.s.target, r.s.guard = "darwin-arm64", &r.g
	r.s.key = ""
	if o.key {
		r.s.key = base64.StdEncoding.EncodeToString(pub)
	}
	r.s.feed = update.Feed{Web: srv.URL, Key: r.s.key}
	r.s.checker = update.Checker{API: srv.URL}
	r.s.status.SelfUpdate, r.s.status.Disabled = r.s.selfState()
	t.Cleanup(r.e.close)
	return r
}

func (r *selfRig) hit(path string) int {
	r.hitsMu.Lock()
	defer r.hitsMu.Unlock()
	return r.hits[path]
}

const manifestPath = "/" + update.Repo + "/releases/latest/download/update-darwin-arm64.json"

func TestSelfUpdateFindsANewerVersionWithOneRequest(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	st, err := r.s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Available || !st.SelfUpdate || st.Latest != "0.3.0" || st.Notes != "What's new." || st.Disabled != "" || st.Published == 0 {
		t.Fatalf("status = %+v", st)
	}
	if len(r.hits) != 1 || r.hit(manifestPath) != 1 {
		t.Fatalf("requests = %v", r.hits)
	}
}

func TestSelfUpdateInstallsAndRestarts(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	if _, err := r.s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := r.s.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || st.Installing || st.InstallError != "" {
		t.Fatalf("status = %+v", st)
	}
	select {
	case <-r.up.relaunch:
	case <-time.After(5 * time.Second):
		t.Fatal("never relaunched")
	}
	// The old version is kept, the new one is on trial.
	if b, _ := os.ReadFile(filepath.Join(r.g.Previous, "Contents", "MacOS", "sona")); string(b) != "0.2.0" {
		t.Fatalf("previous = %q", b)
	}
	if p := r.g.Pending(); p == nil || p.From != "0.2.0" || p.To != "0.3.0" || p.Starts != 0 {
		t.Fatalf("pending = %+v", p)
	}
	r.rec.mu.Lock()
	defer r.rec.mu.Unlock()
	if n := len(r.rec.events); n < 2 || r.rec.events[0] != EventUpdateInstall || !r.rec.data[0].(UpdateStatus).Installing {
		t.Fatalf("events = %v", r.rec.events)
	}
	if last := r.rec.data[len(r.rec.data)-1].(UpdateStatus); !last.Installed {
		t.Fatalf("last event = %+v", last)
	}
}

func TestSelfUpdateRefusesABadSignature(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true, badSig: true})
	if _, err := r.s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := r.s.Install(context.Background())
	if !errors.Is(err, update.ErrSignature) || CodeOf(err) != CodeUpdateSignature {
		t.Fatalf("err = %v", err)
	}
	if st.InstallError != CodeUpdateSignature || st.Installed || st.Installing {
		t.Fatalf("status = %+v", st)
	}
	select {
	case <-r.up.relaunch:
		t.Fatal("relaunched after a refused update")
	default:
	}
	if r.g.Pending() != nil {
		t.Fatal("a refused update stays on trial")
	}
	if b, _ := os.ReadFile(r.up.exe); string(b) != "0.2.0" {
		t.Fatalf("the app changed: %q", b)
	}
}

// The statement binds the manifest's version to the signed archive: the same
// archive offered under a higher version is refused at the check, before
// anything is offered.
func TestSelfUpdateRefusesAVersionTheKeyDidNotSign(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", claims: "99.0.0", key: true})
	st, err := r.s.Check(context.Background())
	if !errors.Is(err, update.ErrSignature) || CodeOf(err) != CodeUpdateSignature {
		t.Fatalf("err = %v", err)
	}
	if st.Available || st.Latest != "" || st.Error != CodeUpdateSignature {
		t.Fatalf("status = %+v", st)
	}
	if _, err := r.s.Install(context.Background()); err == nil {
		t.Fatal("installed after a refused check")
	}
	if b, _ := os.ReadFile(r.up.exe); string(b) != "0.2.0" {
		t.Fatalf("the app changed: %q", b)
	}
}

// The tester's report: an update whose archive the updater refuses ("./")
// failed, and left a copy of the running version as the previous one. A
// failed install leaves what was kept before it, says why in Diagnostics, and
// a later install that works keeps the version it replaced.
func TestSelfUpdateFailedInstallLeavesTheKeptVersionAndSaysWhy(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	r.keepPrevious(t, "0.1.0", "0.2.0")
	if _, err := r.s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.up.failWith = errors.New(`mygo: installing the update: invalid path in update archive: "./"`)
	st, err := r.s.Install(context.Background())
	if CodeOf(err) != CodeUpdateFailed || !errors.Is(err, update.ErrInstall) {
		t.Fatalf("err = %v", err)
	}
	if st.InstallError != CodeUpdateFailed || st.Installed || st.Installing || !st.Available {
		t.Fatalf("status = %+v", st)
	}
	select {
	case <-r.up.relaunch:
		t.Fatal("relaunched after a failed install")
	default:
	}
	if r.g.Pending() != nil {
		t.Fatal("a failed install stays on trial")
	}
	if p := r.s.Previous(); !p.Available || p.Version != "0.1.0" {
		t.Fatalf("previous = %+v, want the 0.1.0 kept before", p)
	}
	if b, _ := os.ReadFile(filepath.Join(r.g.Previous, "Contents", "MacOS", "sona")); string(b) != "0.1.0" {
		t.Fatalf("the kept version's files are %q", b)
	}
	if _, err := os.Stat(r.g.Previous + ".staging"); err == nil {
		t.Error("the staged copy stays")
	}
	ie, _ := r.s.bundle().(map[string]any)["installError"].(map[string]any)
	if ie["code"] != CodeUpdateFailed || ie["latest"] != "0.3.0" || !strings.Contains(ie["detail"].(string), `invalid path in update archive: "./"`) {
		t.Fatalf("Diagnostics installError = %v", ie)
	}

	// Trying again, with the fault gone, installs and keeps 0.2.0.
	r.up.failWith = nil
	st, err = r.s.Install(context.Background())
	if err != nil || !st.Installed || st.InstallError != "" {
		t.Fatalf("second install = %+v, %v", st, err)
	}
	if v, ok := r.g.HasPrevious(); !ok || v != "0.2.0" {
		t.Fatalf("previous after the update = %q, %v", v, ok)
	}
	if _, ok := r.s.bundle().(map[string]any)["installError"]; ok {
		t.Error("Diagnostics still reports the failure of an install that worked")
	}
}

func TestSelfUpdateWithoutAKeyIsOffAndSaysSo(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0"})
	st, err := r.s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.SelfUpdate || st.Disabled != UpdatesUnsigned || !st.Available {
		t.Fatalf("status = %+v", st)
	}
	if r.hit(manifestPath) != 0 || r.hit("/a.tar.gz") != 0 {
		t.Fatalf("an unsigned build read the update feed: %v", r.hits)
	}
	if _, err := r.s.Install(context.Background()); CodeOf(err) != CodeUnsupported {
		t.Fatalf("Install err = %v", err)
	}
}

func TestSelfUpdateNotWritableIsReported(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	r.up.enabled = false
	r.s.status.SelfUpdate, r.s.status.Disabled = r.s.selfState()
	if st := r.s.Status(); st.SelfUpdate || st.Disabled != UpdatesNotWritable {
		t.Fatalf("status = %+v", st)
	}
}

func TestSelfUpdateDevBuildNeverChecks(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "dev", latest: "0.3.0", key: true})
	r.e.Gate.Open()
	st, err := r.s.Check(context.Background())
	if err != nil || st.Checkable || st.SelfUpdate || st.Disabled != "" || len(r.hits) != 0 {
		t.Fatalf("Check = %+v, %v (requests %v)", st, err, r.hits)
	}
}

func TestSelfUpdateCheckWaitsForFirstPaintAndTheSetting(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	if len(r.hits) != 0 {
		t.Fatal("checked before the first paint")
	}
	if _, err := r.e.Settings.Update(func(s *config.Settings) { s.Updates.Check = false }); err != nil {
		t.Fatal(err)
	}
	r.e.Gate.Open()
	if len(r.hits) != 0 {
		t.Fatalf("checked with updates.check off: %v", r.hits)
	}
}

func TestFirstPaintEndsTheTrialOfAnUpdate(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.3.0", latest: "0.3.0", key: true})
	if err := r.g.Mark("0.2.0", "0.3.0"); err != nil {
		t.Fatal(err)
	}
	r.e.Gate.Open()
	if r.g.Pending() != nil {
		t.Fatal("the marker stays after the first paint")
	}
}
