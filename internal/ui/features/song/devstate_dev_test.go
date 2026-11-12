//go:build dev

package song

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/devstate"
)

// The song page keeps its scroll offset across dev restarts: devstate.Scroll
// restores the saved offset of its location into the page's scroll place. Run
// with -tags dev (the default build's devstate.Scroll does nothing).
func TestSongPageRestoresItsScrollOffsetInDevBuilds(t *testing.T) {
	const want = 150
	r := newRigWith(t, 1132, 300, func(r *rig) {
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		devstate.SetLocation(r.router.Location)
		t.Cleanup(func() { devstate.SetLocation(nil) })
		devstate.Restore(devstate.State{Scroll: map[string]float32{"/song/1440": want}})
	}, airbag())
	r.open()
	r.tt.Until("the saved offset restored", func() bool { return r.s.page.Pos.Y >= want-1 })
}
