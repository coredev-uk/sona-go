package settings

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// inOrder reports whether want is a subsequence of got.
func inOrder(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

func stopLabels(stops []access.Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Node.Label
	}
	return out
}

// Every section is readable and reachable by assistive technology: nothing
// is nested inside a leaf, the keyboard walk is not a trap and the index
// comes first, in order, the section after it; the section is a region
// named by its h2, the list is named, and the current section's link says
// so in its description.
func TestA11ySectionsAreOnTheTreeAndOnTheTabWalk(t *testing.T) {
	first := map[string]string{
		secGeneral: "Language", secPlayback: "Audio quality", secAppearance: "Reduce effects", secConnections: "Connect ListenBrainz",
		secShortcuts: "Filter shortcuts", secStorage: "", secDiagnostics: "Copy diagnostics",
	}
	for _, section := range sections {
		t.Run(section, func(t *testing.T) {
			caps := linuxCaps()
			r := newRigCaps(t, 1132, 1400, fakeShortcuts(), &caps)
			*stateOf[storageAPI](r.s, "seam:storage") = &fakeStorage{}
			r.router.Replace(settingsPath + "?section=" + section)
			r.tt.Until("the section", r.tt.Shows(sectionTitle(section)))
			// A section's controls may wait on a service's answer (Connections
			// on the ListenBrainz status): wait for the first control the walk
			// is to reach, not just the title.
			if f := first[section]; f != "" {
				r.tt.Until("the section's first control", func() bool {
					for _, n := range access.Read(t, r.tt.Tester).Nodes {
						if n.Label == f && n.Has(access.Focusable) {
							return true
						}
					}
					return false
				})
			}
			r.tt.Drain()
			tt := r.tt.Tester

			access.NoNestedControls(t, tt)
			tr := access.Read(t, tt)
			if _, ok := tr.Find(ui.RoleGroup, sectionTitle(section)); !ok {
				t.Errorf("no region named %q:\n%s", sectionTitle(section), tr.Dump())
			}
			var h2 []string
			for _, n := range tr.Roles(ui.RoleHeading) {
				if n.Level == 2 {
					h2 = append(h2, n.Label)
				}
			}
			if !slices.Contains(h2, sectionTitle(section)) {
				t.Errorf("level 2 headings %q", h2)
			}
			if _, ok := tr.Find(ui.RoleList, "Settings sections"); !ok {
				t.Error("the section list is not named")
			}
			// Every control that takes the focus has a name.
			for _, n := range tr.Nodes {
				if n.Has(access.Focusable) && n.Label == "" && n.Role != ui.RoleGroup && n.Role != ui.RoleScroll {
					t.Errorf("nameless control %v", n)
				}
			}
			// The current section says so; no other does.
			for _, id := range sections {
				n, ok := tr.Find(ui.RoleLink, sectionTitle(id))
				if !ok {
					t.Fatalf("no link %q", sectionTitle(id))
				}
				if cur := strings.HasPrefix(n.Description, "Current page"); cur != (id == section) {
					t.Errorf("link %q description %q (current %v)", n.Label, n.Description, id == section)
				}
			}

			want := []string{"Find a setting"}
			for _, id := range sections {
				want = append(want, sectionTitle(id))
			}
			stops := stopLabels(access.TabWalk(t, tt, 200))
			if f := first[section]; f != "" {
				want = append(want, f)
			}
			if !inOrder(stops, want) {
				t.Errorf("Tab walk %q, want in order %q", stops, want)
			}
		})
	}
}

// Shortcut recorders are toggle buttons pressed while recording, Reset all
// stays focusable and explains itself when there is nothing to reset, and
// its question closes on Escape.
func TestA11yShortcutRecordersAndResetAll(t *testing.T) {
	r := tallRig(t)
	tt := r.tt.Tester
	access.NoNestedControls(t, tt) // no Tab walk: this rig binds Tab to a shortcut
	tr := access.Read(t, tt)
	rec, ok := tr.Find(ui.RoleToggleButton, "test.free not set")
	if !ok {
		t.Fatalf("no recorder:\n%s", tr.Dump())
	}
	if rec.Has(access.Checked) {
		t.Error("a recorder at rest is pressed")
	}
	if _, ok := tr.Find(ui.RoleHeading, "General"); !ok {
		t.Error("the General group has no heading")
	}
	reset, ok := tr.Find(ui.RoleButton, "Reset all")
	if !ok || reset.Has(access.Disabled) || reset.Description != "No shortcut has been changed." {
		t.Errorf("Reset all with nothing to reset (focusable, explains itself): %+v", reset)
	}

	// Recording is the pressed state; Escape stops it and keeps the focus.
	r.record("test.free", "not set")
	rec, _ = access.Read(t, tt).Find(ui.RoleToggleButton, "test.free Press keys…")
	if !rec.Has(access.Checked) {
		t.Errorf("a recorder recording is not pressed: %+v", rec)
	}
	r.press(0, ui.KeyEscape)
	r.show("test.free not set")
	if !r.tt.Focused("test.free not set") {
		t.Error("the focus left the recorder after Escape")
	}

	// Once a shortcut is changed, Reset all is a plain button again.
	r.record("test.free", "not set")
	r.press(ui.Ctrl, ui.KeyJ)
	r.tt.Until("a binding saved", func() bool { return len(r.saved()) == 1 })
	r.tt.Drain()
	reset, ok = access.Read(t, tt).Find(ui.RoleButton, "Reset all")
	if !ok {
		t.Fatal("no Reset all button after a binding was saved")
	}
	if reset.Description != "" {
		t.Errorf("Reset all still explains itself: %q", reset.Description)
	}

	// Reset all asks, Escape cancels, and the focus returns to the button.
	r.click("Reset all")
	r.show("Reset every shortcut?")
	r.press(0, ui.KeyEscape)
	r.hidden("Reset every shortcut?")
	if len(r.saved()) != 1 {
		t.Errorf("Escape reset the shortcuts: %v", r.saved())
	}
	r.tt.Drain()
	if tr := access.Read(t, tt); tr.Focus == nil || tr.Focus.Label != "Reset all" {
		t.Errorf("the focus did not return to Reset all: %v", tr.Focus)
	}
}

// What the page says that nobody is looking at: the diagnostics verdict
// (aria-live polite) is announced when it changes, and an unencrypted
// sign-in warning (role="alert") when it appears.
func TestA11yAnnouncements(t *testing.T) {
	t.Run("diagnostics verdict", func(t *testing.T) {
		gate := make(chan struct{})
		g := newDiagRig(t, linuxCaps(), engineReport(), func(g *diagRig) { g.sys.gate = gate })
		g.tt.Until("checking", g.tt.Shows("Checking the playback host, database and keyring…"))
		g.tt.Announcements()
		close(gate)
		g.tt.Until("verdict", g.tt.Shows("Everything Sona needs is working"))
		if ann := g.tt.Announcements(); !slices.Contains(ann, "Everything Sona needs is working") {
			t.Errorf("announcements %q", ann)
		}
	})
	t.Run("unencrypted sign-ins", func(t *testing.T) {
		caps := linuxCaps()
		r := newRigCaps(t, 1132, 1400, nil, &caps)
		r.router.Replace(settingsPath + "?section=connections")
		st := services.ScrobbleStatus{Unencrypted: true, LastFM: services.ScrobblerStatus{Available: true, Connected: true}}
		store.Set(r.h.Env.Queries, keyScrobble, st)
		r.tt.Until("the warning", r.tt.Shows("Your scrobbler sign-ins are stored unencrypted"))
		if ann := r.tt.Announcements(); !slices.Contains(ann, "Your scrobbler sign-ins are stored unencrypted") {
			t.Errorf("announcements %q", ann)
		}
		tr := access.Read(t, r.tt.Tester)
		if _, ok := tr.Find(ui.RoleStatus, "Your scrobbler sign-ins are stored unencrypted"); !ok {
			t.Errorf("the warning is not a status:\n%s", tr.Dump())
		}
	})
}
