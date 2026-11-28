//go:build linux

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
)

func mustManifest(t *testing.T, text string) addons.Manifest {
	t.Helper()
	m, err := addons.ParseManifest([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// devFolder writes an add-on folder outside the add-ons folder (0700, the
// program 0700) and returns it.
func devFolder(t *testing.T, id, manifest, script string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	if err := os.Mkdir(dir, 0o700); err != nil {
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

// waitAsk waits for the question the command put in Sona's window.
func (a *addonEnv) waitAsk(t *testing.T) AddonDevAsk {
	t.Helper()
	var q AddonDevAsk
	waitUntil(t, "the question", func() bool {
		a.rec.mu.Lock()
		defer a.rec.mu.Unlock()
		for i, name := range a.rec.events {
			if name == EventAddonsDevAsk {
				q = a.rec.data[i].(AddonDevAsk)
				return true
			}
		}
		return false
	})
	return q
}

func (a *addonEnv) asked() int {
	a.rec.mu.Lock()
	defer a.rec.mu.Unlock()
	n := 0
	for _, name := range a.rec.events {
		if name == EventAddonsDevAsk {
			n++
		}
	}
	return n
}

type devResult struct {
	d   cli.AddonDev
	err error
}

// startDev runs DevStart off the test's goroutine, as the command server does.
func (a *addonEnv) startDev(dir string) <-chan devResult {
	out := make(chan devResult, 1)
	go func() {
		d, err := a.svc.DevStart(context.Background(), dir)
		out <- devResult{d, err}
	}()
	return out
}

func recv(t *testing.T, ch <-chan devResult) devResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(hangGuard):
		t.Fatal("DevStart never returned")
		return devResult{}
	}
}

func devAddonScript(mark string) string {
	return "MARK=" + mark + "\n" + hello("devadd", "playback.read") + `printf 'dev add-on says hi\n' >&2
printf 'ran\n' > "$MARK"
while read -r line; do :; done
`
}

// The run needs the person's yes in Sona's window; then it streams its log,
// is given the manifest's permissions for the run, saves nothing, and ends
// when its client says so.
func TestDevRunsAfterTheConfirmationAndStreamsItsLog(t *testing.T) {
	a := newAddonEnv(t)
	mark := filepath.Join(t.TempDir(), "mark")
	dir := devFolder(t, "devadd", manifestJSON("devadd", "playback.read"), devAddonScript(mark))
	before := a.e.Settings.Get().Addons

	res := a.startDev(dir)
	q := a.waitAsk(t)
	if q.ID != "devadd" || q.Dir != dir || q.Name != "Add-on devadd" || !reflect.DeepEqual(q.Permissions, []string{"playback.read"}) || q.Token == "" {
		t.Errorf("question = %+v", q)
	}
	// Nothing runs while the person hasn't answered.
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("the add-on ran before the person confirmed")
	}
	a.svc.AnswerDev(q.Token, true)
	r := recv(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.d.ID != "devadd" || r.d.Token == "" || r.d.Token == q.Token {
		t.Errorf("answer = %+v", r.d)
	}

	// Its log arrives by poll, with Sona's own note and the add-on's line.
	var lines []string
	next := int64(0)
	waitUntil(t, "the add-on's line", func() bool {
		l, err := a.svc.DevLog(r.d.Token, next)
		if err != nil {
			t.Fatal(err)
		}
		lines, next = append(lines, l.Lines...), l.Next
		return strings.Contains(strings.Join(lines, "\n"), "dev add-on says hi")
	})
	if !strings.Contains(strings.Join(lines, "\n"), "its permissions are held for this run only") {
		t.Errorf("lines = %q", lines)
	}
	// It is a session like any other: it said hello and holds only the manifest's permissions.
	waitUntil(t, "hello", func() bool {
		a.svc.mu.Lock()
		defer a.svc.mu.Unlock()
		r := a.svc.runners["devadd"]
		st, _ := r.snapshot()
		return st == AddonRunning
	})
	// Nothing was saved: no grant, no setting.
	if got := a.e.Settings.Get().Addons; !reflect.DeepEqual(got, before) {
		t.Errorf("the run changed the saved add-ons: %+v, was %+v", got, before)
	}
	// It is not installed: Settings → Add-ons doesn't list it.
	for _, i := range a.svc.List() {
		if i.ID == "devadd" {
			t.Errorf("a development run is listed as installed: %+v", i)
		}
	}

	if err := a.svc.DevStop(r.d.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := a.svc.DevLog(r.d.Token, 0); !errors.Is(err, ErrNotFoundAddon) {
		t.Errorf("the token of a stopped run: %v", err)
	}
	a.svc.mu.Lock()
	_, left := a.svc.runners["devadd"]
	a.svc.mu.Unlock()
	if left {
		t.Error("the stopped run is still a runner")
	}
}

func TestDevRefusesWithoutTheConfirmation(t *testing.T) {
	for name, answer := range map[string]func(a *addonEnv, q AddonDevAsk){
		"no":          func(a *addonEnv, q AddonDevAsk) { a.svc.AnswerDev(q.Token, false) },
		"wrong token": func(a *addonEnv, q AddonDevAsk) { a.svc.AnswerDev("not-the-token", true) },
		"too late":    func(a *addonEnv, q AddonDevAsk) {},
	} {
		t.Run(name, func(t *testing.T) {
			a := newAddonEnv(t)
			a.svc.opts.devAsk = 600 * time.Millisecond
			mark := filepath.Join(t.TempDir(), "mark")
			dir := devFolder(t, "devadd", manifestJSON("devadd"), devAddonScript(mark))
			res := a.startDev(dir)
			answer(a, a.waitAsk(t))
			r := recv(t, res)
			var ce *cli.Error
			if !errors.As(r.err, &ce) || ce.Code != cli.CodeDenied {
				t.Fatalf("DevStart = %+v, %v", r.d, r.err)
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := os.Stat(mark); err == nil {
				t.Error("the add-on ran without a yes")
			}
		})
	}
}

// Only one question at a time: a second is refused, so nothing buries the window.
func TestDevAsksOneQuestionAtATime(t *testing.T) {
	a := newAddonEnv(t)
	mark := filepath.Join(t.TempDir(), "mark")
	first := a.startDev(devFolder(t, "devadd", manifestJSON("devadd"), devAddonScript(mark)))
	q := a.waitAsk(t)
	second := recv(t, a.startDev(devFolder(t, "devtwo", manifestJSON("devtwo"), "exit 0\n")))
	var ce *cli.Error
	if !errors.As(second.err, &ce) || ce.Code != "busy" {
		t.Errorf("second = %v", second.err)
	}
	if n := a.asked(); n != 1 {
		t.Errorf("%d questions asked", n)
	}
	a.svc.AnswerDev(q.Token, false)
	recv(t, first)
}

func TestDevRefusesWhatInstallingWouldRefuseAndAnInstalledID(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "taken", manifestJSON("taken"), "exit 0\n")
	// A refused folder holding the same name still counts as installed.
	if err := os.MkdirAll(filepath.Join(a.root, "broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.root, "broken", "addon.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	groupW := devFolder(t, "sloppy", manifestJSON("sloppy"), "exit 0\n")
	if err := os.Chmod(groupW, 0o770); err != nil {
		t.Fatal(err)
	}
	linkTarget := devFolder(t, "linked", manifestJSON("linked"), "exit 0\n")
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(linkTarget, link); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(a.root, "inside")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "addon.json"), []byte(manifestJSON("inside")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "run"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	notExec := devFolder(t, "plain", manifestJSON("plain"), "exit 0\n")
	if err := os.Chmod(filepath.Join(notExec, "run"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{
		"an installed ID":              devFolder(t, "taken", manifestJSON("taken"), "exit 0\n"),
		"a reserved ID":                devFolder(t, "lastfm", manifestJSON("lastfm"), "exit 0\n"),
		"a folder writable by others":  groupW,
		"a link to a folder":           link,
		"a folder in the add-ons one":  inside,
		"a program that isn't one":     notExec,
		"a relative path":              "relative/path",
		"a folder that doesn't exist":  filepath.Join(t.TempDir(), "nothing"),
		"a name that is installed too": devFolder(t, "broken", manifestJSON("broken"), "exit 0\n"),
	} {
		t.Run(name, func(t *testing.T) {
			r := recv(t, a.startDev(dir))
			if !errors.Is(r.err, ErrInvalidArgument) {
				t.Errorf("DevStart = %+v, %v", r.d, r.err)
			}
			if n := a.asked(); n != 0 {
				t.Errorf("the person was asked about it (%d questions)", n)
			}
		})
	}
}

// A folder that changes while the person is being asked is not what they confirmed.
func TestDevRefusesAFolderThatChangedWhileAsking(t *testing.T) {
	a := newAddonEnv(t)
	mark := filepath.Join(t.TempDir(), "mark")
	dir := devFolder(t, "devadd", manifestJSON("devadd", "playback.read"), devAddonScript(mark))
	res := a.startDev(dir)
	q := a.waitAsk(t)
	// More permissions than the person was shown.
	if err := os.WriteFile(filepath.Join(dir, "addon.json"), []byte(manifestJSON("devadd", "playback.read", "playback.control", "volume")), 0o600); err != nil {
		t.Fatal(err)
	}
	a.svc.AnswerDev(q.Token, true)
	r := recv(t, res)
	if !errors.Is(r.err, ErrInvalidArgument) {
		t.Fatalf("DevStart = %+v, %v", r.d, r.err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(mark); err == nil {
		t.Error("the add-on ran with permissions the person never saw")
	}
}

// A run whose client stopped asking is ended, and a dev add-on is held to the
// manifest's permissions without any grant being saved.
func TestDevEndsWhenItsClientStopsAskingAndHoldsOnlyTheManifestsPermissions(t *testing.T) {
	a := newAddonEnv(t)
	// The lease is read off the services' clock, which stands still until the
	// test moves it: the run lives while its client asks however slowly a
	// loaded machine answers, and ends when the test says a lease has passed.
	// (The lease looks at that clock every tenth of a lease in real time,
	// which only decides how soon it notices.)
	clk := &testClock{t: time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)}
	a.e.Now = clk.Now
	a.svc.opts.devLease = 400 * time.Millisecond
	mark := filepath.Join(t.TempDir(), "mark")
	// It asks for the volume (not in its manifest) and logs what Sona answered.
	script := "MARK=" + mark + "\n" + hello("devadd", "playback.read", "volume") + `
printf '{"id":2,"method":"call","params":{"command":"volume"}}\n'
read -r first; read -r second
printf 'answer: %s\n' "$second" >&2
while read -r line; do :; done
`
	dir := devFolder(t, "devadd", manifestJSON("devadd", "playback.read"), script)
	res := a.startDev(dir)
	a.svc.AnswerDev(a.waitAsk(t).Token, true)
	r := recv(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	run := a.svc.devRun(r.d.Token)
	var lines []string
	next := int64(0)
	waitUntil(t, "the refusal in its log", func() bool {
		l, _ := a.svc.DevLog(r.d.Token, next)
		lines, next = append(lines, l.Lines...), l.Next
		return strings.Contains(strings.Join(lines, "\n"), "permission_denied")
	})
	select {
	case <-run.runner.done:
		t.Fatal("the run ended while its client was asking")
	default:
	}
	// No more asking: a lease passes and the run ends. Its runner is done
	// once it has left the runners and its buffer is ended.
	clk.set(clk.Now().Add(a.svc.opts.devLease + time.Millisecond))
	waitClosed(t, run.runner.done, "the run to end")
	a.svc.mu.Lock()
	_, running := a.svc.runners["devadd"]
	a.svc.mu.Unlock()
	if running {
		t.Error("the ended run is still registered")
	}
	if l := run.buf.since(0); !l.Ended || l.Exit == "" {
		t.Errorf("the buffer of the ended run = %+v", l)
	}
	if got := a.e.Settings.Get().Addons["devadd"]; !reflect.DeepEqual(got, config.AddonSetting{}) {
		t.Errorf("a grant or setting was saved: %+v", got)
	}
}

// A development add-on is given the defaults of its rows and none of the
// user's saved values or secrets, even when its ID once was an installed
// add-on's.
func TestDevAddonGetsNeitherSavedValuesNorSecrets(t *testing.T) {
	a, kr := rowsEnv(t)
	m := mustManifest(t, manifestWith("devadd", rowsManifest))
	if _, err := a.e.updateSettings(func(s *config.Settings) {
		s.Addons = map[string]config.AddonSetting{"devadd": {Values: map[string]string{"url": "https://private.example/", "post": "false"}}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set("sona", "addon.devadd", `{"token":"hunter2"}`); err != nil {
		t.Fatal(err)
	}
	dev, installed := newAddonOwn(a.svc, m, true), newAddonOwn(a.svc, m, false)
	if v, ok := dev.Secret("token"); ok || v != "" {
		t.Errorf("a development add-on read a secret: %q", v)
	}
	if v, ok := installed.Secret("token"); !ok || v != "hunter2" {
		t.Errorf("control: an installed add-on's secret = %q, %v", v, ok)
	}
	got, want := dev.values(nil), installed.values(a.e.Settings.Get().Addons["devadd"].Values)
	if got.Values["url"] != "" || got.Values["post"] != "true" {
		t.Errorf("a development add-on's values = %+v, want the defaults", got.Values)
	}
	if want.Values["url"] != "https://private.example/" {
		t.Errorf("control: %+v", want.Values)
	}
	// Publishing for a development add-on never reads the saved values.
	ch, cancel := dev.WatchSettings()
	defer cancel()
	if v := (<-ch).(cli.SettingsValues); v.Values["url"] != "" || v.Values["post"] != "true" {
		t.Errorf("published %+v", v)
	}
}

// A development run's actions are offered while it runs, with no rows.
func TestDevRunsActionsAreOfferedWhileItRuns(t *testing.T) {
	a := newAddonEnv(t)
	script := hello("devadd") + `
while read -r line; do
  case $line in
    *'"method":"invoke"'*)
      id=${line#'{"id":'}; id=${id%%,*}
      printf '{"id":%s,"result":{"toast":"from dev"}}\n' "$id" ;;
  esac
done
`
	dir := devFolder(t, "devadd", manifestWith("devadd", sendAction), script)
	res := a.startDev(dir)
	a.svc.AnswerDev(a.waitAsk(t).Token, true)
	r := recv(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	waitUntil(t, "the contribution", func() bool {
		for _, c := range a.svc.Contributions("en") {
			if c.ID == "devadd" && c.Running && len(c.Actions) == 1 && len(c.Rows) == 0 {
				return true
			}
		}
		return false
	})
	got, err := a.svc.Invoke(t.Context(), "devadd", "addon.devadd.send", &AddonTarget{Kind: "album", ID: "1", Name: "x"})
	if err != nil || got.Toast != "from dev" {
		t.Errorf("invoke = %+v, %v", got, err)
	}
	if err := a.svc.DevStop(r.d.Token); err != nil {
		t.Fatal(err)
	}
	for _, c := range a.svc.Contributions("en") {
		if c.ID == "devadd" {
			t.Errorf("a stopped run still contributes: %+v", c)
		}
	}
}

// `addons` is the person's: the command server answers it, an add-on's
// channel refuses it, and a request must carry an absolute folder.
func TestDevCommandIsNotReachableFromAnAddonChannelAndNeedsAnAbsoluteFolder(t *testing.T) {
	if _, ok := cli.RequestPermission(cli.Request{Command: "addons"}); ok {
		t.Error("an add-on may send `addons`")
	}
	a := newAddonEnv(t)
	reply := cli.Handle(t.Context(), a.srv, []byte(`{"command":"addons","args":["dev","relative"]}`))
	if reply.Error == nil || reply.Error.Code != cli.CodeUsage {
		t.Errorf("a relative folder: %+v", reply)
	}
	reply = cli.Handle(t.Context(), a.srv, []byte(`{"command":"addons","args":["dev-log","nope","0"]}`))
	if reply.Error == nil || reply.Error.Code != string(CodeNotFound) {
		t.Errorf("an unknown token: %+v", reply.Error)
	}
	reply = cli.Handle(t.Context(), a.srv, []byte(`{"command":"addons","args":["evil"]}`))
	if reply.Error == nil || reply.Error.Code != cli.CodeUsage {
		t.Errorf("an unknown verb: %+v", reply.Error)
	}
}

// handleTransport is the CLI's transport straight into the command server.
type handleTransport struct {
	t   *testing.T
	srv cli.Server
}

func (h handleTransport) Call(ctx context.Context, req cli.Request) (json.RawMessage, error) {
	raw, _ := json.Marshal(req)
	return cli.DecodeReply(mustJSON(cli.Handle(ctx, h.srv, raw)))
}

func mustJSON(r cli.Reply) []byte { b, _ := json.Marshal(r); return b }

func (handleTransport) Follow(context.Context, func(cli.Status) error) error { return nil }
func (handleTransport) Close() error                                         { return nil }

// `sona addons dev` end to end: the client asks, the person answers in the
// window, the add-on runs, its log reaches the terminal stripped of escape
// sequences, and the client leaves when the add-on does.
func TestDevThroughTheCommandLine(t *testing.T) {
	a := newAddonEnv(t)
	script := hello("devadd") + `printf 'line \033[31mone\033[0m\n' >&2
printf 'line two\n' >&2
sleep 1
`
	dir := devFolder(t, "devadd", manifestJSON("devadd"), script)
	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(t.Context(), []string{"addons", "dev", dir}, cli.Env{
			Stdout: &out, Stderr: &errOut, Terminal: true, Dial: func() (cli.Transport, error) { return handleTransport{t, a.srv}, nil },
		})
	}()
	q := a.waitAsk(t)
	if q.Dir != dir {
		t.Errorf("the question named %q, want %q", q.Dir, dir)
	}
	a.svc.AnswerDev(q.Token, true)
	select {
	case code := <-done:
		if code != cli.ExitOK {
			t.Fatalf("exit %d, stderr %q", code, errOut.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the command never returned")
	}
	// Sona's log already replaced the escape character; the terminal gets none.
	if !strings.Contains(out.String(), "one") || !strings.Contains(out.String(), "line two\n") || strings.Contains(out.String(), "\x1b") {
		t.Errorf("stdout %q", out.String())
	}
	if !strings.Contains(errOut.String(), "Running Add-on devadd (devadd) from "+dir) || !strings.Contains(errOut.String(), "ended") {
		t.Errorf("stderr %q", errOut.String())
	}
}

// waitClosed waits for ch to close. It lives here, beside its one user, as
// this file is Linux-only and addons_test.go also builds on macOS.
func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(hangGuard):
		t.Fatalf("timed out waiting for %s", what)
	}
}
