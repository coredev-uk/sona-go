package main

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/app"
)

func TestRelaunchArgs(t *testing.T) {
	const link = "music://music.apple.com/us/album/x/1"
	for _, tc := range []struct {
		name    string
		started []string
		want    []string
	}{
		{"started plain", nil, nil},
		{"autostarted hidden", []string{"--hidden"}, []string{app.ShownFlag}},
		{"opened by a link", []string{link}, []string{app.ShownFlag}},
		{"hidden with a link", []string{"--hidden", link}, []string{app.ShownFlag}},
		{"another argument stays", []string{"--hidden", "--flag"}, []string{"--flag"}},
		{"nothing to drop", []string{"--flag"}, []string{"--flag"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := relaunchArgs(tc.started)
			if !slices.Equal(got, tc.want) {
				t.Errorf("relaunchArgs(%q) = %q, want %q", tc.started, got, tc.want)
			}
			// What a restart is given must never ask for no window, and an
			// instance that began with arguments must not repeat them
			// by being given none (MyGo repeats the old ones then).
			if slices.Contains(got, app.HiddenFlag) {
				t.Errorf("relaunchArgs(%q) = %q keeps --hidden", tc.started, got)
			}
			if len(tc.started) > 0 && len(got) == 0 {
				t.Errorf("relaunchArgs(%q) = nothing: MyGo would repeat the old arguments", tc.started)
			}
		})
	}
}

// The stand-in flag starts Sona like any launch with a window: it is no
// command, no link and not hidden.
func TestShownFlagIsInert(t *testing.T) {
	act := app.Activation{Args: []string{app.ShownFlag}}
	if act.Hidden() || app.IsMusicURL(app.ShownFlag) {
		t.Error("--shown is read as hidden or as a link")
	}
}
