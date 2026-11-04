package library

import (
	"context"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// itemStateKey is the query key of an item's state (v0.2.0's
// keys.itemState): ["library","state",kind,id].
func itemStateKey(kind, id string) store.Key { return store.Key{"library", "state", kind, id} }

// isUpcoming is v0.2.0's shared/lib/release.ts: an album is still to come
// when it is prerelease and has no full release date, or that date is ahead.
func isUpcoming(a services.Album, now time.Time) bool {
	return a.Prerelease && (a.ReleaseAt == 0 || now.UnixMilli() < a.ReleaseAt)
}

// actions are the library's item actions (v0.2.0's features/library/
// actions.ts): add to or remove from the library, Favourite, Suggest less
// and Copy link, for songs, albums, artists and playlists, the same for a
// selection, and Copy link for stations. Their labels and availability read
// the item's library state, loaded (batched) when the menu opens.
func (l *library) actions() []kit.ActionDef {
	st := newItemStates(l.env.Queries, func() itemStateService { return l.lib })
	l.states = st
	var defs []kit.ActionDef
	defs = append(defs, l.itemActions(st, kit.KindSong, stateSongs, true, true)...)
	defs = append(defs, l.itemActions(st, kit.KindAlbum, stateAlbums, true, true)...)
	defs = append(defs, l.itemActions(st, kit.KindArtist, stateArtists, false, true)...)
	defs = append(defs, l.itemActions(st, kit.KindPlaylist, statePlaylists, true, false)...)
	defs = append(defs, kit.ActionDef{
		ID:     "library.copy-link.station",
		Target: kit.KindStation,
		Label:  func(*kit.ActionContext) string { return i18n.T("Copy link") },
		Icon:   "link-2",
		Group:  kit.GroupItem,
		Order:  90,
		Run: func(a *kit.ActionContext) {
			s := a.Item().Station()
			l.copyLink("stations", stateItem{ID: s.ID, Source: services.SourceCatalog})
		},
	})
	return append(defs, l.bulkActions(st)...)
}

// loadedState is what a single action's menu data read: the item's state,
// nil until loaded (or when it could not be had).
func loadedState(a *kit.ActionContext, d kit.MenuData) (services.ItemState, bool) {
	return kit.DataOf[services.ItemState](a, d)
}

// itemActions are the actions of one target kind: add (catalogue items) and
// remove (library items) are offered as given.
func (l *library) itemActions(st *itemStates, target kit.ItemKind, kind string, add, remove bool) []kit.ActionDef {
	state := kit.MenuData{
		ID: "library.state." + string(target),
		Load: func(ctx context.Context, items []kit.Item) any {
			_, si, ok := stateOfItem(items[0])
			if !ok {
				return nil
			}
			if s, ok := st.Load(ctx, kind, si); ok {
				return s
			}
			return nil
		},
	}
	// ratingOf is the item's rating, 0 until its state loads.
	ratingOf := func(a *kit.ActionContext) int {
		s, _ := loadedState(a, state)
		return s.Rating
	}
	loaded := func(a *kit.ActionContext) bool {
		_, ok := loadedState(a, state)
		return ok
	}
	// inLibrary: the loaded state, else what the item's source says.
	inLibrary := func(a *kit.ActionContext) bool {
		if s, ok := loadedState(a, state); ok {
			return s.InLibrary
		}
		_, si, _ := stateOfItem(a.Item())
		return si.Source == services.SourceLibrary
	}
	item := func(a *kit.ActionContext) (stateItem, string) {
		_, si, _ := stateOfItem(a.Item())
		return si, a.Item().Name()
	}
	rate := func(on, off kit.RatingAction, rating int) func(*kit.ActionContext) {
		return func(a *kit.ActionContext) {
			si, name := item(a)
			act := on
			if ratingOf(a) == rating {
				act = off
			}
			st.Rate(kind, si, name, act)
		}
	}
	defs := []kit.ActionDef{
		{
			ID:     "library.favorite." + string(target),
			Target: target,
			Label: func(a *kit.ActionContext) string {
				if ratingOf(a) == 1 {
					return i18n.T("Undo favourite")
				}
				return i18n.T("Favourite")
			},
			Icon:    "star",
			Filled:  func(a *kit.ActionContext) bool { return ratingOf(a) == 1 },
			Group:   kit.GroupRating,
			Order:   10,
			Data:    []kit.MenuData{state},
			Enabled: loaded,
			Run:     rate(kit.RateFavorite, kit.RateUnfavorite, 1),
		},
		{
			ID:     "library.suggest-less." + string(target),
			Target: target,
			Label: func(a *kit.ActionContext) string {
				if ratingOf(a) == -1 {
					return i18n.T("Undo suggest less")
				}
				return i18n.T("Suggest less")
			},
			Icon:    "thumbs-down",
			Filled:  func(a *kit.ActionContext) bool { return ratingOf(a) == -1 },
			Group:   kit.GroupRating,
			Order:   20,
			Data:    []kit.MenuData{state},
			Enabled: loaded,
			Run:     rate(kit.RateSuggestLess, kit.RateUndoSuggestLess, -1),
		},
		{
			ID:     "library.copy-link." + string(target),
			Target: target,
			Label:  func(*kit.ActionContext) string { return i18n.T("Copy link") },
			Icon:   "link-2",
			Group:  kit.GroupItem,
			Order:  90,
			Available: func(a *kit.ActionContext) bool {
				si, _ := item(a)
				return si.Source == services.SourceCatalog || si.CatalogID != ""
			},
			Run: func(a *kit.ActionContext) {
				si, _ := item(a)
				l.copyLink(kind, si)
			},
		},
	}
	if add {
		defs = append(defs, kit.ActionDef{
			ID:     "library.add." + string(target),
			Target: target,
			// An album not out yet is pre-added: the same call (Apple's web
			// player's too).
			Label: func(a *kit.ActionContext) string {
				if target == kit.KindAlbum && isUpcoming(a.Item().Album(), time.Now()) {
					return i18n.T("Pre-add to library")
				}
				return i18n.T("Add to library")
			},
			Icon:  "plus",
			Group: kit.GroupItem,
			Order: 30,
			Data:  []kit.MenuData{state},
			Available: func(a *kit.ActionContext) bool {
				si, _ := item(a)
				return si.Source == services.SourceCatalog && !inLibrary(a)
			},
			Enabled: loaded,
			Run: func(a *kit.ActionContext) {
				si, name := item(a)
				st.AddOne(kind, si, name)
			},
		})
	}
	if remove {
		defs = append(defs, kit.ActionDef{
			ID:        "library.remove." + string(target),
			Target:    target,
			Label:     func(*kit.ActionContext) string { return i18n.T("Delete from library") },
			Icon:      "trash-2",
			Group:     kit.GroupItem,
			Order:     35,
			Data:      []kit.MenuData{state},
			Available: inLibrary,
			Run: func(a *kit.ActionContext) {
				si, name := item(a)
				st.RemoveOne(kind, si, name)
			},
		})
	}
	return defs
}

// kinded is the selection with each item's state kind (stations and music
// videos have none here).
func kinded(items []kit.Item) []kindedItem {
	var out []kindedItem
	for _, it := range items {
		if it.Kind == kit.KindStation || it.Kind == kit.KindMusicVideo {
			continue
		}
		if kind, si, ok := stateOfItem(it); ok {
			out = append(out, kindedItem{kind, si})
		}
	}
	return out
}

// bulkActions are the `targets[]` forms of the library actions: Favourite,
// add to and remove from the library for a selection. Their labels and
// availability read every selected item's state (batched into one Go call
// when the menu opens).
func (l *library) bulkActions(st *itemStates) []kit.ActionDef {
	states := kit.MenuData{
		ID: "library.state.many",
		Load: func(ctx context.Context, items []kit.Item) any {
			list := kinded(items)
			out := make([]*services.ItemState, len(list))
			var wg sync.WaitGroup
			for i, k := range list {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if s, ok := st.Load(ctx, k.Kind, k.Item); ok {
						out[i] = &s
					}
				}()
			}
			wg.Wait()
			return out
		},
	}
	// data is every selected item's state: nil elements have none; ok is
	// false until the data loads.
	data := func(a *kit.ActionContext) ([]*services.ItemState, bool) {
		return kit.DataOf[[]*services.ItemState](a, states)
	}
	stateAt := func(s []*services.ItemState, i int) *services.ItemState {
		if i < len(s) {
			return s[i]
		}
		return nil
	}
	inLibrary := func(a *kit.ActionContext) bool {
		s, _ := data(a)
		for i, k := range kinded(a.Items) {
			if x := stateAt(s, i); x != nil {
				if !x.InLibrary {
					return false
				}
			} else if k.Item.Source != services.SourceLibrary {
				return false
			}
		}
		return true
	}
	// notInLibrary are the catalogue items not (yet) in the library.
	notInLibrary := func(a *kit.ActionContext) []kindedItem {
		s, _ := data(a)
		var out []kindedItem
		for i, k := range kinded(a.Items) {
			if x := stateAt(s, i); k.Item.Source == services.SourceCatalog && (x == nil || !x.InLibrary) {
				out = append(out, k)
			}
		}
		return out
	}
	allFavourite := func(a *kit.ActionContext) bool {
		s, _ := data(a)
		if len(s) == 0 {
			return false
		}
		for _, x := range s {
			if x == nil || x.Rating != 1 {
				return false
			}
		}
		return true
	}
	loaded := func(a *kit.ActionContext) bool {
		_, ok := data(a)
		return ok
	}
	return []kit.ActionDef{
		{
			ID:     "library.favorite.many",
			Target: kit.KindSelection,
			Kinds:  []kit.ItemKind{kit.KindSong, kit.KindAlbum, kit.KindArtist, kit.KindPlaylist},
			Label: func(a *kit.ActionContext) string {
				if allFavourite(a) {
					return i18n.T("Undo favourite")
				}
				return i18n.T("Favourite")
			},
			Icon:    "star",
			Filled:  allFavourite,
			Group:   kit.GroupRating,
			Order:   10,
			Data:    []kit.MenuData{states},
			Enabled: loaded,
			Run: func(a *kit.ActionContext) {
				undo := allFavourite(a)
				// Favouriting skips items that already are; undoing applies
				// to all.
				s, _ := data(a)
				var targets []kindedItem
				for i, k := range kinded(a.Items) {
					if x := stateAt(s, i); undo || x == nil || x.Rating != 1 {
						targets = append(targets, k)
					}
				}
				act := kit.RateFavorite
				if undo {
					act = kit.RateUnfavorite
				}
				st.RateMany(targets, act, kit.DescribeItems(a.Items))
			},
		},
		{
			ID:     "library.add.many",
			Target: kit.KindSelection,
			Kinds:  []kit.ItemKind{kit.KindSong, kit.KindAlbum, kit.KindPlaylist},
			Label:  func(*kit.ActionContext) string { return i18n.T("Add to library") },
			Icon:   "plus",
			Group:  kit.GroupItem,
			Order:  30,
			Data:   []kit.MenuData{states},
			Available: func(a *kit.ActionContext) bool {
				return len(notInLibrary(a)) > 0
			},
			Enabled: loaded,
			Run: func(a *kit.ActionContext) {
				st.AddMany(notInLibrary(a), kit.DescribeItems(a.Items))
			},
		},
		{
			ID:        "library.remove.many",
			Target:    kit.KindSelection,
			Kinds:     []kit.ItemKind{kit.KindSong, kit.KindAlbum, kit.KindArtist},
			Label:     func(*kit.ActionContext) string { return i18n.T("Delete from library") },
			Icon:      "trash-2",
			Group:     kit.GroupItem,
			Order:     35,
			Data:      []kit.MenuData{states},
			Available: inLibrary,
			Run: func(a *kit.ActionContext) {
				st.RemoveMany(kinded(a.Items), kit.DescribeItems(a.Items))
			},
		},
	}
}
