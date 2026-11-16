package main

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
)

// The first start of an update finds the version it replaced still holding
// the single instance (it starts this one as it quits): it waits for it to
// let go and takes over, where a plain start would hand its launch to the
// instance on its way out and exit, leaving no Sona.
func TestOpenPlatformAfterUpdateWaitsForTheReplacedVersionToQuit(t *testing.T) {
	calls := 0
	want := &platform{}
	open := func(config.Paths, app.Activation, *slog.Logger) (*platform, error) {
		calls++
		if calls < 3 {
			return nil, app.ErrForwarded // the old version is still quitting
		}
		return want, nil
	}
	got, err := openPlatformAfterUpdate(config.Paths{}, app.Activation{}, slog.New(slog.DiscardHandler), open)
	if err != nil || got != want || calls != 3 {
		t.Fatalf("got %v, %v after %d tries; want the platform on the third", got, err, calls)
	}
}

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
