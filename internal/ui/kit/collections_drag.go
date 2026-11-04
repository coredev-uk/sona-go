package kit

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Sona's one drag-and-drop mechanism for lists (v0.2.0's shared/dnd/drag.ts),
// over MyGo's value drag: a source is an element that Drag()s a payload, a
// target an element that takes it (Drop, DragOver). MyGo does the press
// tracking (a press moving as far as starts a drag is no click, Escape
// gives the drag up, the scroll container under the pointer scrolls as it
// nears its edge). The kit adds what Sona's lists need on top:
//
//   - a drag starts after the pointer travels DragThreshold (6 DIPs), as
//     v0.2.0's, not MyGo's 4; until then a press is a click;
//   - the ghost is a card (artwork, title, and a caption saying what a
//     drop here does) built in an Overlay beside the pointer, not MyGo's
//     translucent copy of the element, which the source leaves empty while
//     it drags (DragItem draws the item itself in its place);
//   - targets say what they accept and describe the drop; the innermost
//     accepting one takes it, and an inner target that declines lets the
//     nearest one around it take the drop; positional targets are told
//     where the pointer is.
//
// The engine is per process and main-thread only (one drag at a time), like
// theme.Current.

// DragThreshold is how far the pointer travels from a press before the press
// becomes a drag.
const DragThreshold = 6

// DragOrigin says a drag reorders inside one list: the list's id and the
// dragged row's position.
type DragOrigin struct {
	List  string
	Index int
}

// DragPayload is what is being dragged.
type DragPayload struct {
	// Items are the items, in order; empty for a queue row, which only
	// reorders.
	Items []Item
	// Origin is set for a reorder inside one list.
	Origin *DragOrigin
	// Label is the ghost's title ("Airbag", "12 songs") and Art its
	// artwork URL (shown at 32 DIPs; "" for none).
	Label string
	Art   string
}

// DragPoint is where the pointer is, in window DIPs and in the target's box.
type DragPoint struct {
	X, Y           float32
	LocalX, LocalY float32
}

// DropTarget describes what a drop target takes and does.
type DropTarget struct {
	// Accepts reports whether the payload may be dropped here; nil accepts
	// everything.
	Accepts func(p *DragPayload) bool
	// Describe is what a drop does, under the ghost's title: Add to
	// “Road trip”.
	Describe func(p *DragPayload) string
	// Hover is told where the pointer is over the target, every frame
	// (positional targets: a queue's drop line), and nil once it left or the
	// drag ended.
	Hover func(p *DragPayload, at *DragPoint)
	// Drop runs when the pointer lets go over the target.
	Drop func(p *DragPayload, at DragPoint)
}

func (t *DropTarget) accepts(p *DragPayload) bool { return t.Accepts == nil || t.Accepts(p) }

type dragTarget struct {
	t      DropTarget
	bounds ui.Rect
	stamp  uint64
}

// dragEngine is the state of the one drag in progress.
type dragEngine struct {
	payload *DragPayload
	// owner is the key of the source dragging, scope the list it is in
	// and index its position there.
	owner dragID
	scope any
	index int
	// tick counts the frames a source built while dragging: the clock of
	// the targets' "over" reports.
	tick    uint64
	targets map[string]*dragTarget

	// native is the target MyGo reports under the pointer, and where.
	native      string
	nativeStamp uint64
	nativeAt    DragPoint

	over    string // the effective target (it accepts the payload)
	hovered string // the target last told its Hover
	caption string
	at      DragPoint

	// The press being watched for the threshold.
	gate struct {
		key    dragID
		dx, dy float32
		on     bool
		// blocked is the source whose drag MyGo cancelled (Escape): it
		// arms nothing until the pointer is released.
		blocked *dragID
	}
	// saw is set once MyGo reported the owner dragging, so a payload set
	// this frame is not taken for a cancelled drag.
	saw bool
}

// dragID identifies a drag source: keys repeat across lists (the same album
// in two shelves), so the list is part of it.
type dragID struct{ scope, key any }

var drag dragEngine

// resetDrag forgets any drag; tests start from here.
func resetDrag() { drag = dragEngine{} }

// DragActive reports whether a drag that accepts would take is in progress:
// for targets that exist only while something is dragged (the sidebar's "New
// playlist" zone).
func DragActive(accepts func(p *DragPayload) bool) bool {
	return drag.payload != nil && (accepts == nil || accepts(drag.payload))
}

// DragPinned reports the position of the item being dragged out of the list
// scope, so a virtual list builds its row even when it scrolled out of view.
func DragPinned(scope any) (index int, ok bool) {
	if drag.payload == nil || drag.scope != scope || scope == nil {
		return 0, false
	}
	return drag.index, true
}

// DropIndex is where a row dragged from `from` lands when dropped into gap
// `slot` (0…n) of its list: slot-1 when slot is after it, else slot.
func DropIndex(from, slot int) int {
	if slot > from {
		return slot - 1
	}
	return slot
}

// DropOn makes e a drop target. id names it for the engine and must be unique
// among the targets of the window. Call it while building e. It reports
// whether the drag in progress is over e and e would take it, for the
// target to show it (an outline, a drop line).
func DropOn(e ui.Element, id string, t DropTarget) (over bool) {
	d := &drag
	if d.targets == nil {
		d.targets = map[string]*dragTarget{}
	}
	info := d.targets[id]
	if info == nil {
		info = &dragTarget{}
		d.targets[id] = info
	}
	info.t, info.bounds, info.stamp = t, e.Bounds(), d.tick

	if _, ok := ui.DragOver[*DragPayload](e); ok {
		x, y, _ := e.PointerPosition()
		wx, wy := e.LocalToWindow(x, y)
		d.native, d.nativeStamp = id, d.tick
		d.nativeAt = DragPoint{X: wx, Y: wy, LocalX: x, LocalY: y}
	}
	if p, ok := ui.Drop[*DragPayload](e); ok {
		d.drop(id, p)
	}
	return d.payload != nil && d.over == id
}

// resolve is the target a drop landing on `native` goes to: native itself
// when it accepts p, else the innermost target around it that does.
func (d *dragEngine) resolve(native string, p *DragPayload) string {
	n := d.targets[native]
	if n == nil {
		return ""
	}
	if n.t.accepts(p) {
		return native
	}
	best, bestArea := "", float32(math.MaxFloat32)
	for id, o := range d.targets {
		if id == native || !o.t.accepts(p) || !containsRect(o.bounds, n.bounds) {
			continue
		}
		if a := o.bounds.W * o.bounds.H; a < bestArea || a == bestArea && id < best {
			best, bestArea = id, a
		}
	}
	return best
}

func containsRect(outer, inner ui.Rect) bool {
	return outer.W > 0 && outer.H > 0 &&
		inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
}

// drop runs the drop that MyGo delivered to `native`.
func (d *dragEngine) drop(native string, p *DragPayload) {
	id := d.resolve(native, p)
	t := d.targets[id]
	if t == nil || t.t.Drop == nil {
		return
	}
	at := d.at
	if d.nativeAt != (DragPoint{}) {
		at = d.nativeAt
	}
	t.t.Drop(p, at)
}

// DragOpts are the options of DragItem.
type DragOpts struct {
	// Key identifies the item among those dragging (any comparable value).
	Key any
	// Scope and Index say which list the item is in and where: a virtual
	// list keeps the row being dragged built (DragPinned) so the ghost
	// does not vanish when edge scrolling moves the row out of view.
	Scope any
	Index int
	// Payload builds the payload when a drag starts; nil makes the item
	// not draggable, and returning nil does too (a keyless row).
	Payload func() *DragPayload
	// Art decodes the ghost's artwork; nil shows none.
	Art *ArtLoader
	// Row lays the item's children out in a row (else a column).
	Row bool
	// Button makes the source a button (Enter and Space activate it);
	// else a plain focusable box.
	Button bool
	// Outer styles the wrapper that holds the item: its size and look.
	Outer func(w ui.Element)
	// Source styles the element that takes the press and holds the content:
	// padding, gap, focus, label.
	Source func(s ui.Element)
}

// DragItem builds one draggable item: a wrapper (Outer) holding the source
// element s (Source) that draws content. While the item drags, content
// moves to a twin that takes no input so the item stays where it was and
// MyGo's copy of s, which follows the pointer, is empty; the ghost card
// shows in an Overlay instead. It returns the wrapper and the source, for
// the caller to read clicks, hover and focus from.
func DragItem(c *ui.Context, o DragOpts, content func(w, s ui.Element)) (w, s ui.Element) {
	d := &drag
	w = ui.Box(c.Key(o.Key))
	if o.Row {
		w.Row()
	}
	if o.Outer != nil {
		o.Outer(w)
	}
	w.Children(func() {
		if o.Button {
			s = ui.ButtonBase(c.Key("src"))
		} else {
			s = ui.Box(c.Key("src"))
		}
		s.Grow(1).MinWidth(0).MinHeight(0)
		if o.Row {
			s.Row().AlignItems(ui.Center)
		} else {
			s.Column().AlignItems(ui.Stretch).Justify(ui.Start)
		}
		if o.Source != nil {
			o.Source(s)
		}
		dragging := d.watch(c, s, o)
		if !dragging {
			s.Children(func() { content(w, s) })
			return
		}
		s.Children(func() {})
		twin := ui.Box(c.Key("twin")).Absolute().Left(0).Right(0).Top(0).Bottom(0).IgnoreInput(true)
		if o.Row {
			twin.Row().AlignItems(ui.Center)
		} else {
			twin.Column().AlignItems(ui.Stretch).Justify(ui.Start)
		}
		if o.Source != nil {
			o.Source(twin)
		}
		twin.Role(ui.RoleNone)
		twin.Children(func() { content(w, s) })
		d.ghost(c, s, o)
	})
	return w, s
}

// watch wires s as the source of a drag and reports whether it is dragging.
// The payload is only built, and the drag only armed, once the pointer has
// travelled the threshold from the press.
func (d *dragEngine) watch(c *ui.Context, s ui.Element, o DragOpts) bool {
	if o.Payload == nil {
		return false
	}
	id := dragID{o.Scope, o.Key}
	dragging := s.Dragging()
	dx, dy, pressing := s.Dragged()
	mine := d.payload != nil && d.owner == id
	switch {
	case dragging:
		if mine {
			d.saw = true
		}
	case pressing && mine && d.saw:
		// MyGo stopped the drag (Escape) with the pointer still down.
		d.end()
		blocked := id
		d.gate.blocked = &blocked
	case pressing:
		if d.gate.blocked != nil && *d.gate.blocked == id {
			break
		}
		if c.Modifiers() != 0 && !mine {
			break // Ctrl/Shift+press selects; it never drags
		}
		if d.gate.key != id || !d.gate.on {
			d.gate.key, d.gate.dx, d.gate.dy, d.gate.on = id, 0, 0, true
		}
		d.gate.dx += dx
		d.gate.dy += dy
		if math.Hypot(float64(d.gate.dx), float64(d.gate.dy)) >= DragThreshold && d.payload == nil {
			if p := o.Payload(); p != nil {
				d.payload, d.owner, d.scope, d.index, d.saw = p, id, o.Scope, o.Index, false
				clear(d.targets) // only the targets that show while it drags
			}
		}
	default:
		if d.gate.key == id {
			d.gate.on = false
		}
		if d.gate.blocked != nil && *d.gate.blocked == id {
			d.gate.blocked = nil
		}
		if mine {
			d.end()
		}
	}
	mine = d.payload != nil && d.owner == id
	if mine && (dragging || pressing) {
		s.Drag(d.payload)
	}
	if dragging && mine {
		s.Cursor(ui.CursorGrabbing)
		d.step(s)
		return true
	}
	return false
}

// step advances a drag by one frame: which target is under the pointer,
// the Hover calls, the caption.
func (d *dragEngine) step(s ui.Element) {
	d.tick++
	x, y, _ := s.PointerPosition()
	wx, wy := s.LocalToWindow(x, y)
	d.at = DragPoint{X: wx, Y: wy}

	over := ""
	if d.nativeStamp+1 >= d.tick {
		over = d.resolve(d.native, d.payload)
		if t := d.targets[over]; t != nil && over == d.native {
			d.at = d.nativeAt
		}
	}
	if d.hovered != "" && d.hovered != over {
		if t := d.targets[d.hovered]; t != nil && t.t.Hover != nil {
			t.t.Hover(d.payload, nil)
		}
	}
	d.hovered, d.over = over, over
	d.caption = ""
	if t := d.targets[over]; t != nil {
		if t.t.Describe != nil {
			d.caption = t.t.Describe(d.payload)
		}
		if t.t.Hover != nil {
			at := d.at
			t.t.Hover(d.payload, &at)
		}
	}
}

// end finishes the drag: the target under the pointer hears it left.
func (d *dragEngine) end() {
	if t := d.targets[d.hovered]; t != nil && t.t.Hover != nil && d.payload != nil {
		t.t.Hover(d.payload, nil)
	}
	d.payload, d.owner, d.saw = nil, dragID{}, false
	d.hovered, d.over, d.native, d.caption = "", "", "", ""
	d.gate.on = false
}

// ghostPlace is where the ghost card of width w goes for a pointer at (x, y)
// in a window ww wide: 14 DIPs right of the pointer and 10 below, flipped to
// the pointer's left near the window's right edge.
func ghostPlace(x, y, w, ww float32) (left, top float32) {
	left = x + 14
	if left+w > ww {
		left = x - 14 - w
	}
	return left, y + 10
}

// ghost builds the drag card above the window: artwork, the payload's label
// and a caption saying what a drop does, 14 DIPs right of the pointer and
// 10 below, flipped to its left near the window's right edge.
func (d *dragEngine) ghost(c *ui.Context, s ui.Element, o DragOpts) {
	p := d.payload
	if p == nil {
		return
	}
	tk := theme.Current()
	title, caption := tk.Type["body"], tk.Type["meta"]
	const maxW, art, pad, gap = 288, 32, 6, 10
	tw, _ := c.MeasureText(0, ui.Span{Text: p.Label, Size: title.Size})
	cw, _ := c.MeasureText(0, ui.Span{Text: d.caption, Size: caption.Size})
	w := pad + pad + gap + max(tw, cw) + 6 + 4 // the border and a little slack
	if p.Art != "" && o.Art != nil {
		w += art
	} else {
		w -= gap
	}
	w = min(w, maxW)
	ww, _ := c.Size()
	left, top := ghostPlace(d.at.X, d.at.Y, w, ww)
	ui.Overlay(c, func() {
		card := ui.Row(c.Key("kit.drag.ghost")).Absolute().Left(left).Top(top).Width(w).
			Padding(pad, 12, pad, pad).Gap(gap).AlignItems(ui.Center).Radius(tk.Radius["lg"]).
			Background(theme.C("raised")).Border(1, theme.C("line")).IgnoreInput(true)
		if sh, ok := tk.Shadow["float"]; ok {
			card.Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color)
		}
		card.Children(func() {
			if p.Art != "" && o.Art != nil {
				Artwork(c, o.Art, p.Art, art, ArtOpts{Radius: tk.Radius["sm"]})
			}
			ui.Column(c).MinWidth(0).Grow(1).Children(func() {
				ui.Text(c, p.Label).FontSize(title.Size).FixedLineHeight(title.LineHeight).
					TextColor(theme.C("ink")).SingleLine()
				if d.caption != "" {
					ui.Text(c, d.caption).FontSize(caption.Size).FixedLineHeight(caption.LineHeight).
						TextColor(theme.C("ink-muted")).SingleLine()
				}
			})
		})
	})
}
