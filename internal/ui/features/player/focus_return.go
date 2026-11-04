package player

import "github.com/egoist/mygo/ui"

// returnFocus gives the keyboard focus back to the bar control that opened
// Now Playing when the view closes (Escape, its Close, a toggle key), as
// v0.2.0's useReturnFocus did: the cover, or Lyrics. Only when the focus was
// inside the view (someone who went on to a sidebar link keeps it there). A
// view a key opened has no opener, and a control that is not built that frame
// (the cover with nothing playing is disabled, Lyrics gives way to Show video)
// is not asked again: the request is taken back on the next frame, so it never
// takes the focus when the control returns much later. Either way the focus is
// then with no control, and Tab starts at the page.
//
// The bar calls it in every frame it builds. Full-screen lyrics close into Now
// Playing, which puts the focus on its Full screen button (view.fromRoom); the
// queue beside the page returns its own (queueAside); the video view's opener
// is the video view's (store.VideoView).
func (p *player) returnFocus(c *ui.Context) {
	v := p.ui
	if v.restoring != nil {
		v.restoring.CancelFocus(c)
		v.restoring = nil
	}
	if v.overlay != overlayNone || v.opener == nil {
		return
	}
	opener := v.opener
	v.opener = nil
	if !p.np.hadFocus {
		return
	}
	p.np.hadFocus = false
	v.restoring = opener
	opener.Focus(c)
}

// opened runs open with the bar control bound to h as the one opening a
// panel, so closing the panel returns the focus to it.
func (p *player) opened(h *ui.Handle, open func()) {
	p.ui.opening = h
	open()
	p.ui.opening = nil
}
