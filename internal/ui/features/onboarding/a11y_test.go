package onboarding

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// walkFrom is the rig's Tab walk, rotated to start at the stop named first
// (the focus starts on the step's title), as the stops' names.
func walkFrom(t *testing.T, r *rig, first string) []string {
	t.Helper()
	r.tt.Drain()
	r.tt.Frame()
	access.NoNestedControls(t, r.tt.Tester)
	var names []string
	for _, s := range access.TabWalk(t, r.tt.Tester, 60) {
		names = append(names, s.Node.Label)
	}
	i := slices.Index(names, first)
	if i < 0 {
		t.Fatalf("the Tab walk %q never reaches %q", names, first)
	}
	return append(slices.Clone(names[i:]), names[:i]...)
}

// inOrder fails unless want is a subsequence of got.
func inOrder(t *testing.T, got, want []string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("Tab walk %q lacks %q in that order", got, want)
	}
}

// TestKeyboardWalk is every step of the setup: Tab reaches its primary
// controls in reading order (the way out, the step's controls, the way on),
// nothing is a trap or a control inside a control.
func TestKeyboardWalk(t *testing.T) {
	tests := []struct {
		name  string
		step  string
		pre   func(*rig)
		marks []string
		want  []string
	}{
		{
			"welcome", "welcome", nil,
			[]string{"Get started"},
			[]string{"Skip setup", "Welcome", "Get started"},
		},
		{
			"account", "account", nil,
			[]string{"Not now", "Sign in"},
			[]string{"Skip setup", "Welcome", "Apple Account", "Password", "Sign in with your browser", "Paste a Music User Token", "Back", "Not now"},
		},
		{
			"playback", "playback", func(r *rig) {
				store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) {
					s.State, s.Version, s.Received, s.Total = "downloading", "", 42, 100
				}))
			},
			[]string{"Audio quality", "Crossfade", "Private listening"},
			[]string{"Skip setup", "Welcome", "High quality", "Sound Check", "Crossfade", "Private listening", "Back", "Continue"},
		},
		{
			"connections", "connections", func(r *rig) {
				r.sc.status = services.ScrobbleStatus{LastFM: services.ScrobblerStatus{Available: true}, ListenBrainz: services.ScrobblerStatus{Available: true}}
				r.in.status = services.IntegrationsStatus{Discord: services.DiscordStatus{Configured: true}}
			},
			[]string{"Last.fm", "Connect Last.fm", "Discord status"},
			[]string{"Skip setup", "Welcome", "Connect Last.fm", "Connect ListenBrainz", "Discord status", "Back", "Continue"},
		},
		{
			"appearance", "appearance", nil,
			[]string{"Theme", "Reduce effects", "Motion artwork"},
			[]string{"Skip setup", "Welcome", "Reduce effects", "Motion artwork", "Back", "Continue"},
		},
		{
			"desktop", "desktop", nil,
			[]string{"When you close the window", "Start Sona when you log in"},
			[]string{"Skip setup", "Welcome", "Keep playing", "Start Sona when you log in", "Track notifications", "Back", "Continue"},
		},
		{
			"done", "done", nil,
			[]string{"Still to do", "Sign in"},
			[]string{"Welcome", "Sign in", "Back", "Done"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, 900, 1300)
			if tc.pre != nil {
				tc.pre(r)
			}
			r.open(tc.step)
			r.tt.Until("the step", r.tt.Shows(tc.marks...))
			r.tt.Frame()
			inOrder(t, walkFrom(t, r, tc.want[0]), tc.want)
		})
	}
}

// TestLayerIsANamedDialogWithTheTitleAsAHeading: the sheet is a dialog named
// and described by its step, whose title is a level-2 heading; the focus
// arrives on the group named by the title around it, which Orca can read (it
// reads a focused heading's role and level, not its words).
func TestLayerIsANamedDialogWithTheTitleAsAHeading(t *testing.T) {
	r := newRig(t, 900, 1000)
	r.open("playback")
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	d, ok := tr.Find(ui.RoleDialog, "Playback")
	if !ok || d.Description != "What plays your music and how it sounds." {
		t.Fatalf("the dialog: %+v\n%s", d, tr.Dump())
	}
	if h, ok := tr.Find(ui.RoleHeading, "Playback"); !ok || h.Level != 2 || !tr.Within(h, ui.RoleDialog) {
		t.Errorf("the title: %+v", h)
	}
	if tr.Focus == nil || tr.Focus.Role != ui.RoleGroup || tr.Focus.Label != "Playback" {
		t.Errorf("the title does not hold the focus: %+v", tr.Focus)
	}
	r.click("Continue")
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Role != ui.RoleGroup || tr.Focus.Label != "Connections" {
		t.Errorf("the next step's title does not take the focus: %+v", tr.Focus)
	}
	for _, st := range access.TabWalk(t, r.tt.Tester, 60) {
		if st.Node.Role == ui.RoleGroup && st.Node.Label == "Connections" {
			t.Errorf("Tab stops at the step title: %+v", st.Node)
		}
	}
}

// TestStepStripIsAListWithTheCurrentStepMarked is StepProgress's nav and ol: a
// named list of one item per step, each holding a button named after the
// step; the current step's is the current step.
func TestStepStripIsAListWithTheCurrentStepMarked(t *testing.T) {
	r := newRig(t, 900, 1000)
	r.open("connections")
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	list, ok := tr.Find(ui.RoleList, "Setup steps")
	if !ok {
		t.Fatalf("no list named Setup steps:\n%s", tr.Dump())
	}
	items := tr.Children(list)
	names := []string{"Welcome", "Apple Music", "Playback", "Connections", "Appearance", "Desktop", "Done"}
	if len(items) != len(names) {
		t.Fatalf("%d items in the strip, want %d", len(items), len(names))
	}
	for i, it := range items {
		if it.Role != ui.RoleListItem {
			t.Errorf("the strip holds a %v, not an item", it.Role)
		}
		kids := tr.Children(it)
		if len(kids) != 1 || kids[0].Role != ui.RoleButton || kids[0].Label != names[i] {
			t.Errorf("item %d holds %v, want the button %q", i, kids, names[i])
			continue
		}
		want := ui.CurrentNone
		if names[i] == "Connections" {
			want = ui.CurrentStep
		}
		if kids[0].Current != want || kids[0].Description != "" {
			t.Errorf("the %q segment is current %v with description %q, want current %v and none", names[i], kids[0].Current, kids[0].Description, want)
		}
	}
	// Going to a step moves the current one.
	r.click("Desktop")
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	for _, n := range tr.Nodes {
		if n.Role == ui.RoleButton && (n.Current == ui.CurrentStep) != (n.Label == "Desktop") && slices.Contains(names, n.Label) {
			t.Errorf("after Desktop, %q is current %v", n.Label, n.Current)
		}
	}
}

// TestSkipConfirmKeepsTheFocusInsideTheSetupDialog: Escape asks in an alert dialog
// whose two buttons are the only Tab stops; Cancel and Escape close it and
// the focus stays in the setup.
func TestSkipConfirmKeepsTheFocusInsideTheSetupDialog(t *testing.T) {
	for _, how := range []string{"Cancel", "Escape"} {
		t.Run(how, func(t *testing.T) {
			r := newRig(t, 900, 1000)
			r.open("playback")
			r.tt.Frame()
			r.key(0, ui.KeyEscape)
			r.tt.Frame()
			tr := access.Read(t, r.tt.Tester)
			if _, ok := tr.Find(ui.RoleAlertDialog, "Skip the rest of the setup?"); !ok {
				t.Fatalf("no alert dialog:\n%s", tr.Dump())
			}
			if tr.Focus == nil || tr.Focus.Label != "Skip setup" || tr.Focus.Role != ui.RoleButton {
				t.Errorf("the confirm's button does not hold the focus: %+v", tr.Focus)
			}
			access.NoNestedControls(t, r.tt.Tester)
			if stops := access.TabWalk(t, r.tt.Tester, 10); len(stops) != 2 {
				t.Errorf("the confirm's Tab stops: %v", stops)
			}
			if how == "Cancel" {
				r.click("Cancel")
			} else {
				r.key(0, ui.KeyEscape)
			}
			r.tt.Advance(300 * time.Millisecond)
			r.tt.Frame()
			if r.tt.HasText("Skip the rest of the setup?") || !r.h.Env.Surfaces.SetupOpen() {
				t.Fatalf("%s did not close the question and keep the setup", how)
			}
			// The focus stays inside the setup (never lost to the page
			// behind). It lands on the first control, not on what had it: the
			// opener MyGo records for a dialog over a modal layer is taken
			// after the dialog's content is built (see the upstream asks).
			tr = access.Read(t, r.tt.Tester)
			if tr.Focus == nil || !tr.Within(*tr.Focus, ui.RoleDialog) {
				t.Errorf("after %s the focus is %+v, outside the setup dialog", how, tr.Focus)
			}
		})
	}
}

// TestWidevineDownloadIsAProgressNode: the bar has a name, a value and a
// text, and the state above it is a status.
func TestWidevineDownloadIsAProgressNode(t *testing.T) {
	r := newRig(t, 900, 1000)
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) {
		s.State, s.Version, s.Received, s.Total = "downloading", "", 42, 100
	}))
	r.open("playback")
	r.tt.Until("the bar", r.tt.Shows("Downloading Widevine…"))
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	p, ok := tr.Find(ui.RoleProgress, "Widevine download")
	if !ok || p.Value != "42%, 42 byte of 100 byte" {
		t.Errorf("the progress node: %+v\n%s", p, tr.Dump())
	}
	if s, ok := tr.Find(ui.RoleStatus, "Downloading Widevine…"); !ok || s.Description == "" {
		t.Errorf("the state: %+v", s)
	}
}

// TestSwitchesAndChoicesCarryTheirState: the playback step's switches say
// whether they are on, and its audio quality is a named radio group with the
// chosen one checked.
func TestSwitchesAndChoicesCarryTheirState(t *testing.T) {
	r := newRig(t, 900, 1300)
	r.open("playback")
	r.tt.Until("the controls", r.tt.Shows("Audio quality", "Sound Check", "Crossfade", "Private listening"))
	r.clickSwitch("Sound Check")
	r.tt.Until("Sound Check saved", func() bool { return r.h.Settings.Get().Audio.SoundCheck })
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleSwitch, "Sound Check"); !ok || !n.Has(access.Checked) || n.Description == "" {
		t.Errorf("Sound Check: %+v", n)
	}
	if n, ok := tr.Find(ui.RoleSwitch, "Crossfade"); !ok || n.Has(access.Checked) {
		t.Errorf("Crossfade: %+v", n)
	}
	if _, ok := tr.Find(ui.RoleRadioGroup, "Audio quality"); !ok {
		t.Errorf("no Audio quality radio group:\n%s", tr.Dump())
	}
	if n, ok := tr.Find(ui.RoleRadio, "High quality"); !ok || !n.Has(access.Checked) {
		t.Errorf("High quality: %+v", n)
	}
	// The crossfade length is a slider with a range and a value.
	r.clickSwitch("Crossfade")
	r.tt.Until("the length", r.tt.Shows("Length", "6 s"))
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	sl := tr.Roles(ui.RoleSlider)
	if len(sl) != 1 || sl[0].Label != "Length" || sl[0].Max <= sl[0].Min || sl[0].Now < sl[0].Min || sl[0].Now > sl[0].Max {
		t.Errorf("the crossfade length slider: %+v\n%s", sl, tr.Dump())
	}
}

// A step that waits for the settings shows a placeholder at about the height
// of what will land: a busy status read as "Loading".
func TestAStepWaitingForTheSettingsIsABusyLoadingStatus(t *testing.T) {
	r := newRig(t, 900, 800)
	r.o.env.Queries.Invalidate(store.KeySettings)
	r.open("desktop")
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleStatus, "Loading")
	if !ok || !n.Has(access.Busy) {
		t.Errorf("no busy Loading status while the settings are read: %+v\n%s", n, tr.Dump())
	}
}
