package settings

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestChoosingASectionReplacesTheLocationAndTheLastOneReopens(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.tt.Until("the page", r.tt.Shows("Settings", "Appearance"))
	if err := r.tt.Click("Appearance"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.router.Location(); got != "/settings?section=appearance" {
		t.Fatalf("location after choosing a section: %q", got)
	}
	// Replace: Back leaves Settings, it does not step through sections.
	if r.router.CanGoBack() {
		t.Error("choosing a section pushed a history entry")
	}
	// /settings without a section opens the one shown last.
	r.router.Push(settingsPath)
	r.tt.Drain()
	if r.s.last != "appearance" {
		t.Errorf("a bare /settings opened %q, want appearance", r.s.last)
	}
	// An unknown section is ignored the same way.
	r.router.Push("/settings?section=nope")
	r.tt.Drain()
	if r.s.last != "appearance" {
		t.Errorf("an unknown section opened %q", r.s.last)
	}
}

func TestTheRetiredAccountSectionOpensTheAccountDialogAndRedirects(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.router.Replace("/settings?section=account")
	r.tt.Drain()
	r.tt.Drain()
	if !r.h.Env.Surfaces.AccountOpen() {
		t.Error("the account dialog did not open")
	}
	if got := r.router.Location(); got != "/settings" {
		t.Errorf("location %q, want /settings", got)
	}
}

func TestSectionListShowsEachSectionsStateInOneLine(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.tt.Until("summaries", r.tt.Shows("Opens on Albums", "Default keys", "Light theme", "Nothing connected", "Checks, logs and bug reports"))
}

func TestBelow960TheSummariesAreForAssistiveTechnologyOnly(t *testing.T) {
	r := newRig(t, 900, 800, nil)
	r.tt.Until("the page", r.tt.Shows("Settings", "General"))
	r.tt.Drain()
	if r.tt.HasText("Nothing connected") {
		t.Error("a summary is drawn on a narrow window")
	}
}

func TestFindListsWhatAQueryFindsInPlaceOfTheSections(t *testing.T) {
	r := newRig(t, 1132, 900, fakeShortcuts())
	r.tt.Until("the page", r.tt.Shows("Settings", "Storage"))
	if err := r.tt.Click("Find a setting"); err != nil {
		t.Fatal(err)
	}
	r.tt.Type("dark")
	r.tt.Drain()
	if !r.tt.HasText("Theme") || !r.tt.HasText("Appearance") {
		t.Fatalf("results missing; texts %q", r.tt.Texts())
	}
	if r.tt.HasText("Storage") {
		t.Error("the section list is still shown while querying")
	}
	if !contains(r.tt.Announcements(), "1 setting found") {
		t.Errorf("announcements %q", r.tt.Announcements())
	}
	// No match: said in words and announced.
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Type("zzz")
	r.tt.Drain()
	if !r.tt.HasText("No setting matches “zzz”.") {
		t.Errorf("texts %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	if !r.tt.HasText("Storage") {
		t.Error("clearing the query did not bring the sections back")
	}
}

func TestFindArrowsMoveIntoTheResultsAndBack(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.tt.Until("the page", r.tt.Shows("Settings"))
	if err := r.tt.Click("Find a setting"); err != nil {
		t.Fatal(err)
	}
	r.tt.Type("cache")
	r.tt.Drain()
	r.tt.Key(0, ui.KeyDown)
	r.tt.Drain()
	r.tt.Drain()
	if !r.tt.Focused("Artwork cache") {
		t.Fatalf("Down in the field did not focus the first result")
	}
	r.tt.Key(0, ui.KeyDown)
	r.tt.Drain()
	r.tt.Drain()
	if !r.tt.Focused("Catalogue cache") {
		t.Error("Down in the results did not move to the next")
	}
	r.tt.Key(0, ui.KeyUp)
	r.tt.Drain()
	r.tt.Drain()
	r.tt.Key(0, ui.KeyUp)
	r.tt.Drain()
	r.tt.Drain()
	if !r.tt.Focused("Find a setting") {
		t.Error("Up from the first result did not return to the field")
	}
}

func TestPickingAResultOpensItsSectionAndLandsOnTheRow(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.tt.Until("the page", r.tt.Shows("Settings"))
	if err := r.tt.Click("Find a setting"); err != nil {
		t.Fatal(err)
	}
	r.tt.Type("theme")
	r.tt.Drain()
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	r.tt.Drain()
	if got := r.router.Location(); got != "/settings?section=appearance" {
		t.Errorf("location %q", got)
	}
	if r.s.find.query != "" {
		t.Errorf("the query was not cleared: %q", r.s.find.query)
	}
}

func TestALandingScrollsFocusesAndWashesTheRowForTwoSeconds(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.tt.Until("the page", r.tt.Shows("Settings"))
	r.s.focusSetting("theme") // asked before its section is built: the page waits for the row
	r.router.Replace(settingsPath + "?section=appearance")
	r.tt.Until("the landing", func() bool { return r.s.found.id == "theme" })
	if r.s.pending != nil {
		t.Error("the landing is still pending")
	}
	r.tt.Drain()
	if !r.tt.Focused("Theme") {
		t.Error("the Theme control did not take the focus")
	}
	if !r.s.now().Before(r.s.found.until) {
		t.Fatal("no wash")
	}
	r.tt.Advance(foundDuration + 100*time.Millisecond)
	r.tt.Drain()
	if r.s.now().Before(r.s.found.until) {
		t.Error("the wash outlived two seconds")
	}
}

func TestALandingOnARowThatNeverComesGivesUpAfterThreeSeconds(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.tt.Until("the page", r.tt.Shows("Settings"))
	r.s.focusSetting("no-such-row")
	r.tt.Advance(landDuration + 100*time.Millisecond)
	r.tt.Drain()
	if r.s.pending != nil {
		t.Error("still waiting after three seconds")
	}
	// A later landing replaces an earlier one.
	r.s.focusSetting("a")
	r.s.focusSetting("theme")
	if r.s.pending.id != "theme" {
		t.Errorf("pending %q", r.s.pending.id)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Every entry of the find index has its row on its section (find.test.tsx
// "the setting index"): a row Find lands on must exist whenever the index
// offers it.
func TestEverySettingInTheIndexHasARowOnItsSection(t *testing.T) {
	for _, section := range sections {
		t.Run(section, func(t *testing.T) {
			caps := linuxCaps()
			r := newRigCaps(t, 1132, 900, fakeShortcuts(), &caps)
			*stateOf[storageAPI](r.s, "seam:storage") = &fakeStorage{}
			r.router.Replace(settingsPath + "?section=" + section)
			var expected []string
			r.tt.Until("the section's rows", func() bool {
				st, caps := r.s.cfg(), r.s.caps()
				if !st.HasData || !caps.HasData {
					return false
				}
				ctx := findContext{caps: &caps.Data, settings: &st.Data, languageRow: r.s.languageRow()}
				expected = expected[:0]
				for _, e := range settingIndex(fakeShortcuts()) {
					if e.Section == section && (e.When == nil || e.When(ctx)) {
						expected = append(expected, e.ID)
					}
				}
				for _, id := range expected {
					if !r.s.seen[id] {
						return false
					}
				}
				return len(expected) > 0
			})
		})
	}
}
