//go:build unix

package services

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/addons/sandbox"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
)

// useAll gives each of Sona's own add-ons something to do: both accounts
// connected, the Discord status on.
func (a *addonEnv) useAll(t *testing.T) {
	t.Helper()
	t.Setenv("SONA_DISCORD_CLIENT_ID", "12345")
	if _, err := a.settings.Update(func(s *config.Settings) {
		s.Scrobbling.LastFM = config.ScrobbleAccount{Enabled: true, Username: "u"}
		s.Scrobbling.ListenBrainz = config.ScrobbleAccount{Enabled: true, Username: "u"}
		s.Integrations.DiscordPresence = true
	}); err != nil {
		t.Fatal(err)
	}
}

// notices are the notification centre's "switched off" entries so far.
func (a *addonEnv) notices(t *testing.T) []Notification {
	t.Helper()
	list, err := a.e.listNotices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var out []Notification
	for _, n := range list.Items {
		if n.Kind == NoticeAddonOff {
			out = append(out, n)
		}
	}
	return out
}

// bundledProgram makes a script stand in for Sona's executable: it logs how
// it was started and says hello as the add-on named by its second argument.
func (a *addonEnv) bundledProgram(t *testing.T) (exe, started string) {
	t.Helper()
	dir := t.TempDir()
	exe, started = filepath.Join(dir, "sona"), filepath.Join(dir, "started.log")
	script := `#!/bin/sh
printf '%s|%s|%s|%s\n' "$1" "$2" "$SONA_ADDON_ID" "$(pwd)" >> ` + started + `
printf '{"id":1,"method":"hello","params":{"id":"%s","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":[]}}\n' "$2"
cat > /dev/null
`
	if err := os.WriteFile(exe, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.exe = exe
	return exe, started
}

// Sona's own add-ons are listed first-hand, on by default (idle until their
// integration is used), with their
// permissions granted and no approval to ask.
func TestAddonsBundledAreListedOnByDefault(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	got := map[string]AddonInfo{}
	for _, i := range a.svc.List() {
		got[i.ID] = i
	}
	for _, id := range addons.BundledIDs {
		i, ok := got[id]
		if !ok || !i.Bundled || !i.Enabled || i.Problem != "" || i.State != AddonIdle || !slices.Equal(i.Granted, i.Permissions) {
			t.Errorf("%s = %+v", id, i)
		}
	}
	if len(a.settings.Get().Addons) != 0 {
		t.Errorf("listing saved %+v", a.settings.Get().Addons)
	}
}

// They run as Sona's executable with --addon <id>, from its folder, with the
// ID in the environment, and the user's settings are only consulted for the
// switch.
func TestAddonsBundledRunAsSonaItself(t *testing.T) {
	a := newAddonEnv(t)
	exe, started := a.bundledProgram(t)
	a.useAll(t)
	a.svc.start()
	for _, id := range addons.BundledIDs {
		a.waitState(t, id, AddonRunning)
	}
	b, _ := os.ReadFile(started)
	real, _ := filepath.EvalSymlinks(filepath.Dir(exe))
	for _, id := range addons.BundledIDs {
		want := "--addon|" + id + "|" + id + "|" + real
		if !strings.Contains(string(b), want+"\n") {
			t.Errorf("no start %q in:\n%s", want, b)
		}
	}
}

// Sona's own add-ons have no switch of their own: whether one runs is its
// integration's (the connected account, the Discord status). Turning one off
// is refused and saves nothing, it goes on running, and "turn on" saves
// nothing either. Its permissions can't be revoked.
func TestAddonsBundledHaveNoSwitchOfTheirOwn(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.useAll(t)
	a.svc.start()
	a.waitState(t, "lastfm", AddonRunning)
	err := a.svc.Disable("lastfm")
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "Settings → Connections") {
		t.Fatalf("Disable = %v, want ErrInvalidArgument naming Settings → Connections", err)
	}
	if st, ok := a.settings.Get().Addons["lastfm"]; ok {
		t.Errorf("a refused Disable saved %+v", st)
	}
	if i := a.info(t, "lastfm"); !i.Enabled || i.State != AddonRunning {
		t.Errorf("lastfm after a refused Disable = %+v", i)
	}
	if err := a.svc.Enable("lastfm", a.info(t, "lastfm").Permissions); err != nil {
		t.Fatal(err)
	}
	if st, ok := a.settings.Get().Addons["lastfm"]; ok {
		t.Errorf("Enable saved %+v for an add-on that has no switch", st)
	}
	a.waitState(t, "lastfm", AddonRunning)
	for _, p := range a.info(t, "lastfm").Permissions {
		if err := a.svc.Revoke("lastfm", p); err == nil {
			t.Errorf("revoked %s from Sona's own add-on", p)
		}
	}
}

// A folder of the user's that claims a bundled ID is never run, never listed
// beside the real one, and cannot shadow it.
func TestAddonsUserFolderCannotClaimABundledID(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.useAll(t)
	marker := filepath.Join(t.TempDir(), "ran")
	a.install(t, "lastfm", manifestJSON("lastfm"), "touch "+marker+"\n")
	a.install(t, "discord", `{"id":"discord","name":"Fake","version":"9.9.9","command":"run","permissions":["like"]}`, "touch "+marker+"\n")
	a.svc.start()
	a.waitState(t, "lastfm", AddonRunning)
	n := 0
	for _, i := range a.svc.List() {
		if i.ID == "lastfm" || i.ID == "discord" {
			n++
			if !i.Bundled || i.Name == "Fake" {
				t.Errorf("%s shadowed: %+v", i.ID, i)
			}
		}
	}
	if n != 2 {
		t.Errorf("%d entries for lastfm and discord, want 2", n)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the user's folder program ran")
	}
	if err := a.svc.Enable("discord", []string{"like"}); err == nil {
		t.Error("enabled the folder's permissions on a bundled ID")
	}
}

// Sona that can't tell where it runs from refuses its own add-ons.
func TestAddonsBundledNeedAnExecutable(t *testing.T) {
	a := newAddonEnv(t) // exe is ""
	for _, id := range addons.BundledIDs {
		if i := a.info(t, id); i.State != AddonInvalid || i.Problem != addons.ProblemNoExecutable {
			t.Errorf("%s = %+v", id, i)
		}
	}
}

// An integration nobody uses costs no process: each add-on runs while its
// account is connected (or the Discord status is on), starts when that
// begins and stops when it ends.
func TestAddonsBundledRunOnlyWhileTheirIntegrationIsUsed(t *testing.T) {
	t.Setenv("SONA_DISCORD_CLIENT_ID", "12345")
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.svc.start()
	for _, id := range addons.BundledIDs {
		if i := a.info(t, id); i.State != AddonIdle || a.svc.runners[id] != nil {
			t.Fatalf("%s runs with nothing to do: %+v", id, i)
		}
	}
	set := func(fn func(*config.Settings)) {
		t.Helper()
		if _, err := a.settings.Update(fn); err != nil {
			t.Fatal(err)
		}
	}
	set(func(s *config.Settings) { s.Scrobbling.LastFM = config.ScrobbleAccount{Enabled: true, Username: "u"} })
	a.waitState(t, "lastfm", AddonRunning)
	if i := a.info(t, "listenbrainz"); i.State != AddonIdle {
		t.Errorf("listenbrainz = %+v", i)
	}
	// Switched off but still connected: queued listens are still delivered.
	set(func(s *config.Settings) { s.Scrobbling.LastFM.Enabled = false })
	if i := a.info(t, "lastfm"); i.State != AddonRunning {
		t.Errorf("a connected, paused lastfm = %+v", i)
	}
	set(func(s *config.Settings) { s.Scrobbling.LastFM = config.ScrobbleAccount{} })
	a.waitState(t, "lastfm", AddonIdle)
	set(func(s *config.Settings) { s.Integrations.DiscordPresence = true })
	a.waitState(t, "discord", AddonRunning)
	set(func(s *config.Settings) { s.Integrations.DiscordPresence = false })
	a.waitState(t, "discord", AddonIdle)
}

// A program of Sona's that stops at once, again and again (the loader that
// can't find its libraries, 127, was how it ended on NixOS) is never switched
// off by the crash rule: not in this run, where it goes on at the longest
// wait, and nothing is saved, so that no Sona that starts later finds it off.
// Meanwhile the add-on's info says why it isn't up; once the program runs, it
// is up and the reason is gone. An add-on of the user's is switched off still.
func TestAddonsBundledAreNeverSwitchedOffByTheCrashRule(t *testing.T) {
	a := newAddonEnv(t)
	exe, _ := a.bundledProgram(t)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho 'error while loading shared libraries: libgstapp-1.0.so.0' >&2\nexit 127\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.maxCrashes = 3
	a.install(t, "demo", manifestJSON("demo"), "exit 1\n")
	if err := a.svc.Enable("demo", nil); err != nil {
		t.Fatal(err)
	}
	a.useAll(t)
	a.svc.start()

	// Well past the rule's three, and the user's add-on is switched off by it.
	waitUntil(t, "lastfm to stop on its own more than maxCrashes times", func() bool { return a.info(t, "lastfm").Restarts > 2*a.svc.opts.maxCrashes })
	a.waitState(t, "demo", AddonCrashed)
	for _, id := range []string{"lastfm", "discord", "listenbrainz"} {
		i := a.info(t, id)
		if i.State == AddonCrashed || !i.Enabled {
			t.Errorf("%s = %+v: switched off by the crash rule", id, i)
		}
		if st, ok := a.settings.Get().Addons[id]; ok {
			t.Errorf("%s: the crash rule saved %+v", id, st)
		}
	}
	if i := a.info(t, "lastfm"); (i.State != AddonRestarting && i.State != AddonStarting) || i.Reason != AddonReasonEndedAtStart {
		t.Errorf("lastfm = %+v, want to be started again with the reason %q", i, AddonReasonEndedAtStart)
	}
	if i := a.info(t, "demo"); i.Enabled || i.Reason != AddonReasonEndedAtStart {
		t.Errorf("demo = %+v, want switched off", i)
	}
	if st := a.settings.Get().Addons["demo"]; st.Enabled {
		t.Errorf("the user's add-on stayed on in the settings: %+v", st)
	}
	if got := a.notices(t); len(got) != 1 || got[0].Subject != "Add-on demo" {
		t.Errorf("notices = %+v, want one, for the user's add-on that was switched off", got)
	}
	lines, _ := a.svc.Log("lastfm")
	if log := strings.Join(lines, "\n"); !strings.Contains(log, "never switched off") || strings.Contains(log, ": switched off") {
		t.Errorf("lastfm's log = %q", log)
	}

	// The program starts to run: the same supervisor takes it up with no
	// one's help, and the reason goes.
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nprintf '{\"id\":1,\"method\":\"hello\",\"params\":{\"id\":\"lastfm\",\"protocol\":{\"min\":\"2.0.0\",\"max\":\"2.0.0\"},\"permissions\":[]}}\\n'\ncat >/dev/null\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if i := a.waitState(t, "lastfm", AddonRunning); i.Reason != "" || i.Restarts == 0 {
		t.Errorf("lastfm = %+v, want running with no reason", i)
	}
}

// What rc.3 saved (all three bundled add-ons off after five quick stops) is
// healed at the next start: it is never read, the file says on again, and the
// add-ons run. A user's add-on that is off stays off.
func TestAddonsBundledStuckSwitchesHealAtStart(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.install(t, "demo", manifestJSON("demo"), hello("demo")+"cat > /dev/null\n")
	if _, err := a.settings.Update(func(s *config.Settings) {
		s.Addons = config.Addons{
			"discord":      {Enabled: false, Granted: []string{"playback.read"}},
			"lastfm":       {Enabled: false, Granted: []string{"network"}},
			"listenbrainz": {Enabled: false},
			"demo":         {Enabled: false},
		}
	}); err != nil {
		t.Fatal(err)
	}
	a.useAll(t)
	// Before the heal: nothing is read from the stuck value.
	if i := a.info(t, "lastfm"); !i.Enabled {
		t.Fatalf("lastfm = %+v: the stuck switch is read", i)
	}
	a.svc.start()
	for _, id := range addons.BundledIDs {
		a.waitState(t, id, AddonRunning)
		if st := a.settings.Get().Addons[id]; !st.Enabled {
			t.Errorf("%s still saved as off: %+v", id, st)
		}
	}
	if st := a.settings.Get().Addons["lastfm"]; !slices.Equal(st.Granted, []string{"network"}) {
		t.Errorf("the heal changed more than the switch: %+v", st)
	}
	if st := a.settings.Get().Addons["demo"]; st.Enabled {
		t.Errorf("the heal switched on an add-on of the user's: %+v", st)
	}
	if i := a.info(t, "demo"); i.State != AddonOff {
		t.Errorf("demo = %+v", i)
	}
}

// Turning a bundled add-on on again is its retry: one that waits out its
// longest backoff is begun at once.
func TestAddonsBundledEnableTriesAgainAtOnce(t *testing.T) {
	a := newAddonEnv(t)
	exe, _ := a.bundledProgram(t)
	good, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 127\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.backoff = []time.Duration{time.Hour}
	a.useAll(t)
	a.svc.start()
	a.waitState(t, "lastfm", AddonRestarting)
	if err := os.WriteFile(exe, good, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := a.svc.Enable("lastfm", a.info(t, "lastfm").Permissions); err != nil {
		t.Fatal(err)
	}
	a.waitState(t, "lastfm", AddonRunning)
}

// Sona's own add-ons are started with the loader's path (LD_LIBRARY_PATH): a
// program that needs it to load ends with status 127 without it. The plain
// child that runs where the sandbox can't start gets it too, which it didn't
// (it ended at once, five times, and was switched off); an add-on of the
// user's never has the variable.
func TestBundledAddonsGetTheLoaderPathInBothStarts(t *testing.T) {
	a := newAddonEnv(t)
	exe, _ := a.bundledProgram(t)
	const path = "/nix/store/a-glib/lib:/nix/store/b-gstreamer/lib"
	script := "#!/bin/sh\n[ \"$LD_LIBRARY_PATH\" = '" + path + "' ] || exit 127\n" +
		"printf '{\"id\":1,\"method\":\"hello\",\"params\":{\"id\":\"%s\",\"protocol\":{\"min\":\"2.0.0\",\"max\":\"2.0.0\"},\"permissions\":[]}}\\n' \"$2\"\ncat > /dev/null\n"
	if err := os.WriteFile(exe, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.environ = func() []string { return append(os.Environ(), "LD_LIBRARY_PATH="+path) }
	a.install(t, "demo", manifestJSON("demo"), "[ -z \"$LD_LIBRARY_PATH\" ] || exit 127\n"+hello("demo")+"cat > /dev/null\n")
	if err := a.svc.Enable("demo", nil); err != nil {
		t.Fatal(err)
	}
	a.useAll(t)
	a.svc.start()
	a.waitState(t, "lastfm", AddonRunning)
	a.waitState(t, "demo", AddonRunning)
	if r := a.info(t, "lastfm").Restarts; r != 0 {
		t.Errorf("lastfm stopped %d times", r)
	}

	// Where the sandbox can't start, the plain start has it as well.
	b := newAddonEnv(t)
	exe2, _ := b.bundledProgram(t)
	if err := os.WriteFile(exe2, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	b.svc.opts.environ = a.svc.opts.environ
	b.svc.opts.start = func(sp addons.Spec) (*addons.Proc, error) {
		if !sp.Unsandboxed {
			return nil, &sandbox.Error{Reason: sandbox.ReasonSetup, Detail: "the sandbox's init ended without a report (exit status 127)"}
		}
		return addons.Start(sp)
	}
	b.useAll(t)
	b.svc.start()
	i := b.waitState(t, "lastfm", AddonRunning)
	if i.Unsandboxed != addons.SandboxSetup || i.Restarts != 0 || i.Reason != "" {
		t.Errorf("lastfm = %+v, want to run without the sandbox, not restarted", i)
	}
}

// A bundled add-on reads its own sign-in with the secret method, no other
// key and no other add-on's; a third-party add-on reads none of them.
func TestAddonsBundledReadTheirOwnSignInOnly(t *testing.T) {
	a := newAddonEnv(t)
	kr := &memKeyring{}
	a.e.Keyring = kr
	_ = kr.Set(store.Service, "addon.lastfm", `{"session":"sk-1","other":"x"}`)
	_ = kr.Set(store.Service, "addon.listenbrainz", `{"token":"tok-1"}`)
	_ = kr.Set(store.Service, "addon.demo", `{"session":"sneaky"}`)
	own := func(id string) *addonOwn {
		m, err := addons.BundledManifest(id)
		if err != nil {
			t.Fatal(err)
		}
		return newAddonOwn(a.svc, m, false)
	}
	if v, ok := own("lastfm").Secret("session"); !ok || v != "sk-1" {
		t.Errorf("lastfm session = %q, %v", v, ok)
	}
	if _, ok := own("lastfm").Secret("other"); ok {
		t.Error("lastfm read a key that is not its sign-in")
	}
	if _, ok := own("lastfm").Secret("token"); ok {
		t.Error("lastfm read listenbrainz's key")
	}
	if v, ok := own("listenbrainz").Secret("token"); !ok || v != "tok-1" {
		t.Errorf("listenbrainz token = %q, %v", v, ok)
	}
	// A third-party add-on named like a sign-in key has no such row.
	third := addons.Manifest{ID: "demo", Name: "Demo", Version: "1.0.0", Command: "run"}
	if _, ok := newAddonOwn(a.svc, third, false).Secret("session"); ok {
		t.Error("a third-party add-on read a bundled sign-in key")
	}
	// A development run gets no secret at all.
	m, _ := addons.BundledManifest("lastfm")
	if _, ok := newAddonOwn(a.svc, m, true).Secret("session"); ok {
		t.Error("a development run read a secret")
	}
}

// The build credentials reach a bundled add-on in its hello answer and no
// add-on of the user's; the environment never carries them.
func TestAddonsBundledGetBuildCredentialsInHelloOnly(t *testing.T) {
	t.Setenv("SONA_LASTFM_KEY", "the-key")
	t.Setenv("SONA_LASTFM_SECRET", "the-secret")
	t.Setenv("SONA_DISCORD_CLIENT_ID", "12345")
	a := newAddonEnv(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "sona")
	script := "#!/bin/sh\nprintf '{\"id\":1,\"method\":\"hello\",\"params\":{\"id\":\"%s\",\"protocol\":{\"min\":\"2.0.0\",\"max\":\"2.0.0\"},\"permissions\":[]}}\\n' \"$2\"\nread line\nprintf '%s\\n' \"$line\" > " + dir + "/hello-$2.json\nenv > " + dir + "/env-$2.txt\ncat > /dev/null\n"
	if err := os.WriteFile(exe, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.exe = exe
	a.useAll(t)
	a.install(t, "demo", manifestJSON("demo"), hello("demo")+"read line\nprintf '%s\\n' \"$line\" > hello.json\nsleep 300\n")
	if err := a.svc.Enable("demo", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	waitUntil(t, "hello answers", func() bool {
		for _, f := range []string{"hello-lastfm.json", "hello-discord.json", "hello-listenbrainz.json"} {
			if b, _ := os.ReadFile(filepath.Join(dir, f)); len(b) == 0 {
				return false
			}
		}
		b, _ := os.ReadFile(filepath.Join(a.root, "demo", "hello.json"))
		return len(b) > 0
	})
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	if h := read(filepath.Join(dir, "hello-lastfm.json")); !strings.Contains(h, `"apiKey":"the-key"`) || !strings.Contains(h, `"apiSecret":"the-secret"`) {
		t.Errorf("lastfm hello = %s", h)
	}
	if h := read(filepath.Join(dir, "hello-discord.json")); !strings.Contains(h, `"applicationId":"12345"`) || strings.Contains(h, "apiKey") {
		t.Errorf("discord hello = %s", h)
	}
	for _, f := range []string{"hello-listenbrainz.json"} {
		if h := read(filepath.Join(dir, f)); strings.Contains(h, "credentials") {
			t.Errorf("%s = %s", f, h)
		}
	}
	if h := read(filepath.Join(a.root, "demo", "hello.json")); strings.Contains(h, "credentials") || strings.Contains(h, "the-key") {
		t.Errorf("a user's add-on got credentials: %s", h)
	}
	for _, id := range addons.BundledIDs {
		if env := read(filepath.Join(dir, "env-"+id+".txt")); strings.Contains(env, "the-key") || strings.Contains(env, "the-secret") || strings.Contains(env, "12345") {
			t.Errorf("%s: credentials in the environment", id)
		}
	}
}

// Where the sandbox can't be set up, one of Sona's own add-ons runs anyway as
// a plain child process (the same code ran in Sona's process before #302):
// it says hello, the state shows why it is not isolated, and the log notes it.
// An add-on of the user's, in the same simulated failure, is still refused and
// never offered the unsandboxed start.
func TestBundledAddonRunsWithoutTheSandboxWhereItCannotStart(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.install(t, "demo", manifestJSON("demo", "playback.read"), "touch ran\n"+hello("demo", "playback.read")+"sleep 30\n")
	var mu sync.Mutex
	plain := map[string]bool{}
	a.svc.opts.start = func(sp addons.Spec) (*addons.Proc, error) {
		if !sp.Unsandboxed {
			return nil, &sandbox.Error{Reason: sandbox.ReasonUserNamespaceRestricted, Detail: "simulated"}
		}
		mu.Lock()
		plain[sp.ID] = true
		mu.Unlock()
		return addons.Start(sp)
	}
	a.useAll(t)
	if err := a.svc.Enable("demo", []string{"playback.read"}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()

	i := a.waitState(t, "lastfm", AddonRunning)
	if i.Unsandboxed != addons.SandboxUserNamespaceRestricted || i.Sandbox != "" || !i.Bundled {
		t.Errorf("lastfm = %+v", i)
	}
	lines, _ := a.svc.Log("lastfm")
	log := strings.Join(lines, "\n")
	if !strings.Contains(log, "running without the sandbox: "+addons.SandboxUserNamespaceRestricted) {
		t.Errorf("log = %q", log)
	}

	d := a.waitState(t, "demo", AddonNoSandbox)
	if d.Unsandboxed != "" || d.Sandbox != addons.SandboxUserNamespaceRestricted {
		t.Errorf("demo = %+v", d)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(a.root, "demo", "ran")); err == nil {
		t.Error("an add-on of the user's ran without its sandbox")
	}
	mu.Lock()
	defer mu.Unlock()
	if plain["demo"] || !plain["lastfm"] {
		t.Errorf("unsandboxed starts = %v", plain)
	}
}
