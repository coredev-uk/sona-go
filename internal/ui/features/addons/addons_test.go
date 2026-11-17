package addons

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// call is one Invoke the fake saw.
type call struct {
	ID, Action string
	Target     *services.AddonTarget
}

// fakeAPI is AddonsService: contributions it is given, and answers it is told
// to give. gate, when set, holds every Invoke until it is closed.
type fakeAPI struct {
	mu    sync.Mutex
	list  []services.AddonContribution
	calls []call
	res   services.AddonResult
	err   error
	gate  chan struct{}
	reads int
	// answers are what AnswerDev was told, "<token>=<yes|no>".
	answers []string
}

func (f *fakeAPI) AnswerDev(token string, run bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = append(f.answers, fmt.Sprintf("%s=%v", token, run))
}

func (f *fakeAPI) answered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.answers)
}

func (f *fakeAPI) Contributions(string) []services.AddonContribution {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.list
}

func (f *fakeAPI) Invoke(ctx context.Context, id, action string, target *services.AddonTarget) (services.AddonResult, error) {
	f.mu.Lock()
	gate, res, err := f.gate, f.res, f.err
	f.calls = append(f.calls, call{id, action, target})
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return services.AddonResult{}, ctx.Err()
		}
	}
	return res, err
}

func (f *fakeAPI) seen() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeAPI) set(res services.AddonResult, err error) {
	f.mu.Lock()
	f.res, f.err = res, err
	f.mu.Unlock()
}

// contribution is a running add-on "Webhook" with an action for songs and
// albums and a command.
func contribution() services.AddonContribution {
	return services.AddonContribution{
		ID: "webhook", Name: "Webhook", Running: true,
		Actions:  []services.AddonAction{{ID: "addon.webhook.send", Label: "Send it", Kinds: []string{"song", "album"}, Icon: "bell"}},
		Commands: []services.AddonCommand{{ID: "addon.webhook.refresh", Label: "Refresh it", Keywords: "post address"}},
	}
}

type rig struct {
	t    *testing.T
	h    *uitest.Harness
	f    *feature
	api  *fakeAPI
	menu *kit.ActionMenus
	tt   *uitest.Session
	now  time.Time
}

// newRig builds the feature over a fake service with one running add-on and
// waits for its contributions to be in.
func newRig(t *testing.T, list ...services.AddonContribution) *rig {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	r := &rig{t: t, h: h, api: &fakeAPI{list: list}, now: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	r.f = &feature{env: h.Env, api: r.api, now: func() time.Time { return r.now }, busy: map[string]bool{}, lastToast: map[string]time.Time{}}
	h.Env.AddonCommands = func() []module.Shortcut { return r.f.commands }
	r.menu = kit.NewActionMenus(h.Env.Actions, nil)
	r.tt = h.Tester(t, func(c *ui.Context) {
		kit.Toaster(c)
		kit.ConfirmHost(c)
		head := ui.Column(c).Fill().Padding(20).AlignItems(ui.Start)
		head.Children(func() {
			r.menu.Trigger(c, head, kit.ActionTarget{Item: kit.AlbumItem(album()), Name: album().Name, Key: "a"}, kit.TriggerOpts{Visible: true})
		})
		r.menu.Popup(c)
	}, 500, 500)
	r.f.refresh()
	r.tt.Until("the contributions", func() bool { return !r.f.loading })
	return r
}

func album() services.Album {
	return services.Album{ID: "l.1", CatalogID: "1700", Name: "Some Album", Artist: "An Artist"}
}

func (r *rig) openMenu() {
	r.t.Helper()
	r.tt.MustClick("More for Some Album")
	r.tt.Drain()
}

func TestAnAlbumActionIsInTheMenuUnderTheAddonsNameAndInvokesTheAddonWithTheItem(t *testing.T) {
	r := newRig(t, contribution())
	r.api.set(services.AddonResult{Toast: "Sent it"}, nil)
	r.openMenu()
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleMenuItem, "Send it · Webhook")
	if !ok || n.Has(access.Disabled) {
		t.Fatalf("menu:\n%s", tr.Dump())
	}
	r.tt.MustClick("Send it · Webhook")
	r.tt.Until("the add-on's answer", r.tt.Shows("Sent it"))
	got := r.api.seen()
	want := call{"webhook", "addon.webhook.send", &services.AddonTarget{Kind: "album", ID: "1700", Name: "Some Album"}}
	if len(got) != 1 || got[0].ID != want.ID || got[0].Action != want.Action || got[0].Target == nil || *got[0].Target != *want.Target {
		t.Fatalf("invoked %+v, want %+v", got, want)
	}
	// The toast is under the add-on's name.
	if !r.tt.HasText("Webhook") {
		t.Errorf("the toast does not name the add-on; texts %q", r.tt.Texts())
	}
	if !announced(r.tt.Announcements(), "Webhook") {
		t.Errorf("announcements %q", r.tt.Announcements())
	}
}

func announced(ann []string, prefix string) bool {
	return slices.ContainsFunc(ann, func(s string) bool { return strings.Contains(s, prefix) })
}

func TestAnActionIsListedOnlyForTheKindsItNames(t *testing.T) {
	c := contribution()
	c.Actions[0].Kinds = []string{"song"}
	r := newRig(t, c)
	if got := len(h(r).Actions.For(kit.KindAlbum)); got != 0 {
		t.Errorf("album actions: %d", got)
	}
	if got := len(h(r).Actions.For(kit.KindSong)); got != 1 {
		t.Errorf("song actions: %d", got)
	}
	for _, k := range []kit.ItemKind{kit.KindArtist, kit.KindPlaylist, kit.KindStation, kit.KindMusicVideo} {
		if got := len(h(r).Actions.For(k)); got != 0 {
			t.Errorf("%s actions: %d", k, got)
		}
	}
}

func h(r *rig) *module.Env { return r.h.Env }

func TestAnAddonThatIsNotRunningContributesNothingAndLeavingTakesItsEntriesAway(t *testing.T) {
	r := newRig(t, contribution())
	if len(h(r).Actions.For(kit.KindAlbum)) != 1 || len(r.f.commands) != 1 {
		t.Fatalf("actions %d, commands %d", len(h(r).Actions.For(kit.KindAlbum)), len(r.f.commands))
	}
	stopped := contribution()
	stopped.Running = false
	r.api.mu.Lock()
	r.api.list = []services.AddonContribution{stopped}
	r.api.mu.Unlock()
	r.f.refresh()
	r.tt.Until("the swap", func() bool { return !r.f.loading })
	if n := len(h(r).Actions.For(kit.KindAlbum)); n != 0 || len(r.f.commands) != 0 {
		t.Errorf("a stopped add-on still has %d actions and %d commands", n, len(r.f.commands))
	}
}

func TestLabelsAreCleanedAndCappedAndEveryEntryNamesItsAddon(t *testing.T) {
	c := contribution()
	c.Name = "We\u202Eb\u0007hook"
	c.Actions[0].Label = "Ev\u202Eil\x1b[31m " + strings.Repeat("x", 300)
	c.Commands[0].Label = "\u200BRefresh"
	c.Commands[0].Keywords = "k\u202Ey"
	r := newRig(t, c)
	for _, def := range h(r).Actions.For(kit.KindAlbum) {
		l := def.Label(&kit.ActionContext{})
		if strings.ContainsAny(l, "\u202E\u200B\x1b\x07") {
			t.Errorf("action label %q keeps a control or bidirectional character", l)
		}
		if len(l) > 60+len(" · ")+60+len("…")*2 {
			t.Errorf("action label is %d bytes", len(l))
		}
		if !strings.HasSuffix(l, " · We b hook") {
			t.Errorf("action label %q does not end with the add-on's name", l)
		}
	}
	cmd := r.f.commands[0]
	if cmd.Label() != "Refresh · We b hook" {
		t.Errorf("command label %q", cmd.Label())
	}
	if strings.ContainsAny(cmd.Keywords(), "\u202E") {
		t.Errorf("keywords %q", cmd.Keywords())
	}
}

func TestACommandIsInThePalettesAddonsGroupHasNoKeysAndInvokesWithNoItem(t *testing.T) {
	r := newRig(t, contribution())
	r.api.set(services.AddonResult{Toast: "Refreshed"}, nil)
	cmds := h(r).AddonCommands()
	if len(cmds) != 1 {
		t.Fatalf("commands %d", len(cmds))
	}
	c := cmds[0]
	if c.Group != module.AddonsGroup || len(c.Keys) != 0 || c.ID != "addon.webhook.refresh" || c.Label() != "Refresh it · Webhook" {
		t.Errorf("command %+v", c)
	}
	c.Run()
	r.tt.Until("the answer", r.tt.Shows("Refreshed"))
	got := r.api.seen()
	if len(got) != 1 || got[0].Action != "addon.webhook.refresh" || got[0].Target != nil {
		t.Errorf("invoked %+v", got)
	}
}

// An invocation never waits on the main thread: Run returns while the add-on
// still holds the call, and the same entry chosen again meanwhile does nothing.
func TestAnInvocationNeverBlocksTheMainThreadAndIsNotRepeatedWhileItRuns(t *testing.T) {
	r := newRig(t, contribution())
	gate := make(chan struct{})
	r.api.mu.Lock()
	r.api.gate = gate
	r.api.mu.Unlock()
	cmd := r.f.commands[0]
	// If Run waited for the add-on, the watchdog frees it and fails the test.
	dog := time.AfterFunc(5*time.Second, func() { t.Error("Run waited for the add-on"); close(gate) })
	cmd.Run()
	cmd.Run()
	dog.Stop()
	r.tt.Until("one call", func() bool { return len(r.api.seen()) == 1 })
	close(gate)
	r.tt.Until("the answer", r.tt.Shows("Done."))
	if n := len(r.api.seen()); n != 1 {
		t.Errorf("the entry chosen twice while it ran invoked %d times", n)
	}
}

func TestWhenTheAddonDoesNotAnswerOrIsGoneTheToastSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"unavailable", services.ErrAddonUnavailable, "It isn't running."},
		{"timeout", context.DeadlineExceeded, "It didn't answer in time."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, contribution())
			r.api.set(services.AddonResult{}, tc.err)
			r.f.commands[0].Run()
			r.tt.Until("the toast", r.tt.Shows(tc.want))
			if !r.tt.HasText("Webhook") {
				t.Errorf("texts %q", r.tt.Texts())
			}
		})
	}
}

func TestAnAnswerIsPlainTextCappedAndAtMostOnePerSecondPerAddon(t *testing.T) {
	r := newRig(t, contribution())
	r.api.set(services.AddonResult{Toast: "One \u202Eand\x1b[2J " + strings.Repeat("y", 2000)}, nil)
	r.f.commands[0].Run()
	r.tt.Until("the first toast", r.tt.Shows("One"))
	for _, s := range r.tt.Texts() {
		if strings.ContainsAny(s, "\u202E\x1b") {
			t.Errorf("toast text %q keeps a control character", s)
		}
		if len(s) > 600 {
			t.Errorf("toast text is %d bytes", len(s))
		}
	}
	// A second answer within a second is dropped; one a second later shows.
	r.api.set(services.AddonResult{Toast: "Second"}, nil)
	r.f.commands[0].Run()
	r.tt.Until("the second call", func() bool { return len(r.api.seen()) == 2 })
	r.tt.Drain()
	r.tt.Frame()
	if r.tt.HasText("Second") {
		t.Error("a second toast came within a second of the first")
	}
	r.now = r.now.Add(1100 * time.Millisecond)
	r.f.commands[0].Run()
	r.tt.Until("the third toast", r.tt.Shows("Second"))
}

// A link is offered as a button and opens only when it is clicked; only http
// and https without credentials are offered at all.
func TestALinkOpensOnlyAfterAClickAndOnlyIfItIsHTTPOrHTTPSWithoutCredentials(t *testing.T) {
	r := newRig(t, contribution())
	r.api.set(services.AddonResult{Toast: "Sent", URL: "https://example.com/played?x=1"}, nil)
	r.f.commands[0].Run()
	r.tt.Until("the link", r.tt.Shows("Open example.com"))
	if got := r.h.Opened(); len(got) != 0 {
		t.Fatalf("the link opened before a click: %q", got)
	}
	r.tt.MustClick("Open example.com")
	r.tt.Drain()
	if got := r.h.Opened(); len(got) != 1 || got[0] != "https://example.com/played?x=1" {
		t.Errorf("opened %q after the click", got)
	}
}

func TestOnlySafeLinksAreOffered(t *testing.T) {
	for _, bad := range []string{
		"file:///etc/passwd", "search-ms:query=x&crumb=location:\\\\evil", "javascript:alert(1)", "ms-msdt:/id", "ftp://example.com/x",
		"data:text/html,x", "https://user@example.com/", "https://user:pw@example.com/", "https://:pw@example.com/", "//example.com/x",
		"https:///nohost", "https://exa mple.com/", "https://example.com/\x00", "https://example.com/\n", " https://example.com/",
		"https://example.com/\u202E", "custom://thing", "mailto:a@b.c", "https://" + strings.Repeat("a", 2100) + ".com/",
		"HTTPS://example.com/%zz",
	} {
		if _, _, ok := safeLink(bad); ok {
			t.Errorf("safeLink offered %q", bad)
		}
	}
	for _, good := range []string{"http://example.com", "https://example.com:8443/a/b?c=d#e", "https://127.0.0.1/", "https://[::1]/"} {
		if _, _, ok := safeLink(good); !ok {
			t.Errorf("safeLink refused %q", good)
		}
	}
}

func TestAnUnsafeLinkInAnAnswerGetsNoButton(t *testing.T) {
	r := newRig(t, contribution())
	r.api.set(services.AddonResult{Toast: "Sent", URL: "file:///etc/passwd"}, nil)
	r.f.commands[0].Run()
	r.tt.Until("the toast", r.tt.Shows("Sent"))
	for _, s := range r.tt.Texts() {
		if strings.HasPrefix(s, "Open ") {
			t.Errorf("a button %q was offered", s)
		}
	}
	// And a link that stopped being safe is never opened, whatever asks.
	r.f.open("file:///etc/passwd")
	r.f.open("https://user:pw@example.com/")
	if got := r.h.Opened(); len(got) != 0 {
		t.Errorf("opened %q", got)
	}
}

func TestAnAddonSwitchedOffIsToldToTheUser(t *testing.T) {
	r := newRig(t, contribution())
	r.f.notice(services.AddonNotice{ID: "webhook", Name: "Webhook", Kind: "crashed"})
	r.tt.Until("the notice", r.tt.Shows("Webhook was switched off"))
}

func TestWhenTheAddonsChangeTheContributionsAreReadAgainOnceAtATime(t *testing.T) {
	r := newRig(t, contribution())
	r.api.mu.Lock()
	before := r.api.reads
	r.api.mu.Unlock()
	r.f.refresh()
	r.f.refresh()
	r.f.refresh()
	r.tt.Until("the reads", func() bool { return !r.f.loading && !r.f.again })
	r.api.mu.Lock()
	n := r.api.reads - before
	r.api.mu.Unlock()
	if n < 1 || n > 2 {
		t.Errorf("three changes in a row caused %d reads, want 1 or 2", n)
	}
}

func devQuestion() services.AddonDevAsk {
	return services.AddonDevAsk{
		Token: "t1", ID: "webhook", Name: "We\u202Eb\u0007hook", Version: "1.0.0", Dir: "/home/me/code/webhook\u202E",
		Permissions: []string{"playback.read", "private.read"},
	}
}

// `sona addons dev` never runs anything on its own: the question is in Sona's
// window, names the folder and the permissions, and only its Run button says yes.
func TestADevRunIsAskedInTheWindowAndOnlyRunSaysYes(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(r *rig)
		want string
	}{
		{"run", func(r *rig) { r.tt.MustClick("Run") }, "t1=true"},
		{"don't run", func(r *rig) { r.tt.MustClick("Don't run") }, "t1=false"},
		{"escape", func(r *rig) { r.tt.Key(0, ui.KeyEscape) }, "t1=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, contribution())
			r.f.devAsk(devQuestion())
			r.tt.Until("the question", r.tt.Shows("Run We b hook?"))
			for _, want := range []string{"in the sandbox from the folder /home/me/code/webhook", "playback.read, private.read", "nothing is saved"} {
				found := slices.ContainsFunc(r.tt.Texts(), func(s string) bool { return strings.Contains(s, want) })
				if !found {
					t.Errorf("the question does not say %q; texts %q", want, r.tt.Texts())
				}
			}
			if got := r.api.answered(); len(got) != 0 {
				t.Fatalf("answered before the person did: %q", got)
			}
			tc.act(r)
			r.tt.Until("the answer", func() bool { return len(r.api.answered()) == 1 })
			if got := r.api.answered(); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("answers %q, want %q", got, tc.want)
			}
		})
	}
}

func TestADevQuestionNeverReplacesOneOfThePersonsOwn(t *testing.T) {
	r := newRig(t, contribution())
	var own []bool
	kit.Confirm(kit.ConfirmOptions{Title: "Delete it?", ConfirmLabel: "Delete"}, func(ok bool) { own = append(own, ok) })
	r.f.devAsk(devQuestion())
	r.tt.Until("the refusal", func() bool { return len(r.api.answered()) == 1 })
	if got := r.api.answered(); !slices.Equal(got, []string{"t1=false"}) {
		t.Errorf("answers %q", got)
	}
	if len(own) != 0 || !kit.ConfirmPending() {
		t.Errorf("the person's own question was settled (%v) or replaced", own)
	}
	kit.ConfirmCancel()
}

func TestADevQuestionThatEndsClosesTheDialogAndAnswersNothing(t *testing.T) {
	r := newRig(t, contribution())
	r.f.devAsk(devQuestion())
	r.tt.Until("the question", r.tt.Shows("Run We b hook?"))
	r.f.devAskEnd("someone else's")
	r.tt.Drain()
	if !kit.ConfirmPending() {
		t.Fatal("another question's end closed this one")
	}
	r.f.devAskEnd("t1")
	r.tt.Drain()
	r.tt.Frame()
	if kit.ConfirmPending() || r.tt.HasText("Run We b hook?") {
		t.Error("the question stayed after it ended")
	}
	if got := r.api.answered(); len(got) != 0 {
		t.Errorf("answers %q", got)
	}
}

// An action or command outside addon.<id>.* is never registered, so an add-on
// can't take the ID of one of Sona's own (the manifest can't say one either,
// but the window does not rely on it).
func TestNothingOutsideTheAddonsOwnIDsIsRegistered(t *testing.T) {
	c := contribution()
	c.Actions = append(c.Actions,
		services.AddonAction{ID: "player.play-next", Label: "Takes Play next", Kinds: []string{"song"}},
		services.AddonAction{ID: "addon.other.send", Label: "Speaks for another", Kinds: []string{"song"}})
	c.Commands = append(c.Commands,
		services.AddonCommand{ID: "player.toggle", Label: "Takes Play or pause"},
		services.AddonCommand{ID: "addon.other.refresh", Label: "Speaks for another"})
	r := newRig(t, c)
	reg := h(r).Actions
	for _, def := range append(reg.For(kit.KindSong), reg.For(kit.KindAlbum)...) {
		if !strings.HasPrefix(def.ID, "addon.webhook.") {
			t.Errorf("registered action %q", def.ID)
		}
	}
	if len(r.f.commands) != 1 || r.f.commands[0].ID != "addon.webhook.refresh" {
		t.Errorf("commands %+v", r.f.commands)
	}
}
