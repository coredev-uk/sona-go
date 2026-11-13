package library

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The library's main pages over the synthetic library, light and dark, at
// a 940×700 window (the shell's narrowest): the few pictures that say the visual language did
// not move by accident. `go test ./internal/ui/features/library -update`
// accepts a change on purpose.
func TestLibraryPagesGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, page := range []struct {
		name, path, ready string
	}{
		{"albums", albumsPath, "24 albums"},
		{"songs", songsPath, "Songs in your library"},
		{"artists", artistsPath, "Filter artists"},
	} {
		for _, mode := range []struct {
			name string
			opts []uitest.Option
		}{
			{"light", nil},
			{"dark", []uitest.Option{uitest.Dark()}},
		} {
			t.Run(page.name+"-"+mode.name, func(t *testing.T) {
				r := newPageRig(t, page.path, 940, 700, nil, mode.opts...)
				r.tt.Until("the page", r.tt.Shows(page.ready))
				r.tt.Until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
				r.tt.Settle()
				uitest.Golden(t, r.tt.Image(), "library-"+page.name+"-"+mode.name)
			})
		}
	}
}

// The two pages whose composition is the redesign's own: Recently added filed
// under its ages, and History as a timeline.
func TestLibraryGroupedPagesGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	t.Run("recent-light", func(t *testing.T) {
		r := newPageRig(t, recentPath, 940, 700, nil)
		r.tt.Until("the page", r.tt.Shows("Earlier"))
		r.tt.Until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
		r.tt.Settle()
		uitest.Golden(t, r.tt.Image(), "library-recent-light")
	})
	for _, mode := range []struct {
		name string
		opts []uitest.Option
	}{{"light", nil}, {"dark", []uitest.Option{uitest.Dark()}}} {
		t.Run("history-"+mode.name, func(t *testing.T) {
			stub := &historyStub{answer: fixed(twoDays())}
			r := newPageRig(t, historyPath, 940, 700, func(l *library) {
				stub.libraryAPI = l.lib
				l.lib = stub
				fakeAuth(l, historyAccount("signed_in"))
			}, mode.opts...)
			r.tt.Until("the days", r.tt.Shows("Today", "Yesterday", "Alpha"))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), "library-history-"+mode.name)
		})
	}
}
