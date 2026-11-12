package module

import (
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// TrackOpts is the shared base of a song list's options: artwork, motion, the
// item menu, what plays (the queue's current item, and whether it plays), the
// account's restriction, and the links to a row's artist, album and "added
// by" profile for the pages a module serves. The page adds its label,
// columns and OnPlay. Build it each frame, on the main thread.
func (e *Env) TrackOpts(menus *kit.ActionMenus, restricted bool) kit.TrackListOpts {
	o := kit.TrackListOpts{Art: e.Art, Motion: e.Motion, Menus: menus, Restricted: restricted}
	if e.Play != nil {
		st := e.Play.State()
		o.Current = st.Current
		o.Playing = st.State == store.StatePlaying || st.State == store.StateLoading
	}
	if e.Routed("/artist/x") {
		o.OpenArtist = func(id string) { e.Nav.Push("/artist/" + id) }
	}
	if e.Routed("/album/x") {
		o.OpenAlbum = func(id string) { e.Nav.Push("/album/" + id) }
	}
	if e.Routed("/profile/x") {
		o.OpenProfile = func(id string) { e.Nav.Push("/profile/" + id) }
	}
	return o
}
