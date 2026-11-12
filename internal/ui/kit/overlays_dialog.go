package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// DialogOpts shapes a Dialog (v0.2.0's shared/ui/dialog.tsx).
type DialogOpts struct {
	// Title names the dialog for assistive technology and, unless
	// HideHeader, heads it (18 semibold). Description is the muted 14 line
	// under it, read as the dialog's description.
	Title, Description string
	// HideHeader keeps Title and Description for assistive technology only
	// (the command palette's sr-only title).
	HideHeader bool
	// MaxWidth caps the panel; 0 is 32 rem (512). It is never wider than
	// the window less 2 rem.
	MaxWidth float32
	// Padding is the panel's inset (24 by default) and Gap the space
	// between its children (16).
	Padding, Gap float32
	// HideClose drops the top-right X button.
	HideClose bool
	// Alert makes it an alertdialog (ConfirmDialog).
	Alert bool
	// Bare drops the padding and the gap and clips the corners (the
	// command palette lays out its own rows).
	Bare bool
	// Raised paints the panel on the raised tone (the palette), not the ground.
	Raised bool
	// HeadingTitle sets the title in the page heading style (16/22
	// semibold) instead of 18 (v0.2.0's `text-heading` DialogTitle).
	HeadingTitle bool
	// Compact is the confirm dialog's header: 16 semibold ink title, 14
	// muted body, 6 apart.
	Compact bool
	// Top places the panel that far from the top instead of centring it
	// (the command palette: 14 vh).
	Top float32
}

// Dialog shows a modal dialog over the window while *open is true: a scrim
// (black at 50 % in light, 70 % in dark; never blurred), the panel centred
// (max 32 rem, rounded 8, 1 px border, the float shadow, fading and
// zooming in over 200 ms unless motion is reduced) with a top-right X
// "Close" button, and build for its content. Escape or a click on the scrim
// sets *open to false, as does the X; Dismissed on the returned panel
// reports it. The page behind is inert, Tab stays inside, and closing gives
// the focus back to the element that had it. It returns the panel, a zero
// Element while closed.
//
//	kit.Dialog(c, &s.renaming, kit.DialogOpts{Title: i18n.T("Rename playlist")}, func() {…})
func Dialog(c *ui.Context, open *bool, o DialogOpts, build func()) ui.Element {
	if !*open {
		return ui.Element{}
	}
	dialogsBuilt++
	tk := theme.Current()
	scrim := ui.RGBA(0, 0, 0, 0.5)
	if theme.Dark() {
		scrim = ui.RGBA(0, 0, 0, 0.7)
	}
	maxW := o.MaxWidth
	if maxW == 0 {
		maxW = 512
	}
	pad, gap := o.Padding, o.Gap
	if pad == 0 && !o.Bare {
		pad = 24
	}
	if gap == 0 && !o.Bare {
		gap = 16
	}
	surface := theme.C("background")
	if o.Raised {
		surface = theme.C("raised")
	}
	var panel ui.Element
	ui.DialogBase(c, open, func(back, p ui.Element) {
		panel = p
		w, _ := c.Size()
		back.Background(scrim)
		if o.Top > 0 {
			back.AlignItems(ui.Center).Justify(ui.Start).Padding(o.Top, 0, 0, 0)
		}
		sh := tk.Shadow["float"]
		p.Width(min(maxW, max(w-32, 0))).Padding(pad).Gap(gap).Radius(tk.Radius["lg"]).Background(surface).
			Border(1, theme.C("border")).Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color).Label(o.Title).Description(o.Description)
		if o.Bare {
			p.Clip()
		}
		if o.Alert {
			p.Role(ui.RoleAlertDialog)
		}
		if !theme.ReducedMotion() {
			m := tk.Motion["float"]
			p.Transition(ui.ElementTransition{Enter: &ui.Motion{Scale: 0.95}, Duration: 200 * time.Millisecond, Ease: m.Ease})
		}
		p.Children(func() {
			if !o.HideHeader && (o.Title != "" || o.Description != "") {
				dialogHeader(c, o.Title, o.Description, o.Compact, o.HeadingTitle)
			}
			build()
			if !o.HideClose {
				if closeButton(c).Clicked() {
					*open = false
				}
			}
		})
	})
	return panel
}

// dialogsBuilt counts the dialogs built since TakeDialogsBuilt. Main thread
// only, like every view.
var dialogsBuilt int

// TakeDialogsBuilt returns how many dialogs (Dialog, ConfirmHost, the
// palette) were built since it was last called, and resets the count. The
// shell calls it once at the end of each frame: a modal open in the frame
// that just ended is a modal covering the page.
func TakeDialogsBuilt() int {
	n := dialogsBuilt
	dialogsBuilt = 0
	return n
}

// closeButton is the dialog's top-right X, named "Close", with the focus
// ring; it sits in the corner whatever the content does.
func closeButton(c *ui.Context) ui.Element {
	b := ui.ButtonBase(c.Key("dialog:close")).Label(i18n.T("Close")).Absolute().Top(16).Right(16).Size(20, 20).
		Radius(theme.Current().Radius["sm"]).Opacity(0.7)
	if b.Hovered() {
		b.Opacity(1)
	}
	b.TextColor(theme.C("muted-foreground")).Center().Children(func() {
		theme.Glyph(c, "x", 16, theme.GlyphOpts{Color: theme.C("muted-foreground"), Inherit: true})
	})
	return b
}

func dialogHeader(c *ui.Context, title, description string, compact, heading bool) {
	gap, fg := float32(8), theme.C("muted-foreground")
	if compact {
		gap, fg = 6, theme.C("ink-muted")
	}
	ui.Column(c.Key("dialog:header")).Gap(gap).Children(func() {
		if title != "" {
			t := theme.Text(c.Key("t"), title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
			if !compact && !heading {
				t.FontSize(18).FixedLineHeight(18).FontWeight(600)
			}
		}
		if description != "" {
			theme.Text(c.Key("d"), description, "body").TextColor(fg)
		}
	})
}

// DialogFooter lays out a dialog's buttons right-aligned, 8 apart. With
// showClose it ends in an outline "Close" button that sets *open to false.
func DialogFooter(c *ui.Context, open *bool, showClose bool, build func()) {
	ui.Row(c.Key("dialog:footer")).Gap(8).Justify(ui.End).AlignItems(ui.Center).FillWidth().Children(func() {
		build()
		if showClose {
			if Button(c.Key("footer-close"), i18n.T("Close"), ButtonOpts{Variant: Outline}).Clicked() {
				*open = false
			}
		}
	})
}

// ConfirmOptions is a question for Confirm.
type ConfirmOptions struct {
	// Title is a question naming the item: "Delete “OK Computer” from your
	// library?"
	Title string
	// Body is one sentence on what else happens.
	Body string
	// ConfirmLabel is the verb on the confirming button: "Delete".
	ConfirmLabel string
	// Destructive styles the confirming button as destructive (outline in
	// --danger).
	Destructive bool
	// CancelLabel words the cancelling button; "" is "Cancel".
	CancelLabel string
	// Hold keeps the question up once it is confirmed, busy: done(true) runs,
	// the confirming button shows BusyLabel with a spinner and takes no
	// click, Cancel is disabled and Escape does nothing, until the caller
	// ends it with ConfirmClose (it worked) or ConfirmRelease (it failed:
	// the same question is asked again).
	Hold bool
	// BusyLabel is the confirming button's label while held: "Signing out…".
	BusyLabel string
}

type confirmRequest struct {
	ConfirmOptions
	done  func(bool)
	focus FocusStep
	busy  bool // confirmed and held: done has been told
}

// FocusStep counts the frames of a FocusWhenShown: the zero value is a focus
// not yet given.
type FocusStep uint8

// FocusWhenShown focuses e once, in the second frame it is built, for the
// control a dialog or overlay opens on. MyGo notes the element that had the
// focus as an overlay appears, after the overlay's content is built, to give
// it back as the overlay closes: a control that takes the focus (AutoFocus)
// in that first frame is taken for the opener, and the focus is lost when
// the overlay goes. Keep one FocusStep per opening of the overlay.
func FocusWhenShown(c *ui.Context, e ui.Element, step *FocusStep) {
	switch *step {
	case 0:
		*step = 1
		c.Invalidate()
	case 1:
		*step = 2
		e.Focus()
	}
}

// pendingConfirm is the question ConfirmHost shows. Main thread only, like
// every view and callback.
var pendingConfirm *confirmRequest

// Confirm asks the user to confirm an action that cannot be undone from
// Sona and calls done with true when they confirm, false when they cancel,
// press Escape or close the dialog. Asking again settles the pending
// question false. It needs a mounted ConfirmHost (the shell mounts one in
// OverlayLayer). Main thread only.
func Confirm(o ConfirmOptions, done func(ok bool)) {
	if p := pendingConfirm; p != nil {
		pendingConfirm = nil
		if p.done != nil && !p.busy {
			p.done(false)
		}
	}
	pendingConfirm = &confirmRequest{ConfirmOptions: o, done: done}
}

// settleConfirm answers the pending question. A confirmed Hold question
// stays up, busy.
func settleConfirm(ok bool) {
	p := pendingConfirm
	if p == nil {
		return
	}
	if p.busy {
		return
	}
	if ok && p.Hold {
		p.busy = true
		if p.done != nil {
			p.done(true)
		}
		return
	}
	pendingConfirm = nil
	if p.done != nil {
		p.done(ok)
	}
}

// CancelConfirm settles the pending question false (the window it was asked
// in is going away). A held question is dropped; it was answered already.
func CancelConfirm() {
	if p := pendingConfirm; p != nil && p.busy {
		pendingConfirm = nil
		return
	}
	settleConfirm(false)
}

// ConfirmClose ends a held question that worked: the dialog closes.
func ConfirmClose() {
	if p := pendingConfirm; p != nil && p.busy {
		pendingConfirm = nil
	}
}

// ConfirmRelease ends the busy state of a held question that failed: the
// question is asked again, enabled, and answers through the same callback.
func ConfirmRelease() {
	if p := pendingConfirm; p != nil && p.busy {
		p.busy = false
		p.focus = 0
	}
}

// ConfirmPending reports whether a question is waiting for an answer.
func ConfirmPending() bool { return pendingConfirm != nil }

// ConfirmHost renders the pending Confirm question as an alertdialog (no X
// button, max 420 DIPs): its title and body, an outline "Cancel" and the
// confirming button, which has the focus. Mount it once, in the shell.
func ConfirmHost(c *ui.Context) {
	p := pendingConfirm
	if p == nil {
		return
	}
	open := true
	Dialog(c.Key("confirm"), &open, DialogOpts{
		Title: p.Title, Description: p.Body, MaxWidth: 420, Padding: 20, Gap: 20, HideClose: true, Alert: true, Compact: true,
	}, func() {
		ui.Row(c.Key("confirm:buttons")).Gap(8).Justify(ui.End).FillWidth().Children(func() {
			cancel := p.CancelLabel
			if cancel == "" {
				cancel = i18n.T("Cancel")
			}
			if Button(c.Key("confirm:cancel"), cancel, ButtonOpts{Variant: Outline, Size: SizeSM, Disabled: p.busy}).Clicked() {
				settleConfirm(false)
			}
			v := Default
			if p.Destructive {
				v = Destructive
			}
			label, icon := p.ConfirmLabel, ""
			if p.busy {
				label, icon = p.BusyLabel, "loader-circle"
				if label == "" {
					label = p.ConfirmLabel
				}
			}
			ok := Button(c.Key("confirm:ok"), label, ButtonOpts{Variant: v, Size: SizeSM, Icon: icon, Spin: p.busy, Disabled: p.busy})
			if !p.busy {
				FocusWhenShown(c, ok, &p.focus)
			}
			if ok.Clicked() {
				settleConfirm(true)
			}
		})
	})
	if !open && pendingConfirm == p {
		settleConfirm(false) // a busy question ignores it
	}
}

// PopoverOpts shapes a Popover.
type PopoverOpts struct {
	// Width of the panel; 0 is 288 (w-72).
	Width float32
	// Padding inside it; 0 is 16.
	Padding float32
}

// Popover shows a panel below anchor while *open is true (above it where
// there is no room): popover colours, 1 px border, radius 8, the float
// shadow, 4 DIPs from the anchor, fading in. A press outside it or Escape
// sets *open to false; the press goes on to what is under the pointer, and
// closing gives the focus back. It returns the panel, a zero Element while
// closed.
func Popover(c *ui.Context, anchor ui.Element, open *bool, o PopoverOpts, build func()) ui.Element {
	tk := theme.Current()
	w, pad := o.Width, o.Padding
	if w == 0 {
		w = 288
	}
	if pad == 0 {
		pad = 16
	}
	return ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		sh := tk.Shadow["float"]
		panel.Width(w).Padding(pad).Margin(4, 0, 0, 0).Radius(tk.Radius["lg"]).Background(theme.C("popover")).
			Border(1, theme.C("border")).Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color).TextColor(theme.C("popover-foreground"))
		if !theme.ReducedMotion() {
			m := tk.Motion["float"]
			panel.Transition(ui.ElementTransition{Enter: &ui.Motion{Scale: 0.95}, Duration: m.Duration, Ease: m.Ease})
		}
		build()
	})
}
