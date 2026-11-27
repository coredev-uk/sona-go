package updates

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// layers is the module's Layer: the release notes dialog of the update on
// offer and the What's New dialog of the one just installed.
func (u *updates) layers(c *ui.Context) {
	u.dialog(c)
	u.whatsNewDialog(c)
}

// recheck is how long pending notes wait before asking again whether the
// dialog that held them is gone: kit has no signal for a dialog closing, and
// asking every frame would redraw the window the whole time one is open.
const recheck = 500 * time.Millisecond

// idle reports whether nothing else holds the window: not the first-run setup
// (a fresh install is not an update, but a computer set up before Sona
// recorded versions may still be in it), the account dialog nor the release
// notes of an update on offer (each closes by the user's input, which draws a
// frame that builds this again), and no other dialog in the last frame the
// shell finished. That one has no such signal: the caller rechecks after a
// delay.
func (u *updates) idle() (idle, dialog bool) {
	s := u.env.Surfaces
	if s.SetupOpen() || s.AccountOpen() || s.ReleaseNotesOpen() {
		return false, false
	}
	shown := u.dialogShown
	if shown == nil {
		shown = kit.DialogShown
	}
	return !shown(), shown()
}

// whatsNewDialog is the What's New dialog: the notes of the version just
// installed, with Close as its one action, open while Surfaces says so.
//
// Go fetches the notes after the first start of a new version and sends them
// (store.UseWhatsNew); they wait here until the window is idle, then open the
// dialog and are marked seen (UpdatesService.MarkWhatsNewSeen), so they show
// once. Settings → General → Updates opens the running version's notes the
// same way without waiting.
func (u *updates) whatsNewDialog(c *ui.Context) {
	s := u.env.Surfaces
	if _, ok := s.WhatsNew(); !ok {
		pending := store.UseWhatsNew(u.env.Queries, u.env.Core)
		if !pending.HasData || pending.Data.Version == "" {
			return
		}
		if idle, dialog := u.idle(); !idle {
			if dialog {
				c.After(recheck)
			}
			return
		}
		w := pending.Data
		s.OpenWhatsNew(w)
		store.Set(u.env.Queries, store.KeyWhatsNew, services.WhatsNew{})
		if u.markSeen != nil {
			u.markSeen(w.Version)
		}
	}
	w, _ := s.WhatsNew()
	open := true
	spec := kit.ReleaseNotesSpec{Version: w.Version, Notes: w.Notes}
	if w.Published > 0 {
		spec.Published = time.UnixMilli(w.Published)
	}
	u.whatsNew.Show(c, &open, spec)
	if !open {
		s.CloseWhatsNew()
	}
}
