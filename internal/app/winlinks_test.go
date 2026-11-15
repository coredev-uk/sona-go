package app

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// fakeWinLinks is the Windows registry and shell as WindowsLinkHandler sees
// them.
type fakeWinLinks struct {
	registered map[string]bool
	choice     map[string]string
	handler    map[string]string // display name of the app Windows resolves
	packaged   map[string]bool
	failOn     string // Register fails for this scheme
	asked      []string
}

func (f *fakeWinLinks) handlerOf() WindowsLinkHandler {
	return WindowsLinkHandler{
		Registered: func(s string) bool { return f.registered[s] },
		Register: func(s string) error {
			f.asked = append(f.asked, s)
			if s == f.failOn {
				return errors.New("access denied")
			}
			f.registered[s] = true
			return nil
		},
		Choice:  func(s string) string { return f.choice[s] },
		Handler: func(s string) (string, bool) { return f.handler[s], f.packaged[s] },
	}
}

func allSchemes(v bool) map[string]bool {
	m := map[string]bool{}
	for _, s := range LinkSchemes {
		m[s] = v
	}
	return m
}

func TestWindowsLinksDefault(t *testing.T) {
	for _, tc := range []struct {
		name      string
		f         fakeWinLinks
		sona      bool
		wantOther string
	}{
		{name: "all registered, no choice", f: fakeWinLinks{registered: allSchemes(true)}, sona: true},
		{name: "the choice names Sona's class", f: fakeWinLinks{registered: allSchemes(true), choice: map[string]string{"music": "Music", "itms": "itms"}}, sona: true},
		{
			name: "one scheme left (an install before musics:// was registered)",
			f:    fakeWinLinks{registered: map[string]bool{"music": true, "itms": true}, handler: map[string]string{"musics": "iTunes"}},
			sona: false, wantOther: "iTunes",
		},
		{
			name: "the user chose another app",
			f:    fakeWinLinks{registered: allSchemes(true), choice: map[string]string{"itms": "AppX6w1q"}, handler: map[string]string{"itms": "Apple Music"}},
			sona: false, wantOther: "Apple Music",
		},
		{
			name: "a packaged app holds the scheme",
			f:    fakeWinLinks{registered: allSchemes(true), handler: map[string]string{"music": "Apple Music"}, packaged: map[string]bool{"music": true}},
			sona: false, wantOther: "Apple Music",
		},
		{name: "nothing registered, no app", f: fakeWinLinks{registered: allSchemes(false)}, sona: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sona, other, err := tc.f.handlerOf().Default()
			if err != nil || sona != tc.sona || other != tc.wantOther {
				t.Fatalf("Default = %v, %q, %v; want %v, %q", sona, other, err, tc.sona, tc.wantOther)
			}
		})
	}
}

// Making Sona the default registers every scheme, and one that fails
// doesn't stop the rest.
func TestWindowsLinksMakeDefault(t *testing.T) {
	f := &fakeWinLinks{registered: allSchemes(false), failOn: "itms"}
	h := f.handlerOf()
	err := h.MakeDefault(t.Context())
	if err == nil || !slices.Equal(f.asked, LinkSchemes) {
		t.Fatalf("MakeDefault = %v, asked %v", err, f.asked)
	}
	f.failOn = ""
	if err := h.MakeDefault(t.Context()); err != nil {
		t.Fatal(err)
	}
	if sona, _, _ := h.Default(); !sona {
		t.Fatal("not the default after MakeDefault")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := h.MakeDefault(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("MakeDefault with an ended context = %v", err)
	}
}
