package kit

import "github.com/egoist/mygo/ui"

// Openers are the handles of controls that open something over the page,
// by key: a list of tiles keeps one per tile, bound to its play button
// (Element.Bind), so the video view the button opens can return the focus to
// it. The zero value is ready.
type Openers struct{ m map[string]*ui.Handle }

// For is the handle for key, made on first use. It stays the same one for
// as long as the Openers does.
func (o *Openers) For(key string) *ui.Handle {
	if h := o.m[key]; h != nil {
		return h
	}
	if o.m == nil {
		o.m = map[string]*ui.Handle{}
	}
	h := new(ui.Handle)
	o.m[key] = h
	return h
}
