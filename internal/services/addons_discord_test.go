//go:build unix

package services

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/discord"
)

// runtimeDir is a fresh $XDG_RUNTIME_DIR (short, owned by the user, owner
// only) and a way to make sockets in it.
func runtimeDir(t *testing.T) (dir string, listen func(rel string) string) {
	t.Helper()
	base := "" // under /tmp where it exists: macOS' temporary folder leaves a socket path too little room
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "xrd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	return dir, func(rel string) string {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		l, err := net.Listen("unix", p)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		return p
	}
}

// specs records every spec the supervisor starts an add-on with.
type specs struct {
	mu  sync.Mutex
	all []addons.Spec
}

func (s *specs) of(id string) []addons.Spec {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []addons.Spec
	for _, sp := range s.all {
		if sp.ID == id {
			out = append(out, sp)
		}
	}
	return out
}

func (a *addonEnv) recordSpecs() *specs {
	rec := &specs{}
	start := a.svc.opts.start
	a.svc.opts.start = func(sp addons.Spec) (*addons.Proc, error) {
		rec.mu.Lock()
		rec.all = append(rec.all, sp)
		rec.mu.Unlock()
		return start(sp)
	}
	return rec
}

// The one sandbox exception: the Discord add-on is given Discord's IPC
// sockets (the runtime folder's, the Flatpak's and the Snap's, as many as
// are there and the user's) and nothing else of the runtime folder, no
// network, and no other add-on is given a socket.
func TestDiscordAddonIsGivenDiscordsSocketsOnly(t *testing.T) {
	dir, listen := runtimeDir(t)
	want := []string{
		listen("discord-ipc-0"), listen("discord-ipc-9"),
		listen("app/com.discordapp.Discord/discord-ipc-3"), listen("snap.discord/discord-ipc-1"),
	}
	bus := listen("bus")
	for _, decoy := range []string{"wayland-0", "pipewire-0", "discord-ipc-10", "discord-ipc-0.lock", "app/com.example.Other/discord-ipc-0", "snap.other/discord-ipc-0"} {
		listen(decoy)
	}
	if err := os.WriteFile(filepath.Join(dir, "discord-ipc-2"), nil, 0o600); err != nil { // a file, not a socket
		t.Fatal(err)
	}
	if err := os.Symlink(bus, filepath.Join(dir, "discord-ipc-4")); err != nil { // a link to another socket
		t.Fatal(err)
	}

	a := newAddonEnv(t)
	a.bundledProgram(t)
	rec := a.recordSpecs()
	a.useAll(t)
	a.svc.start()
	for _, id := range addons.BundledIDs {
		a.waitState(t, id, AddonRunning)
	}

	d := rec.of("discord")
	if len(d) != 1 {
		t.Fatalf("discord started %d times", len(d))
	}
	got := slices.Clone(d[0].Sockets)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("sockets = %v, want exactly %v", got, want)
	}
	if d[0].Network {
		t.Error("the Discord add-on has the network")
	}
	if !slices.Contains(d[0].Env, discord.EnvSockets+"="+discord.SocketsEnv(discord.Sockets())) {
		t.Errorf("the add-on is not told its sockets: %v", d[0].Env)
	}
	for _, id := range []string{"lastfm", "listenbrainz"} {
		for _, sp := range rec.of(id) {
			if len(sp.Sockets) != 0 {
				t.Errorf("%s was given sockets %v", id, sp.Sockets)
			}
			if slices.ContainsFunc(sp.Env, func(kv string) bool { return strings.HasPrefix(kv, discord.EnvSockets+"=") }) {
				t.Errorf("%s was told Discord's sockets", id)
			}
		}
	}
	// What the manifest asks is what is granted: the topics, no network.
	m, err := addons.BundledManifest("discord")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Permissions, []string{cli.PermPlaybackRead}) {
		t.Errorf("permissions = %v", m.Permissions)
	}
}

// A bind mount shows the add-on the socket that was there when it started,
// and a Discord that came up, or restarted, since is another one: so when the
// add-on says it isn't connected and the sockets are no longer the ones it was
// shown, Sona starts it again; it doesn't while they are.
func TestDiscordAddonRestartsWhenDiscordsSocketsChange(t *testing.T) {
	_, listen := runtimeDir(t)
	a := newAddonEnv(t)
	_, started := a.bundledProgram(t)
	rec := a.recordSpecs()
	var mu sync.Mutex
	var conn []bool
	a.svc.discordConnected = func(c bool) { mu.Lock(); conn = append(conn, c); mu.Unlock() }
	a.useAll(t)
	a.svc.start()
	a.waitState(t, "discord", AddonRunning)
	runner := func() *addonRunner {
		a.svc.mu.Lock()
		defer a.svc.mu.Unlock()
		return a.svc.runners["discord"]
	}
	first := runner()
	report := first.connectionReport(a.svc.entryOf("discord", "discord"))

	report(false) // Discord isn't there, and wasn't
	time.Sleep(300 * time.Millisecond)
	if runner() != first || len(rec.of("discord")) != 1 {
		t.Fatal("the add-on was started again with no change of sockets")
	}
	sock := listen("discord-ipc-0") // Discord starts
	report(false)
	waitUntil(t, "the add-on to start again", func() bool { return runner() != first })
	a.waitState(t, "discord", AddonRunning)
	waitUntil(t, "the second start", func() bool { return len(rec.of("discord")) == 2 })
	if got := rec.of("discord")[1].Sockets; !slices.Equal(got, []string{sock}) {
		t.Errorf("the second start was shown %v, want %v", got, []string{sock})
	}

	// Seen now, so a failure to connect to a Discord that is there is no reason.
	second := runner()
	second.connectionReport(a.svc.entryOf("discord", "discord"))(false)
	time.Sleep(300 * time.Millisecond)
	if runner() != second || len(rec.of("discord")) != 2 {
		t.Error("the add-on was started again for a socket it was shown")
	}
	if b, _ := os.ReadFile(started); strings.Count(string(b), "--addon|discord|") != 2 {
		t.Errorf("starts:\n%s", b)
	}

	// What it says reaches Settings → Connections, and it is not connected once it ends.
	second.connectionReport(a.svc.entryOf("discord", "discord"))(true)
	mu.Lock()
	if len(conn) == 0 || !conn[len(conn)-1] {
		t.Errorf("connection reports = %v, want the last true", conn)
	}
	mu.Unlock()
	if _, err := a.settings.Update(func(s *config.Settings) { s.Integrations.DiscordPresence = false }); err != nil {
		t.Fatal(err)
	}
	a.waitState(t, "discord", AddonIdle)
	waitUntil(t, "the add-on's end to be reported", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return !conn[len(conn)-1]
	})
}

// Only the Discord add-on has the connection method; the others, and the
// add-ons of the user's (whose session has no such callback), answer it as
// they would any unknown method.
func TestOnlyTheDiscordAddonReportsAConnection(t *testing.T) {
	a := newAddonEnv(t)
	r := &addonRunner{s: a.svc}
	for _, id := range addons.BundledIDs {
		en := addons.Entry{ID: id, Bundled: true}
		if got := r.connectionReport(en) != nil; got != (id == "discord") {
			t.Errorf("%s: has the method = %v", id, got)
		}
	}
	if r.connectionReport(addons.Entry{ID: "discord"}) != nil {
		t.Error("an add-on of the user's named discord has the method")
	}
}

// Where Discord's presence stays in Sona's process (Platform.DiscordInProcess:
// Windows) the add-on is neither listed nor started.
func TestDiscordAddonIsAbsentWhereItsPresenceStaysInSona(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.e.Platform.DiscordInProcess = true
	rec := a.recordSpecs()
	a.useAll(t)
	a.svc.start()
	a.waitState(t, "lastfm", AddonRunning)
	for _, i := range a.svc.List() {
		if i.ID == "discord" {
			t.Errorf("listed: %+v", i)
		}
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(rec.of("discord")); n != 0 {
		t.Errorf("started %d times", n)
	}
}

// A build with no Discord application ID has nothing to show, whatever the
// saved switch says: the add-on doesn't run.
func TestDiscordAddonNeedsTheApplicationID(t *testing.T) {
	t.Setenv("SONA_DISCORD_CLIENT_ID", "")
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.useAll(t)
	t.Setenv("SONA_DISCORD_CLIENT_ID", "")
	a.svc.start()
	a.waitState(t, "lastfm", AddonRunning)
	if i := a.info(t, "discord"); i.State != AddonIdle {
		t.Errorf("discord = %+v", i)
	}
}
