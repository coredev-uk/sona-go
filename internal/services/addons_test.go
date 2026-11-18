//go:build unix

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/addons/sandbox"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// addonEnv is a supervisor over a temp add-ons folder and the command
// environment's host.
type addonEnv struct {
	*commandEnv
	svc  *AddonsService
	root string
	logs string
}

func newAddonEnv(t *testing.T) *addonEnv {
	t.Helper()
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	dir := t.TempDir()
	ce.e.Paths = config.Paths{
		Config: filepath.Join(dir, "config"), Data: filepath.Join(dir, "data"), Cache: filepath.Join(dir, "cache"),
		Art: filepath.Join(dir, "cache", "art"), State: filepath.Join(dir, "state"),
	}
	if err := ce.e.Paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	// These tests are about the supervisor, on any system: their fake add-ons
	// run without the sandbox, which internal/addons/sandbox's tests cover.
	t.Setenv(sandbox.EnvUnsandboxed, "1")
	s := newAddonsService(ce.e)
	s.attach(ce.srv.addon)
	ce.srv.addons = s
	// The test binary is not Sona: its bundled add-ons stay refused unless a
	// test points exe at a script of its own.
	s.opts.exe = ""
	s.opts.backoff = []time.Duration{5 * time.Millisecond}
	s.opts.grace = 100 * time.Millisecond
	s.opts.helloTimeout = 5 * time.Second
	t.Cleanup(s.close)
	return &addonEnv{commandEnv: ce, svc: s, root: ce.e.Paths.AddonsDir(), logs: ce.e.Paths.AddonLogDir()}
}

func manifestJSON(id string, perms ...string) string {
	b, _ := json.Marshal(perms)
	return fmt.Sprintf(`{"id":%q,"name":"Add-on %s","version":"1.0.0","command":"run","permissions":%s}`, id, id, b)
}

// manifestWith is manifestJSON with contributions.
func manifestWith(id, contributions string, perms ...string) string {
	b, _ := json.Marshal(perms)
	return fmt.Sprintf(`{"id":%q,"name":"Add-on %s","version":"1.0.0","command":"run","permissions":%s,"contributions":%s}`, id, id, b, contributions)
}

const sendAction = `{"actions":[{"id":"send","label":"Send","kinds":["album"]}],"commands":[{"id":"ping","label":"Ping"}]}`

// install writes an add-on folder with a shell script as its program.
func (a *addonEnv) install(t *testing.T, id, manifest, script string) string {
	t.Helper()
	dir := filepath.Join(a.root, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "addon.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// hello is the shell of the add-on's first frame.
func hello(id string, perms ...string) string {
	b, _ := json.Marshal(perms)
	return fmt.Sprintf(`printf '%%s\n' '{"id":1,"method":"hello","params":{"id":%q,"protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":%s}}'`+"\n", id, b)
}

func (a *addonEnv) info(t *testing.T, id string) AddonInfo {
	t.Helper()
	for _, i := range a.svc.List() {
		if i.ID == id {
			return i
		}
	}
	t.Fatalf("no add-on %q in %+v", id, a.svc.List())
	return AddonInfo{}
}

func (a *addonEnv) waitState(t *testing.T, id string, want AddonState) AddonInfo {
	t.Helper()
	var got AddonInfo
	waitUntil(t, fmt.Sprintf("%s to be %s", id, want), func() bool {
		got = a.info(t, id)
		return got.State == want
	})
	return got
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func fileHas(path, text string) func() bool {
	return func() bool {
		b, _ := os.ReadFile(path)
		return strings.Contains(string(b), text)
	}
}

func TestAddonsListsAndRefuses(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "good", manifestJSON("good", "playback.read"), "exit 0\n")
	a.install(t, "broken", `{"id":"broken","name":"x","version":"1.0.0","command":"run","permissions":[],"extra":1}`, "exit 0\n")
	a.install(t, "sona-tools", manifestJSON("sona-tools"), "exit 0\n")
	a.install(t, "named-wrong", manifestJSON("other"), "exit 0\n")
	got := map[string]AddonInfo{}
	for _, i := range a.svc.List() {
		got[i.ID] = i
	}
	if i := got["good"]; i.State != AddonOff || i.Enabled || i.Name != "Add-on good" || len(i.Permissions) != 1 || len(i.Granted) != 0 || i.Problem != "" {
		t.Errorf("good = %+v", i)
	}
	for id, problem := range map[string]string{"broken": addons.ProblemManifest, "sona-tools": addons.ProblemReservedID, "other": addons.ProblemIDMismatch} {
		i, ok := got[id]
		if !ok && id == "other" {
			i = got["named-wrong"]
		}
		if i.State != AddonInvalid || i.Problem != problem {
			t.Errorf("%s = %+v, want invalid %s", id, i, problem)
		}
	}
	// A refused add-on can't be enabled; one that doesn't exist is not found.
	if err := a.svc.Enable("sona-tools", nil); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("enable sona-tools: %v", err)
	}
	if err := a.svc.Enable("nothing", nil); CodeOf(err) != CodeNotFound {
		t.Errorf("enable unknown: %v (%s)", err, CodeOf(err))
	}
}

func TestAddonsEnableNeedsTheApprovedPermissions(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "demo", manifestJSON("demo", "playback.read", "queue.read"), "exit 0\n")
	for name, approve := range map[string][]string{
		"fewer": {"playback.read"}, "none": nil, "more": {"playback.read", "queue.read", "playback.control"}, "other": {"playback.read", "like"},
	} {
		if err := a.svc.Enable("demo", approve); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: enable = %v", name, err)
		}
	}
	if i := a.info(t, "demo"); i.Enabled || len(i.Granted) != 0 {
		t.Errorf("a refused enable left %+v", i)
	}
	if err := a.svc.Enable("demo", []string{"queue.read", "playback.read"}); err != nil {
		t.Fatal(err)
	}
	st := a.settings.Get().Addons["demo"]
	if !st.Enabled || len(st.Granted) != 2 {
		t.Errorf("settings = %+v", st)
	}
	// Before the network opens nothing runs.
	if i := a.info(t, "demo"); i.Enabled != true || a.svc.runners["demo"] != nil {
		t.Errorf("enabled before first paint: %+v", i)
	}
	if err := a.svc.Disable("demo"); err != nil {
		t.Fatal(err)
	}
	if st := a.settings.Get().Addons["demo"]; st.Enabled || len(st.Granted) != 2 {
		t.Errorf("after disable: %+v", st)
	}
}

// A running add-on says hello, reads the status through the host, and the
// revocation of its permission refuses the very next call.
func TestAddonsRunServeAndRevoke(t *testing.T) {
	a := newAddonEnv(t)
	dir := a.install(t, "demo", manifestJSON("demo", "playback.read"), hello("demo", "playback.read")+`
exec 3<&0
cat <&3 > in.log &
n=1
while :; do
  n=$((n+1))
  printf '{"id":%d,"method":"call","params":{"command":"status"}}\n' "$n"
  sleep 0.05
done
`)
	if err := a.svc.Enable("demo", []string{"playback.read"}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "demo", AddonRunning)
	waitUntil(t, "a status answer", fileHas(filepath.Join(dir, "in.log"), `"running":true`))
	if err := a.svc.Revoke("demo", "playback.read"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "permission_denied", fileHas(filepath.Join(dir, "in.log"), `"code":"permission_denied"`))
	// The add-on keeps running with fewer permissions this session; at its
	// next start it needs approval.
	if i := a.info(t, "demo"); (i.State != AddonRunning && i.State != AddonNeedsApproval) || len(i.Granted) != 0 {
		t.Errorf("after revoke: %+v", i)
	}
	if err := a.svc.Disable("demo"); err != nil {
		t.Fatal(err)
	}
	a.svc.close()
	// The log is the add-on's own, 0600, with the supervisor's notes marked.
	lines, err := a.svc.Log("demo")
	if err != nil || !strings.Contains(strings.Join(lines, "\n"), "sona: started") {
		t.Errorf("log = %q, %v", lines, err)
	}
	if fi, err := os.Stat(addons.LogPath(a.logs, "demo")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("log file: %v %v", fi, err)
	}
}

// An add-on whose manifest asks for more than was approved does not run.
func TestAddonsNewPermissionNeedsApproval(t *testing.T) {
	a := newAddonEnv(t)
	dir := a.install(t, "demo", manifestJSON("demo", "playback.read"), "echo ran > ran.out\nexit 0\n")
	if err := a.svc.Enable("demo", []string{"playback.read"}); err != nil {
		t.Fatal(err)
	}
	// The author's update adds a permission.
	if err := os.WriteFile(filepath.Join(dir, "addon.json"), []byte(manifestJSON("demo", "playback.read", "playback.control")), 0o600); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	if i := a.info(t, "demo"); i.State != AddonNeedsApproval || !i.Enabled {
		t.Errorf("state = %+v", i)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "ran.out")); err == nil {
		t.Error("the add-on ran without approval of its new permission")
	}
	// Approving what it asks for now starts it.
	if err := a.svc.Enable("demo", []string{"playback.read", "playback.control"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the add-on to run", func() bool { _, err := os.Stat(filepath.Join(dir, "ran.out")); return err == nil })
}

func TestAddonsRestartWithBackoffThenSwitchOff(t *testing.T) {
	a := newAddonEnv(t)
	a.svc.opts.maxCrashes = 3
	dir := a.install(t, "crashy", manifestJSON("crashy"), "echo run >> runs.out\necho oops >&2\nexit 3\n")
	if err := a.svc.Enable("crashy", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	i := a.waitState(t, "crashy", AddonCrashed)
	if i.Restarts != 3 || i.Enabled {
		t.Errorf("info = %+v", i)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "runs.out"))
	if n := strings.Count(string(b), "run"); n != 3 {
		t.Errorf("started %d times, want 3", n)
	}
	if st := a.settings.Get().Addons["crashy"]; st.Enabled {
		t.Errorf("settings still enabled: %+v", st)
	}
	var notice *AddonNotice
	a.rec.mu.Lock()
	for k, name := range a.rec.events {
		if name == EventAddonsNotice {
			n := a.rec.data[k].(AddonNotice)
			notice = &n
		}
	}
	a.rec.mu.Unlock()
	if notice == nil || notice.ID != "crashy" || notice.Kind != "crashed" || notice.Name != "Add-on crashy" {
		t.Errorf("notice = %+v", notice)
	}
	lines, _ := a.svc.Log("crashy")
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "oops") || !strings.Contains(text, "switched off") {
		t.Errorf("log:\n%s", text)
	}
}

func TestAddonsBackoffGrowsToACeiling(t *testing.T) {
	o := defaultAddonOptions()
	if o.maxCrashes < 2 || o.backoff[len(o.backoff)-1] > time.Minute || o.backoff[0] < 500*time.Millisecond {
		t.Errorf("options = %+v", o)
	}
	for i := 1; i < len(o.backoff); i++ {
		if o.backoff[i] <= o.backoff[i-1] {
			t.Errorf("backoff %v does not grow", o.backoff)
		}
	}
}

func TestAddonsKillOneThatNeverSaysHello(t *testing.T) {
	a := newAddonEnv(t)
	a.svc.opts.helloTimeout = 150 * time.Millisecond
	a.svc.opts.maxCrashes = 2
	a.install(t, "mute", manifestJSON("mute"), "sleep 300\n")
	if err := a.svc.Enable("mute", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "mute", AddonCrashed)
	lines, _ := a.svc.Log("mute")
	if !strings.Contains(strings.Join(lines, "\n"), "no hello") {
		t.Errorf("log: %q", lines)
	}
}

// Invocations run with deadlines; a hung add-on blocks only its own caller,
// and the rest of the service answers meanwhile.
func TestAddonsInvokeAndTimeout(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "answers", manifestWith("answers", sendAction), hello("answers")+`
while read -r line; do
  case $line in
    *'"method":"invoke"'*)
      id=${line#'{"id":'}; id=${id%%,*}
      printf '{"id":%s,"result":{"toast":"Sent","url":"https://example.com/x"}}\n' "$id" ;;
  esac
done
`)
	a.install(t, "hangs", manifestWith("hangs", sendAction), hello("hangs")+"sleep 300\n")
	for _, id := range []string{"answers", "hangs"} {
		if err := a.svc.Enable(id, nil); err != nil {
			t.Fatal(err)
		}
	}
	a.svc.start()
	a.waitState(t, "answers", AddonRunning)
	a.waitState(t, "hangs", AddonRunning)

	res, err := a.svc.Invoke(t.Context(), "answers", "addon.answers.send", &AddonTarget{Kind: "album", ID: "1", Name: "Abbey Road"})
	if err != nil || res.Toast != "Sent" || res.URL != "https://example.com/x" {
		t.Errorf("invoke = %+v, %v", res, err)
	}
	if res, err := a.svc.Invoke(t.Context(), "answers", "addon.answers.ping", nil); err != nil || res.Toast != "Sent" {
		t.Errorf("command = %+v, %v", res, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := a.svc.Invoke(ctx, "hangs", "addon.hangs.ping", nil)
		done <- err
	}()
	// The UI's other calls are not held up by the hung add-on.
	start := time.Now()
	a.svc.List()
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("List took %v while an add-on hung", d)
	}
	select {
	case err := <-done:
		if CodeOf(err) != CodeTimeout {
			t.Errorf("hung invoke = %v (%s), want a timeout", err, CodeOf(err))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("invoke never returned")
	}

	// Actions of another add-on, and of one that isn't running, are refused.
	album := &AddonTarget{Kind: "album", ID: "1"}
	for name, tc := range map[string]struct {
		action string
		target *AddonTarget
	}{
		"another add-on's action":   {"addon.hangs.send", album},
		"an action nobody declared": {"addon.answers.evil", album},
		"an action without an item": {"addon.answers.send", nil},
		"a command with an item":    {"addon.answers.ping", album},
		"a station":                 {"addon.answers.send", &AddonTarget{Kind: "station", ID: "1"}},
		"an item with no ID":        {"addon.answers.send", &AddonTarget{Kind: "album"}},
	} {
		if _, err := a.svc.Invoke(t.Context(), "answers", tc.action, tc.target); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := a.svc.Invoke(t.Context(), "answers", "addon.hangs.send", nil); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("another add-on's action: %v", err)
	}
	if _, err := a.svc.Invoke(t.Context(), "answers", "sona.player.play", nil); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("a shortcut ID: %v", err)
	}
	if _, err := a.svc.Invoke(t.Context(), "ghost", "addon.ghost.x", nil); CodeOf(err) != CodeAddonUnavailable {
		t.Errorf("not running: %v", err)
	}
}

// Quit tells add-ons to stop, then ends them and everything they started:
// nothing is left running or unreaped.
func TestAddonsQuitStopsAddonsAndTheirChildren(t *testing.T) {
	a := newAddonEnv(t)
	dir := a.install(t, "stubborn", manifestJSON("stubborn"), hello("stubborn")+`
trap '' TERM
sleep 300 &
echo $! > child.pid
exec 3<&0
cat <&3 > in.log &
wait
`)
	if err := a.svc.Enable("stubborn", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "stubborn", AddonRunning)
	var child int
	waitUntil(t, "the child's pid", func() bool {
		b, _ := os.ReadFile(filepath.Join(dir, "child.pid"))
		n, err := strconv.Atoi(strings.TrimSpace(string(b)))
		child = n
		return err == nil
	})
	start := time.Now()
	a.svc.close()
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("quit took %v", d)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "in.log")); !strings.Contains(string(b), `"event":"shutdown"`) {
		t.Errorf("the add-on was not told to shut down: %q", b)
	}
	waitUntil(t, "the child to die", func() bool { return processGone(child) })
	// After quit nothing starts again.
	a.svc.start()
	if _, err := a.svc.Invoke(t.Context(), "stubborn", "addon.stubborn.x", nil); CodeOf(err) != CodeAddonUnavailable {
		t.Errorf("after quit: %v", err)
	}
}

func TestAddonsEnvironmentHasNoSecrets(t *testing.T) {
	t.Setenv("MUSIC_USER_TOKEN", "tok-secret")
	t.Setenv("SONA_LASTFM_KEY", "lfm-secret")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nowhere")
	a := newAddonEnv(t)
	dir := a.install(t, "env", manifestJSON("env"), "env > env.out\n"+hello("env")+"sleep 300\n")
	if err := a.svc.Enable("env", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "env", AddonRunning)
	b, _ := os.ReadFile(filepath.Join(dir, "env.out"))
	for _, banned := range []string{"MUSIC_USER_TOKEN", "tok-secret", "SONA_LASTFM_KEY", "lfm-secret", "DBUS_"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("the add-on's environment has %q", banned)
		}
	}
}

// The bug-report section lists add-ons and never their logs.
func TestAddonsDiagnosticsExcludeLogs(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "talky", manifestJSON("talky"), "echo 'password hunter2 from the add-on' >&2\nexit 0\n")
	if err := a.svc.Enable("talky", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.maxCrashes = 1
	a.svc.start()
	a.waitState(t, "talky", AddonCrashed)
	waitUntil(t, "the log", func() bool { l, _ := a.svc.Log("talky"); return strings.Contains(strings.Join(l, ""), "hunter2") })
	b, err := json.Marshal(a.svc.bundle())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "hunter2") || !strings.Contains(string(b), `"id":"talky"`) || !strings.Contains(string(b), `"version":"1.0.0"`) {
		t.Errorf("bundle = %s", b)
	}
}

func TestAddonsCapabilityIsReported(t *testing.T) {
	if !addons.Supported() {
		t.Skip()
	}
	if !(&PlatformService{caps: Capabilities{Addons: addons.Supported()}}).Capabilities().Addons {
		t.Error("capabilities don't report add-ons")
	}
}

// Nothing runs before the first paint: the add-ons start when the network
// opens, the gate's moment for work that mustn't slow a cold start.
func TestAddonsStartWhenTheNetworkOpens(t *testing.T) {
	a := newAddonEnv(t)
	dir := a.install(t, "late", manifestJSON("late"), "echo ran > ran.out\n"+hello("late")+"sleep 300\n")
	if err := a.svc.Enable("late", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "ran.out")); err == nil {
		t.Fatal("the add-on ran before the first paint")
	}
	a.e.Gate.Open()
	a.waitState(t, "late", AddonRunning)
}

// processGone reports that pid is not running: it doesn't exist, or on Linux
// it is a zombie (a container without an init process doesn't reap orphans,
// and a dead process that nobody waited for still answers signal 0).
func processGone(pid int) bool {
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		if i := strings.LastIndexByte(string(b), ')'); i > 0 && i+2 < len(b) && b[i+2] == 'Z' {
			return true
		}
	}
	return syscall.Kill(pid, 0) != nil
}

func TestAddonsOpenFolderCreatesItPrivateAndOpensIt(t *testing.T) {
	a := newAddonEnv(t)
	if err := os.RemoveAll(a.root); err != nil {
		t.Fatal(err)
	}
	var opened []string
	a.svc.e.OpenURL = func(u string) error { opened = append(opened, u); return nil }
	if err := a.svc.OpenFolder(); err != nil {
		t.Fatal(err)
	}
	if want := "file://" + a.root; len(opened) != 1 || opened[0] != want {
		t.Errorf("opened %q, want %q", opened, want)
	}
	fi, err := os.Stat(a.root)
	if err != nil || !fi.IsDir() {
		t.Fatalf("folder: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("folder mode %v, want 0700", fi.Mode().Perm())
	}
	a.svc.e.OpenURL = nil
	if err := a.svc.OpenFolder(); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("without a way to open: %v", err)
	}
}

// Where the sandbox can't be set up the add-on is refused, with the reason,
// and never run: no restart, no run without it.
func TestAddonsRefusedWhenTheSandboxCannotStart(t *testing.T) {
	a := newAddonEnv(t)
	dir := a.install(t, "demo", manifestJSON("demo", "playback.read"), "touch ran\n"+hello("demo", "playback.read")+"sleep 30\n")
	var starts int
	a.svc.opts.start = func(addons.Spec) (*addons.Proc, error) {
		starts++
		return nil, &sandbox.Error{Reason: sandbox.ReasonUserNamespaceRestricted, Detail: "no capabilities"}
	}
	if err := a.svc.Enable("demo", []string{"playback.read"}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	i := a.waitState(t, "demo", AddonNoSandbox)
	if i.Sandbox != addons.SandboxUserNamespaceRestricted || !i.Enabled || i.Problem != "" {
		t.Errorf("info = %+v", i)
	}
	time.Sleep(100 * time.Millisecond) // a restart would show by now
	if starts != 1 || a.info(t, "demo").Restarts != 0 {
		t.Errorf("started %d times, info %+v: a sandbox failure is not a crash to restart", starts, a.info(t, "demo"))
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Error("the add-on ran without its sandbox")
	}
	lines, _ := a.svc.Log("demo")
	if !strings.Contains(strings.Join(lines, "\n"), "not started: the sandbox can't be set up") {
		t.Errorf("log = %q", lines)
	}
	// Switching it off clears the reason; turning it on tries again.
	if err := a.svc.Disable("demo"); err != nil {
		t.Fatal(err)
	}
	if i := a.info(t, "demo"); i.State != AddonOff || i.Sandbox != "" {
		t.Errorf("after disable: %+v", i)
	}
	if err := a.svc.Enable("demo", []string{"playback.read"}); err != nil {
		t.Fatal(err)
	}
	a.waitState(t, "demo", AddonNoSandbox)
	if starts != 2 {
		t.Errorf("turning it on again started %d times, want 2", starts)
	}
}

// An add-on gets its private data folder, 0700, and the network only when its
// manifest asks and the user approved; uninstalling it leaves the folder.
func TestAddonsDataFolderAndNetwork(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "net", manifestJSON("net", "network"), "exit 0\n")
	a.install(t, "quiet", manifestJSON("quiet", "playback.read"), "exit 0\n")
	specs := map[string]addons.Spec{}
	var mu sync.Mutex
	a.svc.opts.start = func(spec addons.Spec) (*addons.Proc, error) {
		mu.Lock()
		defer mu.Unlock()
		specs[filepath.Base(spec.Dir)] = spec
		return nil, &sandbox.Error{Reason: sandbox.ReasonSetup}
	}
	if err := a.svc.Enable("net", []string{"network"}); err != nil {
		t.Fatal(err)
	}
	if err := a.svc.Enable("quiet", []string{"playback.read"}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "net", AddonNoSandbox)
	a.waitState(t, "quiet", AddonNoSandbox)
	mu.Lock()
	defer mu.Unlock()
	if !specs["net"].Network || specs["quiet"].Network {
		t.Errorf("network: net %v, quiet %v", specs["net"].Network, specs["quiet"].Network)
	}
	data := a.e.Paths.AddonDataDir("net")
	if specs["net"].Data != data {
		t.Errorf("data folder %q, want %q", specs["net"].Data, data)
	}
	if fi, err := os.Stat(data); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("data folder: %v, %v", fi, err)
	}
	env := strings.Join(specs["net"].Env, "\n")
	for _, want := range []string{"SONA_ADDON_DATA=" + data, "HOME=" + data, "TMPDIR=" + filepath.Join(data, "tmp")} {
		if !strings.Contains(env, want) {
			t.Errorf("environment lacks %q:\n%s", want, env)
		}
	}
	// Uninstalling leaves the folder: it is the user's to remove.
	if err := os.WriteFile(filepath.Join(data, "state.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(a.root, "net")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "state.db")); err != nil {
		t.Errorf("uninstalling removed the data folder's content: %v", err)
	}
}

// The network is the sandbox's, fixed when the process starts, so taking the
// permission back ends the add-on instead of leaving it online.
func TestAddonsRevokingNetworkStopsTheAddon(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "net", manifestJSON("net", "network"), hello("net", "network")+"sleep 30\n")
	if err := a.svc.Enable("net", []string{"network"}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "net", AddonRunning)
	if err := a.svc.Revoke("net", "network"); err != nil {
		t.Fatal(err)
	}
	a.waitState(t, "net", AddonNeedsApproval)
	a.svc.mu.Lock()
	r := a.svc.runners["net"]
	a.svc.mu.Unlock()
	if r != nil {
		t.Error("the runner of an add-on that lost the network is still registered")
	}
}
