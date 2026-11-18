package settings

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// fakeAddons is AddonsService: the add-ons and rows it is given, the calls it
// was sent, and a state that follows them.
type fakeAddons struct {
	mu     sync.Mutex
	infos  []services.AddonInfo
	rows   map[string][]services.AddonRow
	log    []string
	calls  []string
	failOn string // a method name whose call fails
}

func (f *fakeAddons) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if strings.HasPrefix(call, f.failOn+" ") || call == f.failOn {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeAddons) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeAddons) List() []services.AddonInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.infos)
}

func (f *fakeAddons) Contributions(string) []services.AddonContribution {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []services.AddonContribution
	for _, a := range f.infos {
		if a.State != services.AddonInvalid {
			out = append(out, services.AddonContribution{ID: a.ID, Name: a.Name, Running: a.State == services.AddonRunning, Rows: slices.Clone(f.rows[a.ID])})
		}
	}
	return out
}

func (f *fakeAddons) update(id string, fn func(*services.AddonInfo)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.infos {
		if f.infos[i].ID == id {
			fn(&f.infos[i])
		}
	}
}

func (f *fakeAddons) Enable(id string, approve []string) error {
	if err := f.record(fmt.Sprintf("enable %s %v", id, approve)); err != nil {
		return err
	}
	f.update(id, func(a *services.AddonInfo) { a.Enabled, a.Granted, a.State = true, approve, services.AddonRunning })
	return nil
}

func (f *fakeAddons) Disable(id string) error {
	if err := f.record("disable " + id); err != nil {
		return err
	}
	f.update(id, func(a *services.AddonInfo) { a.Enabled, a.State = false, services.AddonOff })
	return nil
}

func (f *fakeAddons) Revoke(id, p string) error {
	if err := f.record("revoke " + id + " " + p); err != nil {
		return err
	}
	f.update(id, func(a *services.AddonInfo) {
		a.Granted = slices.DeleteFunc(slices.Clone(a.Granted), func(g string) bool { return g == p })
		a.State = services.AddonNeedsApproval
	})
	return nil
}

func (f *fakeAddons) Log(id string) ([]string, error) {
	if err := f.record("log " + id); err != nil {
		return nil, err
	}
	return f.log, nil
}

func (f *fakeAddons) SetValue(id, row, value string) error {
	if err := f.record(fmt.Sprintf("value %s %s %s", id, row, value)); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows[id] {
		if r.ID == row {
			f.rows[id][i].Value = value
		}
	}
	return nil
}

func (f *fakeAddons) SetSecret(id, row, value string) error {
	if err := f.record(fmt.Sprintf("secret %s %s %s", id, row, value)); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows[id] {
		if r.ID == row {
			f.rows[id][i].Saved = value != ""
		}
	}
	return nil
}

func (f *fakeAddons) OpenFolder() error { return f.record("folder") }

// webhook is a running add-on with every kind of row.
func webhook() services.AddonInfo {
	return services.AddonInfo{
		ID: "webhook", Name: "Now playing webhook", Version: "1.2.0", Description: "Posts the song that plays to a web address.",
		Permissions: []string{"playback.read", "private.read"}, Granted: []string{"playback.read", "private.read"},
		Enabled: true, State: services.AddonRunning,
	}
}

func webhookRows() []services.AddonRow {
	return []services.AddonRow{
		{ID: "url", Kind: "text", Label: "Web address", Description: "Where the song is posted.", Value: "https://example.com/hook"},
		{ID: "post", Kind: "toggle", Label: "Post songs", Value: "true"},
		{ID: "format", Kind: "choice", Label: "Format", Value: "json", Options: []services.AddonOption{{Value: "json", Label: "JSON"}, {Value: "text", Label: "Plain text"}}},
		{ID: "token", Kind: "secret", Label: "Access token", Saved: true},
	}
}

func idleAddon() services.AddonInfo {
	return services.AddonInfo{
		ID: "home-bridge", Name: "Home bridge", Version: "0.3.1", Permissions: []string{"playback.control"}, Granted: []string{},
		State: services.AddonOff,
	}
}

func refusedAddon() services.AddonInfo {
	return services.AddonInfo{ID: "sloppy", Name: "sloppy", State: services.AddonInvalid, Problem: addons.ProblemUnsafe, Permissions: []string{}, Granted: []string{}}
}

// addonsRig is the Add-ons section over a fake service.
func addonsRig(t *testing.T, f *fakeAddons, width, height int, opts ...uitest.Option) *rig {
	t.Helper()
	caps := linuxCaps()
	r := newRigCaps(t, width, height, nil, &caps, opts...)
	r.s.addons = f
	r.router.Replace(settingsPath + "?section=addons")
	r.tt.Until("the section", r.tt.Shows("Open add-ons folder"))
	// The list comes from a goroutine: wait for it as well.
	if infos := f.List(); len(infos) > 0 {
		r.tt.Until("the add-ons", r.tt.Shows(addonName(infos[0])))
	}
	return r
}

// openAddon opens an add-on's row and waits for its details (and its
// settings rows, which come from a goroutine of their own).
func openAddon(r *rig, f *fakeAddons, id, name string) {
	r.t.Helper()
	r.tt.MustClick(name)
	r.tt.Until("the details of "+name, r.tt.Shows("Permissions"))
	if rows := f.rows[id]; len(rows) > 0 {
		r.tt.Until("the rows of "+name, r.tt.Shows(rows[0].Label))
	}
}

// hasPrefix reports a text on screen that starts with prefix.
func hasPrefix(r *rig, prefix string) bool {
	return slices.ContainsFunc(r.tt.Texts(), func(s string) bool { return strings.HasPrefix(s, prefix) })
}

func standardAddons() *fakeAddons {
	return &fakeAddons{
		infos: []services.AddonInfo{idleAddon(), refusedAddon(), webhook()},
		rows:  map[string][]services.AddonRow{"webhook": webhookRows()},
		log:   []string{"12:00:01 started", "12:00:02 posted 200"},
	}
}

// ownAddons are Sona's own three as a person with Last.fm connected sees
// them where the sandbox can't be set up.
func ownAddons() []services.AddonInfo {
	return []services.AddonInfo{
		{
			ID: "lastfm", Name: "Last.fm", Bundled: true, Enabled: true, State: services.AddonRunning, Unsandboxed: addons.SandboxNoUserNamespace,
			Permissions: []string{"playback.read", "network"}, Granted: []string{"playback.read", "network"},
		},
		{
			ID: "listenbrainz", Name: "ListenBrainz", Bundled: true, Enabled: true, State: services.AddonOff,
			Permissions: []string{"playback.read", "network"}, Granted: []string{"playback.read", "network"},
		},
		{
			ID: "discord", Name: "Discord", Version: "1.0.0", Bundled: true, Enabled: true, State: services.AddonIdle,
			Permissions: []string{"playback.read"}, Granted: []string{"playback.read"},
		},
	}
}

// Each add-on is one row that says where it is in words: its state, how it
// runs and, when it needs the person, why. The details open under it.
func TestAddonsAreARosterOfRowsThatSayTheirState(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1400)
	r.tt.Until("the rows", r.tt.Shows("Now playing webhook", "Home bridge", "sloppy"))
	for _, want := range []string{
		"Running · Sandboxed", "Off · Sandboxed", "Won't run",
		"The folder or the program isn't yours alone: it belongs to someone else or others may change it.",
	} {
		if !r.tt.HasText(want) {
			t.Errorf("the section does not show %q; texts %q", want, r.tt.Texts())
		}
	}
	// Closed, a row shows none of its details.
	for _, hidden := range []string{"Playback state", "Web address", "Version 1.2.0 · Posts the song that plays to a web address."} {
		if r.tt.HasText(hidden) {
			t.Errorf("%q shows before its row is opened", hidden)
		}
	}
	tr := access.Read(t, r.tt.Tester)
	for _, name := range []string{"Now playing webhook", "Home bridge", "sloppy"} {
		if _, ok := tr.Find(ui.RoleDisclosure, name); !ok {
			t.Errorf("no disclosure %q:\n%s", name, tr.Dump())
		}
	}
	if _, ok := tr.Find(ui.RoleSwitch, "Turn on sloppy"); ok {
		t.Error("a refused add-on has a switch")
	}
	if got := r.s.addonsSummary(); got != "3 installed · 1 needs you" {
		t.Errorf("summary %q", got)
	}

	openAddon(r, f, "webhook", "Now playing webhook")
	for _, want := range []string{
		"Version 1.2.0 · Posts the song that plays to a web address.", "Playback state", "Private listening, read only",
		"Web address", "Post songs", "Format", "Access token",
	} {
		if !r.tt.HasText(want) {
			t.Errorf("the opened row does not show %q", want)
		}
	}
	// One open at a time: opening another closes it.
	r.tt.MustClick("Home bridge")
	r.tt.Drain()
	if r.tt.HasText("Web address") {
		t.Error("two add-ons are open at once")
	}
}

// Sona's own three come first, under a heading of their own, as status only:
// no switch, no approval, nothing to revoke, and a way to Connections, where
// their accounts are.
func TestSonasOwnAddonsAreStatusOnlyWithAWayToConnections(t *testing.T) {
	f := standardAddons()
	f.infos = append(ownAddons(), f.infos...)
	r := addonsRig(t, f, 1132, 1600)
	r.tt.Until("the own group", r.tt.Shows("Sona's Own", "Last.fm", "ListenBrainz", "Discord", "Installed"))
	if !hasPrefix(r, "Running without isolation: this system doesn't let Sona create the isolated space") {
		t.Errorf("Last.fm's reason is missing; texts %q", r.tt.Texts())
	}
	for _, want := range []string{"Not in use", "On"} {
		if !r.tt.HasText(want) {
			t.Errorf("no %q", want)
		}
	}
	tr := access.Read(t, r.tt.Tester)
	for _, name := range []string{"Last.fm", "ListenBrainz", "Discord"} {
		if _, ok := tr.Find(ui.RoleSwitch, "Turn on "+name); ok {
			t.Errorf("%s has a switch of its own here", name)
		}
	}
	if got := r.s.addonsSummary(); got != "3 installed · 1 needs you" {
		t.Errorf("Sona's own are counted as installed: %q", got)
	}
	openAddon(r, f, "discord", "Discord")
	if r.tt.HasText("Revoke") || hasPrefix(r, "Version 1.0.0") {
		t.Error("one of Sona's own shows a version or a Revoke")
	}
	r.tt.MustClick("Manage Discord in Connections")
	r.tt.Drain()
	if got := r.router.Location(); got != "/settings?section=connections" {
		t.Errorf("location %q", got)
	}
}

// Where the sandbox can't be set up the add-on does not run, and its row says
// why in plain words: the reason of each cause, none of them a way to run it.
func TestAnAddonWithoutASandboxSaysWhyItDoesNotRun(t *testing.T) {
	for code, want := range map[string]string{
		addons.SandboxUserNamespaceRestricted: "this computer's security settings (AppArmor)",
		addons.SandboxNoUserNamespace:         "doesn't let Sona create the isolated space",
		addons.SandboxNoSeccomp:               "system-call filter",
		addons.SandboxNoSeatbelt:              "macOS's sandbox tool",
		addons.SandboxNoAppContainer:          "AppContainer",
		addons.SandboxUnsupported:             "has no sandbox for add-ons",
		addons.SandboxSetup:                   "couldn't set up its sandbox",
	} {
		a := webhook()
		a.State, a.Sandbox = services.AddonNoSandbox, code
		f := &fakeAddons{infos: []services.AddonInfo{a}, rows: map[string][]services.AddonRow{}}
		r := addonsRig(t, f, 1132, 1000)
		r.tt.Until("the reason "+code, func() bool {
			return slices.ContainsFunc(r.tt.Texts(), func(s string) bool {
				return strings.Contains(s, "It doesn't run:") && strings.Contains(s, want) && strings.Contains(s, "never runs an add-on without")
			})
		})
		if !r.tt.HasText("Can't run · Sandboxed") || hasPrefix(r, "Running") {
			t.Errorf("%s: the row's state; texts %q", code, r.tt.Texts())
		}
	}
}

// One of Sona's own add-ons that runs without the sandbox shows as running,
// with the reason under it; every reason has words of its own.
func TestAnAddonRunningWithoutIsolationSaysWhy(t *testing.T) {
	f := &fakeAddons{infos: ownAddons()[:1], rows: map[string][]services.AddonRow{}}
	r := addonsRig(t, f, 1132, 1000)
	r.tt.Until("the line", func() bool {
		return hasPrefix(r, "Running without isolation:") && r.tt.HasText("Running")
	})
	for _, c := range []string{
		addons.SandboxUserNamespaceRestricted, addons.SandboxNoUserNamespace, addons.SandboxNoSeccomp, addons.SandboxNoSeatbelt,
		addons.SandboxNoAppContainer, addons.SandboxUnsupported,
	} {
		if unisolatedText(c) == unisolatedText("zzz") {
			t.Errorf("reason %q has no words of its own", c)
		}
	}
}

func TestOpenAddonsFolderOpensIt(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1400)
	r.tt.MustClick("Open add-ons folder")
	r.tt.Until("the call", func() bool { return slices.Contains(f.seen(), "folder") })
}

func TestTurningAnAddonOnShowsItsPermissionsFirstAndApprovesExactlyThose(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1400)
	r.tt.MustClick("Turn on Home bridge")
	r.tt.Drain()
	if !r.tt.HasText("Turn on Home bridge?") || !r.tt.HasText("Playback controls") || !r.tt.HasText("Can play, pause, skip, go back and seek, play an Apple Music link, and change shuffle, repeat and crossfade.") {
		t.Fatalf("no approval panel; texts %q", r.tt.Texts())
	}
	if !announced(r.tt.Announcements(), "Turn on Home bridge?") {
		t.Errorf("announcements %q", r.tt.Announcements())
	}
	if got := f.seen(); len(got) != 0 {
		t.Fatalf("the add-on was enabled before the approval: %q", got)
	}
	if err := r.tt.Click("Turn on"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the enable", func() bool { return slices.Contains(f.seen(), "enable home-bridge [playback.control]") })
	r.tt.Until("it runs", func() bool { return !r.tt.HasText("Off · Sandboxed") })
}

func announced(ann []string, prefix string) bool {
	return slices.ContainsFunc(ann, func(s string) bool { return strings.HasPrefix(s, prefix) })
}

func TestCancellingTheApprovalOrEscapeEnablesNothing(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1400)
	r.tt.MustClick("Turn on Home bridge")
	r.tt.Drain()
	r.tt.MustClick("Cancel")
	r.tt.Drain()
	if r.tt.HasText("Turn on Home bridge?") {
		t.Error("the panel stayed after Cancel")
	}
	r.tt.MustClick("Turn on Home bridge")
	r.tt.Drain()
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	if r.tt.HasText("Turn on Home bridge?") {
		t.Error("the panel stayed after Escape")
	}
	if got := f.seen(); len(got) != 0 {
		t.Errorf("calls %q", got)
	}
}

func TestTurningAnAddonOffStopsItAtOnce(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1400)
	r.tt.MustClick("Turn on Now playing webhook") // the switch is on: a click turns it off
	r.tt.Until("the disable", func() bool { return slices.Contains(f.seen(), "disable webhook") })
	r.tt.Until("it is off", func() bool { return !hasPrefix(r, "Running") })
}

func TestRevokingAPermissionIsOneClickAndSaysWhatIsLeft(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1600)
	openAddon(r, f, "webhook", "Now playing webhook")
	r.tt.MustClick("Revoke Playback state · Now playing webhook")
	r.tt.Until("the revoke", func() bool { return slices.Contains(f.seen(), "revoke webhook playback.read") })
	r.tt.Until("the permission is not approved", r.tt.Shows("Not approved"))
	r.tt.Until("it waits for approval", r.tt.Shows("Needs your approval · Sandboxed", "It asks for a permission you haven't approved. It doesn't run until you do."))
}

func TestSettingsRowsSaveAndASecretIsNeverShown(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1600)
	openAddon(r, f, "webhook", "Now playing webhook")

	// A switch row saves at once.
	r.tt.MustClick("Post songs")
	r.tt.Until("the toggle", func() bool { return slices.Contains(f.seen(), "value webhook post false") })

	// A text row saves with Save, not on each key.
	r.tt.MustClick("Web address")
	r.tt.Key(ui.Ctrl, ui.KeyA)
	r.tt.Type("https://example.org/x")
	r.tt.Drain()
	if slices.ContainsFunc(f.seen(), func(c string) bool { return strings.HasPrefix(c, "value webhook url") }) {
		t.Fatal("a key saved the field")
	}
	r.tt.MustClick("Save Web address · Now playing webhook")
	r.tt.Until("the text", func() bool { return slices.Contains(f.seen(), "value webhook url https://example.org/x") })

	// A secret row says one is saved and shows no value; saving sends it to
	// the secret call, never the value call.
	if r.tt.HasText("hunter2") {
		t.Fatal("a secret shows")
	}
	r.tt.Until("the note", r.tt.Shows("Saved in your system keyring. It isn't shown again."))
	r.tt.MustClick("Access token")
	r.tt.Type("hunter2")
	r.tt.Drain()
	r.tt.MustClick("Save Access token · Now playing webhook")
	r.tt.Until("the secret", func() bool { return slices.Contains(f.seen(), "secret webhook token hunter2") })
	for _, c := range f.seen() {
		if strings.HasPrefix(c, "value webhook token") {
			t.Errorf("a secret went through SetValue: %q", c)
		}
	}
	r.tt.Drain()
	if r.tt.HasText("hunter2") {
		t.Error("the secret shows after it was saved")
	}
	r.tt.MustClick("Remove Access token · Now playing webhook")
	r.tt.Until("the removal", func() bool { return slices.Contains(f.seen(), "secret webhook token ") })
}

func TestAFailedCallIsASentenceUnderTheAddon(t *testing.T) {
	f := standardAddons()
	f.failOn = "revoke"
	r := addonsRig(t, f, 1132, 1600)
	openAddon(r, f, "webhook", "Now playing webhook")
	r.tt.MustClick("Revoke Playback state · Now playing webhook")
	r.tt.Until("the sentence", func() bool { return hasPrefix(r, "Sona couldn't do that") })
	// The error is an alert for assistive technology, and the add-on is as it was.
	tr := access.Read(t, r.tt.Tester)
	if n := tr.Roles(ui.RoleAlert); len(n) == 0 {
		t.Errorf("no alert:\n%s", tr.Dump())
	}
}

func TestTheLogIsReadWhenAskedForAndShownAsPlainLines(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1600)
	openAddon(r, f, "webhook", "Now playing webhook")
	if slices.ContainsFunc(f.seen(), func(c string) bool { return strings.HasPrefix(c, "log ") }) {
		t.Fatal("a log was read before it was asked for")
	}
	r.tt.MustClick("Show log · Now playing webhook")
	r.tt.Until("the lines", r.tt.Shows("12:00:01 started", "12:00:02 posted 200"))
	if !slices.Contains(f.seen(), "log webhook") {
		t.Errorf("calls %q", f.seen())
	}
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleLog, "Log"); !ok {
		t.Errorf("no log region:\n%s", tr.Dump())
	}
	r.tt.MustClick("Hide log · Now playing webhook")
	r.tt.Drain()
	if r.tt.HasText("12:00:01 started") {
		t.Error("the log stayed after Hide log")
	}
}

func TestAnEmptyFolderSaysHowToInstallAndAnUnsupportedPlatformSaysItCannot(t *testing.T) {
	r := addonsRig(t, &fakeAddons{}, 1132, 1000)
	r.tt.Until("the empty block", r.tt.Shows("No add-ons installed", "Put an add-on's folder in the add-ons folder, then look again."))
	if got := r.s.addonsSummary(); got != "None installed" {
		t.Errorf("summary %q", got)
	}

	caps := linuxCaps()
	caps.Addons = false
	u := newRigCaps(t, 1132, 1000, nil, &caps)
	u.s.addons = standardAddons()
	u.router.Replace(settingsPath + "?section=addons")
	u.tt.Until("the reason", u.tt.Shows("Not available on Linux yet."))
	if u.tt.HasText("Now playing webhook") {
		t.Error("add-ons are listed where none can run")
	}
}

func TestEveryPermissionAndProblemHasWords(t *testing.T) {
	for _, p := range []string{"playback.read", "playback.control", "queue.read", "lyrics.read", "search", "open", "volume", "like", "private.read", "network"} {
		name, allows := permissionText(p)
		if name == p || allows == "" {
			t.Errorf("permission %q has no words: %q %q", p, name, allows)
		}
	}
	for _, c := range []string{
		addons.SandboxUserNamespaceRestricted, addons.SandboxNoUserNamespace, addons.SandboxNoSeccomp, addons.SandboxNoSeatbelt,
		addons.SandboxNoAppContainer, addons.SandboxUnsupported,
	} {
		if sandboxText(c) == sandboxText("zzz") {
			t.Errorf("sandbox reason %q has no words of its own", c)
		}
	}
	for _, c := range []string{
		addons.ProblemUnreadable, addons.ProblemTooLarge, addons.ProblemManifest, addons.ProblemID, addons.ProblemReservedID,
		addons.ProblemIDMismatch, addons.ProblemName, addons.ProblemVersion, addons.ProblemCommand, addons.ProblemPermission,
		addons.ProblemNoExecutable, addons.ProblemEscapes, addons.ProblemUnsafe, addons.ProblemTooMany, addons.ProblemContribution,
		addons.ProblemStrings,
	} {
		if addonProblemText(c) == addonProblemText("zzz") {
			t.Errorf("problem %q has no words of its own", c)
		}
	}
}

// Nothing in the section is nameless or nested, every control that is
// pressed names its add-on, and the Tab walk goes row by row: a row's
// disclosure, its switch, then, while it is open, its details.
func TestA11yAddonsSection(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 1600)
	openAddon(r, f, "webhook", "Now playing webhook")
	tt := r.tt.Tester
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	for _, n := range tr.Nodes {
		if n.Has(access.Focusable) && n.Label == "" && n.Role != ui.RoleGroup && n.Role != ui.RoleScroll {
			t.Errorf("nameless control %v", n)
		}
	}
	for _, want := range []struct {
		role  ui.Role
		label string
	}{
		{ui.RoleDisclosure, "Now playing webhook"},
		{ui.RoleSwitch, "Turn on Now playing webhook"},
		{ui.RoleSwitch, "Turn on Home bridge"},
		{ui.RoleButton, "Revoke Playback state · Now playing webhook"},
		{ui.RoleButton, "Show log · Now playing webhook"},
		{ui.RoleSwitch, "Post songs"},
		{ui.RoleButton, "Save Web address · Now playing webhook"},
		{ui.RoleButton, "Remove Access token · Now playing webhook"},
	} {
		if _, ok := tr.Find(want.role, want.label); !ok {
			t.Errorf("no %v %q:\n%s", want.role, want.label, tr.Dump())
		}
	}
	// The walk starts at the disclosure just pressed and wraps round.
	stops := stopLabels(access.TabWalk(t, tt, 200))
	if !inOrder(stops, []string{
		"Turn on Now playing webhook", "Revoke Playback state · Now playing webhook", "Web address", "Show log · Now playing webhook",
		"Open add-ons folder", "Home bridge", "Turn on Home bridge", "sloppy", "Now playing webhook",
	}) {
		t.Errorf("Tab walk %q", stops)
	}
}

func TestAddonsGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts []uitest.Option
	}{
		{"settings-addons-light", nil},
		{"settings-addons-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := standardAddons()
			f.infos = append(ownAddons(), f.infos...)
			r := addonsRig(t, f, 1132, 1420, tc.opts...)
			openAddon(r, f, "webhook", "Now playing webhook")
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
	t.Run("settings-addons-approval-light", func(t *testing.T) {
		f := standardAddons()
		r := addonsRig(t, f, 1132, 760)
		r.tt.MustClick("Turn on Home bridge")
		r.tt.Until("the panel", r.tt.Shows("Turn on Home bridge?"))
		r.tt.Settle()
		uitest.Golden(t, r.tt.Image(), "settings-addons-approval-light")
	})
}

// An add-on's rows live under its own heading in Settings → Add-ons and
// nowhere else: not on Connections (where the sign-ins are), nor any other section.
func TestAddonRowsAreOnlyInTheAddonsSection(t *testing.T) {
	for _, section := range sections {
		if section == secAddons {
			continue
		}
		t.Run(section, func(t *testing.T) {
			f := standardAddons()
			caps := linuxCaps()
			r := newRigCaps(t, 1132, 1600, fakeShortcuts(), &caps)
			*stateOf[storageAPI](r.s, "seam:storage") = &fakeStorage{}
			r.s.addons = f
			r.router.Replace(settingsPath + "?section=" + section)
			r.tt.Until("the section", r.tt.Shows(sectionTitle(section)))
			r.tt.Drain()
			for _, add := range []string{"Web address", "Access token", "Post songs", "Now playing webhook"} {
				if r.tt.HasText(add) {
					t.Errorf("%s shows %q", section, add)
				}
			}
		})
	}
}
