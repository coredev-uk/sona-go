package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// RatingAction is what a click on RatingButtons asks for: the same names as
// the library service's rating verbs.
type RatingAction string

// The rating actions; RateNone is no click.
const (
	RateNone            RatingAction = ""
	RateFavorite        RatingAction = "favorite"
	RateUnfavorite      RatingAction = "unfavorite"
	RateSuggestLess     RatingAction = "suggest-less"
	RateUndoSuggestLess RatingAction = "undo-suggest-less"
)

// RatingOpts are the options of RatingButtons.
type RatingOpts struct {
	// Name is the item's name, in the buttons' labels.
	Name string
	// Rating is the item's state: 1 Favourite, -1 Suggest Less, 0 none.
	Rating int
	// Pending disables both while the item's state loads or a rating is
	// being saved.
	Pending bool
	// Compact uses 24px icon buttons (bars and rows), else 32px.
	Compact bool
	// HideSuggestLess shows Favourite alone (the now-playing bar).
	HideSuggestLess bool
}

// RatingButtons are Favourite (a star, filled when the rating is 1) and
// Suggest Less (a thumb down, with an active fill when it is -1) for an
// item's header, built into the current parent. The caller owns the state
// and the service call: the buttons report the action asked for, and show
// whatever Rating says afterwards.
// (shared/components/RatingButtons.tsx)
func RatingButtons(c *ui.Context, o RatingOpts) RatingAction {
	favourite, less := o.Rating == 1, o.Rating == -1
	size := SizeSM
	if o.Compact {
		size = SizeXS
	}
	action := RateNone
	name := i18n.T("Favourite {name}", "name", o.Name)
	tip := i18n.T("Favourite")
	if favourite {
		name, tip = i18n.T("Undo favourite for {name}", "name", o.Name), i18n.T("Undo favourite")
	}
	if Button(c.Key("favourite"), "", ButtonOpts{
		Variant: Ghost, Size: size, Icon: "star", Filled: favourite, Muted: true, Quiet: true,
		Pressed: &favourite, Disabled: o.Pending, A11yLabel: name, Tip: tip,
	}).Clicked() {
		action = RateFavorite
		if favourite {
			action = RateUnfavorite
		}
	}
	if o.HideSuggestLess {
		return action
	}
	name = i18n.T("Suggest less like {name}", "name", o.Name)
	tip = i18n.T("Suggest less")
	if less {
		name, tip = i18n.T("Undo suggest less for {name}", "name", o.Name), i18n.T("Undo suggest less")
	}
	if Button(c.Key("suggest-less"), "", ButtonOpts{
		Variant: Ghost, Size: size, Icon: "thumbs-down", Filled: less, Muted: true,
		Pressed: &less, Disabled: o.Pending, A11yLabel: name, Tip: tip,
	}).Clicked() {
		action = RateSuggestLess
		if less {
			action = RateUndoSuggestLess
		}
	}
	return action
}

// LibraryAction is what a click on LibraryButton asks for.
type LibraryAction int

// The library actions; LibraryNone is no click.
const (
	LibraryNone LibraryAction = iota
	LibraryAdd
	// LibraryRemove asks to delete the item from the library: the caller
	// confirms first.
	LibraryRemove
)

// LibraryOpts are the options of LibraryButton.
type LibraryOpts struct {
	// Name is the item's name, in the Remove button's label.
	Name string
	// InLibrary is the item's membership: from the batched item state, or
	// its source being the library before that resolves.
	InLibrary bool
	// Pending disables the Add button while the state loads or a change is
	// being saved.
	Pending bool
	// AddLabel replaces "Add" ("Pre-add" for an album not out yet).
	AddLabel string
	// Playlist is a playlist: once in the library it shows a plain "In
	// library" label, since the playlist editor owns deletion.
	Playlist bool
}

// LibraryButton is a header's library control: an outline "Add" button
// (with a plus) for an item outside the library, an outline "In library"
// check button that asks to remove it once inside, or for a library
// playlist a non-interactive "In library" label.
// (shared/components/LibraryButton.tsx)
func LibraryButton(c *ui.Context, o LibraryOpts) LibraryAction {
	switch {
	case !o.InLibrary:
		label := o.AddLabel
		if label == "" {
			label = i18n.T("Add")
		}
		if Button(c, label, ButtonOpts{Variant: Outline, Size: SizeSM, Icon: "plus", Disabled: o.Pending}).Clicked() {
			return LibraryAdd
		}
	case o.Playlist:
		ui.Row(c).Height(32).PaddingX(8).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Icon(c, theme.Icon("check")).FontSize(16).TextColor(theme.C("ink-muted"))
			controlsText(c, i18n.T("In library"), "body").TextColor(theme.C("ink-muted")).SingleLine()
		})
	default:
		if Button(c, i18n.T("In library"), ButtonOpts{
			Variant: Outline, Size: SizeSM, Icon: "check",
			A11yLabel: i18n.T("In library. Delete “{name}” from your library", "name", o.Name),
			Tip:       i18n.T("Delete from library"),
		}).Clicked() {
			return LibraryRemove
		}
	}
	return LibraryNone
}
