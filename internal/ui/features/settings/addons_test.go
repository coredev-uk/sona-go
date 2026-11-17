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
	// The list and the rows come from goroutines: wait for them as well.
	if infos := f.List(); len(infos) > 0 {
		r.tt.Until("the add-ons", r.tt.Shows(infos[0].Name))
		for id, rows := range f.rows {
			if len(rows) > 0 {
				r.tt.Until("the rows of "+id, r.tt.Shows(rows[0].Label))
			}
		}
	}
	return r
}

func standardAddons() *fakeAddons {
	return &fakeAddons{
		infos: []services.AddonInfo{idleAddon(), refusedAddon(), webhook()},
		rows:  map[string][]services.AddonRow{"webhook": webhookRows()},
		log:   []string{"12:00:01 started", "12:00:02 posted 200"},
	}
}

func TestAddonsSectionListsEachAddonUnderItsOwnHeadingWithItsState(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 2600)
	r.tt.Until("the add-ons", r.tt.Shows("Now playing webhook", "Home bridge"))
	for _, want := range []string{
		"Version 1.2.0 · Posts the song that plays to a web address.", "Running.", "Off. It isn't running.",
		"Playback state", "Private listening, read only", "Web address", "Post songs", "Format", "Access token",
		"Sona won't run this add-on", "The folder or the program isn't yours alone: it belongs to someone else or others may change it.",
	} {
		if !r.tt.HasText(want) {
			t.Errorf("the section does not show %q; texts %q", want, r.tt.Texts())
		}
	}
	tr := access.Read(t, r.tt.Tester)
	for _, name := range []string{"Now playing webhook", "Home bridge", "sloppy"} {
		if _, ok := tr.Find(ui.RoleHeading, name); !ok {
			t.Errorf("no heading %q:\n%s", name, tr.Dump())
		}
	}
	// A refused add-on has nothing to turn on.
	if _, ok := tr.Find(ui.RoleSwitch, "Turn on sloppy"); ok {
		t.Error("a refused add-on has a switch")
	}
	if got := r.s.addonsSummary(); got != "3 add-ons · 1 on" {
		t.Errorf("summary %q", got)
	}
}

func TestTheSectionSaysPermissionsAreAdvisoryAndAnAddonRunsWithYourRights(t *testing.T) {
	r := addonsRig(t, standardAddons(), 1132, 1400)
	if !r.tt.HasText("An add-on is a program that runs with your rights. Permissions limit what Sona gives it, not what the program can do: it can read your files and use the network. Turn on only add-ons you trust.") {
		t.Errorf("texts %q", r.tt.Texts())
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
	r := addonsRig(t, f, 1132, 2600)
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
	r.tt.Until("it runs", r.tt.Shows("Running."))
}

// Sona's own add-ons (Last.fm, ListenBrainz, Discord) are part of the signed
// release: turning one on approves nothing, there is no permission to revoke
// and no version of their own to show.
func TestSonasOwnAddonsNeedNoApprovalAndHaveNothingToRevoke(t *testing.T) {
	f := &fakeAddons{infos: []services.AddonInfo{{
		ID: "discord", Name: "Discord", Version: "1.0.0", Description: "Shows the song you play on your Discord profile.",
		Permissions: []string{"playback.read"}, Granted: []string{"playback.read"},
		Bundled: true, Enabled: false, State: services.AddonOff,
	}}}
	r := addonsRig(t, f, 1132, 2000)
	if r.tt.HasText("Version 1.0.0") {
		t.Error("a bundled add-on shows a version of its own")
	}
	if r.tt.HasText("Revoke Playback state · Discord") {
		t.Error("a bundled add-on's permission can be revoked")
	}
	r.tt.MustClick("Turn on Discord")
	r.tt.Until("the enable", func() bool { return slices.Contains(f.seen(), "enable discord [playback.read]") })
	if r.tt.HasText("Turn on Discord?") {
		t.Error("a bundled add-on asked for approval")
	}
	r.tt.Until("it runs", r.tt.Shows("Running."))
}

func announced(ann []string, prefix string) bool {
	return slices.ContainsFunc(ann, func(s string) bool { return strings.HasPrefix(s, prefix) })
}

func TestCancellingTheApprovalOrEscapeEnablesNothing(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 2600)
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
	r := addonsRig(t, f, 1132, 2600)
	r.tt.MustClick("Turn on Now playing webhook") // the switch is on: a click turns it off
	r.tt.Until("the disable", func() bool { return slices.Contains(f.seen(), "disable webhook") })
	r.tt.Until("it is off", func() bool { return r.s.addonsSummary() == "3 add-ons · 0 on" })
}

func TestRevokingAPermissionIsOneClickAndSaysWhatIsLeft(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 2600)
	r.tt.MustClick("Revoke Playback state · Now playing webhook")
	r.tt.Until("the revoke", func() bool { return slices.Contains(f.seen(), "revoke webhook playback.read") })
	r.tt.Until("the permission is not approved", r.tt.Shows("Not approved"))
	r.tt.Until("it waits for approval", r.tt.Shows("Waiting for you: it asks for a permission you haven't approved. It doesn't run until you do."))
}

func TestSettingsRowsSaveAndASecretIsNeverShown(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 2600)
	r.tt.Until("the rows", r.tt.Shows("Post songs"))

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
	r := addonsRig(t, f, 1132, 2600)
	r.tt.MustClick("Revoke Playback state · Now playing webhook")
	r.tt.Until("the sentence", func() bool {
		for _, s := range r.tt.Texts() {
			if strings.HasPrefix(s, "Sona couldn't do that") {
				return true
			}
		}
		return false
	})
	// The error is an alert for assistive technology, and the add-on is as it was.
	tr := access.Read(t, r.tt.Tester)
	if n := tr.Roles(ui.RoleAlert); len(n) == 0 {
		t.Errorf("no alert:\n%s", tr.Dump())
	}
}

func TestTheLogIsReadWhenAskedForAndShownAsPlainLines(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 2600)
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
	r := addonsRig(t, &fakeAddons{}, 1132, 1400)
	r.tt.Until("the empty line", r.tt.Shows("No add-ons are installed. Open the folder, put an add-on's folder in it and it appears here."))
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
// pressed names its add-on, and the Tab walk reaches the switch, the
// permissions, the rows and the log in the order they are drawn.
func TestA11yAddonsSection(t *testing.T) {
	f := standardAddons()
	r := addonsRig(t, f, 1132, 2600)
	r.tt.Until("the rows", r.tt.Shows("Post songs"))
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
	stops := stopLabels(access.TabWalk(t, tt, 200))
	if !inOrder(stops, []string{"Open add-ons folder", "Turn on Home bridge", "Turn on Now playing webhook", "Revoke Playback state · Now playing webhook", "Show log · Now playing webhook"}) {
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
			r := addonsRig(t, f, 1132, 1230, tc.opts...)
			r.tt.Until("the add-ons", r.tt.Shows("Post songs", "Home bridge"))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
	t.Run("settings-addons-approval-light", func(t *testing.T) {
		f := standardAddons()
		r := addonsRig(t, f, 1132, 760)
		r.tt.Until("the add-ons", r.tt.Shows("Home bridge"))
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
