package config

import (
	"strings"
	"testing"
)

func TestDesktopNormalizedDropsUnsafeRoutes(t *testing.T) {
	for _, r := range []string{
		"https://evil.example/",
		"//evil.example/x",
		"album/1",
		"/album/1#frag",
		"/" + strings.Repeat("a", maxRouteLen),
		"/album/1\n",
	} {
		if got := (Desktop{LastRoute: r}).normalized().LastRoute; got != "" {
			t.Errorf("LastRoute %q kept as %q", r, got)
		}
	}
	if got := (Desktop{LastRoute: "/album/1?song=2"}).normalized().LastRoute; got != "/album/1?song=2" {
		t.Errorf("valid route dropped: %q", got)
	}
}

func TestDesktopNormalizedUnknownValues(t *testing.T) {
	d := Desktop{CloseAction: "minimise", StartPage: "/settings"}.normalized()
	if d.CloseAction != CloseToBackground || d.StartPage != "/" {
		t.Fatalf("got %+v", d)
	}
}

func TestStartRoute(t *testing.T) {
	d := Desktop{StartPage: "/radio", LastRoute: "/album/1"}
	if got := d.StartRoute(); got != "/radio" {
		t.Errorf("resume off: %q", got)
	}
	d.ResumeLastRoute = true
	if got := d.StartRoute(); got != "/album/1" {
		t.Errorf("resume on: %q", got)
	}
	d.LastRoute = ""
	if got := d.StartRoute(); got != "/radio" {
		t.Errorf("resume on, nothing saved: %q", got)
	}
}

func TestWindowNormalized(t *testing.T) {
	for _, tc := range []struct{ in, want Window }{
		{Window{}, Window{Width: DefaultWindowWidth, Height: DefaultWindowHeight}},
		{Window{Width: 200, Height: 100, Maximised: true}, Window{Width: MinWindowWidth, Height: MinWindowHeight, Maximised: true}},
		{Window{Width: 1 << 20, Height: 900}, Window{Width: maxWindowEdge, Height: 900}},
	} {
		if got := tc.in.normalized(); got != tc.want {
			t.Errorf("%+v: got %+v, want %+v", tc.in, got, tc.want)
		}
	}
}
