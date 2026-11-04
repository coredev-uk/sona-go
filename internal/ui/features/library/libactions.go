package library

import (
	"context"
	"sync"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The library's item commands with their feedback (toasts, the removal
// confirmation), shared by header controls and menu actions: v0.2.0's
// shared/lib/library-actions.ts. Stand-in for the same methods of
// kit.ItemStates (album-artist-song PR); drop at rebase.

// plainToast is sonner's toast(title): no icon, nothing but the title.
func plainToast(title string) { kit.ShowToast(kit.ToastSpec{Title: title}) }

// ratingFailure is the toast of a failed change of subject
// (“Airbag”, "12 songs", "2 of 12 songs").
func ratingFailure(ch stateChange, subject string) string {
	switch kit.RatingAction(ch) {
	case kit.RateFavorite:
		return i18n.T("Couldn't favourite {subject}", "subject", subject)
	case kit.RateUnfavorite:
		return i18n.T("Couldn't undo favourite for {subject}", "subject", subject)
	case kit.RateSuggestLess:
		return i18n.T("Couldn't suggest less of {subject}", "subject", subject)
	case kit.RateUndoSuggestLess:
		return i18n.T("Couldn't undo suggest less for {subject}", "subject", subject)
	}
	return i18n.T("Couldn't remove {subject}", "subject", subject)
}

// Rate changes an item's rating from the main thread, with a toast naming
// what failed.
func (s *itemStates) Rate(kind string, it stateItem, name string, a kit.RatingAction) {
	if a == kit.RateNone || it.ID == "" {
		return
	}
	go func() {
		if _, err := s.Change(context.Background(), kind, it, ratingChange(a)); err != nil {
			failToast(ratingFailure(ratingChange(a), i18n.T("“{name}”", "name", name)), err)
		}
	}()
}

// kindedItem is one item of a bulk command, with its state kind.
type kindedItem struct {
	Kind string
	Item stateItem
}

// changeAll changes every item at once (each shows at once) and returns the
// failures' count and the first.
func (s *itemStates) changeAll(items []kindedItem, ch stateChange) (failed int, first error) {
	var wg sync.WaitGroup
	errs := make([]error, len(items))
	for i, it := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.Change(context.Background(), it.Kind, it.Item, ch)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			failed++
			if first == nil {
				first = err
			}
		}
	}
	return failed, first
}

// RateMany changes the rating of several items at once; one toast names how
// many failed, what being the items in words ("12 songs").
func (s *itemStates) RateMany(items []kindedItem, a kit.RatingAction, what string) {
	if a == kit.RateNone || len(items) == 0 {
		return
	}
	go func() {
		failed, first := s.changeAll(items, ratingChange(a))
		if failed == 0 {
			return
		}
		subject := what
		if failed != len(items) {
			subject = i18n.T("{count} of {what}", "count", failed, "what", what)
		}
		failToast(ratingFailure(ratingChange(a), subject), first)
	}()
}

// AddOne adds a catalogue item to the library, with a toast either way.
func (s *itemStates) AddOne(kind string, it stateItem, name string) {
	s.AddMany([]kindedItem{{kind, it}}, i18n.T("“{name}”", "name", name))
}

// AddMany adds catalogue items of several kinds to the library, one call per
// kind, with one toast naming them (what: "“Airbag”", "3 songs").
func (s *itemStates) AddMany(items []kindedItem, what string) {
	if len(items) == 0 {
		return
	}
	go func() {
		byKind := map[string][]stateItem{}
		var kinds []string
		for _, it := range items {
			if _, ok := byKind[it.Kind]; !ok {
				kinds = append(kinds, it.Kind)
			}
			byKind[it.Kind] = append(byKind[it.Kind], it.Item)
		}
		for _, kind := range kinds {
			if err := s.Add(context.Background(), kind, byKind[kind]); err != nil {
				failToast(i18n.T("Couldn't add {what} to your library", "what", what), err)
				return
			}
		}
		plainToast(i18n.T("Added {what} to your library", "what", what))
	}()
}

// removeNote is what else a removal takes with it.
func removeNote(kind string) string {
	switch kind {
	case stateAlbums:
		return i18n.T("Its songs leave your library and any playlists they're in.")
	case stateArtists:
		return i18n.T("Their albums and songs leave your library too.")
	default:
		return i18n.T("It also leaves any playlists it's in.")
	}
}

// RemoveOne asks, then deletes an item from the library, with a toast
// either way. Main thread only.
func (s *itemStates) RemoveOne(kind string, it stateItem, name string) {
	kit.Confirm(kit.ConfirmOptions{
		Title:        i18n.T("Delete “{name}” from your library?", "name", name),
		Body:         removeNote(kind),
		ConfirmLabel: i18n.T("Delete"),
		Destructive:  true,
	}, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			if _, err := s.Change(context.Background(), kind, it, changeRemove); err != nil {
				failToast(i18n.T("Couldn't delete “{name}”", "name", name), err)
				return
			}
			plainToast(i18n.T("Deleted “{name}” from your library", "name", name))
		}()
	})
}

// RemoveMany asks once, then deletes several items from the library, with
// one toast (what: "3 songs").
func (s *itemStates) RemoveMany(items []kindedItem, what string) {
	if len(items) == 0 {
		return
	}
	kit.Confirm(kit.ConfirmOptions{
		Title:        i18n.T("Delete {what} from your library?", "what", what),
		Body:         i18n.T("They also leave any playlists they're in."),
		ConfirmLabel: i18n.T("Delete"),
		Destructive:  true,
	}, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			if failed, first := s.changeAll(items, changeRemove); failed > 0 {
				failToast(i18n.T("Couldn't delete {what}", "what", what), first)
				return
			}
			plainToast(i18n.T("Deleted {what} from your library", "what", what))
		}()
	})
}

// copyLink copies an item's music.apple.com link (Go writes the clipboard):
// kind is a state kind or "stations".
func (l *library) copyLink(kind string, it stateItem) {
	l.run(i18n.T("Couldn't copy the link"), func(context.Context) error {
		_, err := l.lib.CopyLink(itemRefOf(kind, it))
		return err
	}, func(ok bool) {
		if ok {
			plainToast(i18n.T("Link copied"))
		}
	})
}
