//go:build unix

package services

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/addons"
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

// Turning one off is saved (it is on without an entry), stops it, and stays
// off; turning it on again runs it. Its permissions can't be revoked.
func TestAddonsBundledSwitchAndRevoke(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.useAll(t)
	a.svc.start()
	a.waitState(t, "lastfm", AddonRunning)
	if err := a.svc.Disable("lastfm"); err != nil {
		t.Fatal(err)
	}
	if st, ok := a.settings.Get().Addons["lastfm"]; !ok || st.Enabled {
		t.Errorf("after disable: %+v, saved %v", st, ok)
	}
	a.waitState(t, "lastfm", AddonOff)
	if i := a.info(t, "listenbrainz"); !i.Enabled {
		t.Errorf("turning lastfm off turned listenbrainz off: %+v", i)
	}
	if err := a.svc.Enable("lastfm", a.info(t, "lastfm").Permissions); err != nil {
		t.Fatal(err)
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

// An add-on that crashes for good stays reported as crashed, and turning it
// on again runs it.
func TestAddonsBundledCrashedStaysCrashedUntilTurnedOn(t *testing.T) {
	a := newAddonEnv(t)
	exe, _ := a.bundledProgram(t)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.maxCrashes = 3
	a.useAll(t)
	a.svc.start()
	a.waitState(t, "lastfm", AddonCrashed)
	waitUntil(t, "the switch to be saved off", func() bool { return !a.info(t, "lastfm").Enabled })
	if st := a.settings.Get().Addons["lastfm"]; st.Enabled {
		t.Errorf("settings %+v", st)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nprintf '{\"id\":1,\"method\":\"hello\",\"params\":{\"id\":\"lastfm\",\"protocol\":{\"min\":\"2.0.0\",\"max\":\"2.0.0\"},\"permissions\":[]}}\\n'\ncat >/dev/null\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := a.svc.Enable("lastfm", a.info(t, "lastfm").Permissions); err != nil {
		t.Fatal(err)
	}
	a.waitState(t, "lastfm", AddonRunning)
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
