package playlist

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

const (
	// addMenuLevel is how many playlists and folders one level of the "Add
	// to playlist" submenu lists; the rest are in the picker, which has a
	// search field.
	addMenuLevel = 15
	// addMenuDepth is how many folders deep the submenu goes (a folder in a
	// folder is the second level); folders below are in the picker.
	addMenuDepth = 2
	// addRecentGrace is how long, once the tree is read, the menu waits for
	// Apple's recently used playlists (a network call) before it lists the
	// playlists in tree order.
	addRecentGrace = 300 * time.Millisecond
)

// addEntry is one row of the "Add to playlist" submenu: a playlist, a folder
// with the rows of its own submenu, or the row that opens the picker.
type addEntry struct {
	id, name string
	// sub are a folder's rows.
	sub []addEntry
	// recent marks a playlist Apple lists as recently added to; they lead
	// the top level.
	recent bool
	// more is "More playlists…": the picker, for what the level leaves out.
	more bool
}

func (e addEntry) folder() bool { return len(e.sub) > 0 }

// addMenu is what the "Add to playlist" submenu lists, worked out when the
// menu opens so the frames only wrap it.
type addMenu struct {
	// entries are the top level's rows; none when the user has no playlist
	// they can edit.
	entries []addEntry
	// failed: the tree could not be read; the action opens the picker, which
	// says so.
	failed bool
}

// addTargetsData is the playlists "Add to playlist" offers, loaded as the
// menu opens: the user's editable playlists and folders from SQLite, and
// Apple's recent targets when they come in time.
func (p *playlists) addTargetsData() kit.MenuData {
	return kit.MenuData{
		ID: "playlists.add-targets",
		Load: func(ctx context.Context, _ []kit.Item) any {
			recents := make(chan []string, 1)
			go func() {
				ids, err := p.api.RecentTargets(ctx)
				if err != nil {
					ids = nil
				}
				recents <- ids
			}()
			tree, err := p.api.Tree(ctx)
			if err != nil {
				return addMenu{failed: true}
			}
			var recent []string
			grace := time.NewTimer(addRecentGrace)
			defer grace.Stop()
			select {
			case recent = <-recents:
			case <-grace.C:
			case <-ctx.Done():
			}
			return buildAddMenu(tree, recent)
		},
	}
}

// buildAddMenu lays the editable playlists out as the submenu lists them.
// The top level leads with Apple's recent targets (at most five, as the
// picker's Recent), then the folders and playlists in the sidebar's order.
// A level of more than addMenuLevel rows keeps the first ones and ends with
// "More playlists…"; so does a folder at addMenuDepth that holds folders.
func buildAddMenu(tree services.PlaylistTree, recentIDs []string) addMenu {
	nodes := buildTree(tree, nil)
	recent, _ := addTargets(nodes, recentIDs, "")
	return addMenu{entries: addLevel(nodes, recent, 0)}
}

// addLevel is the rows of one level: depth is how many folders deep it is (0
// the top). recent are listed first and not again in the level.
func addLevel(nodes []*treeNode, recent []services.TreePlaylist, depth int) []addEntry {
	var out []addEntry
	skip := map[string]bool{}
	for _, tp := range recent {
		skip[tp.Playlist.ID] = true
		out = append(out, addEntry{id: tp.Playlist.ID, name: tp.Playlist.Name, recent: true})
	}
	more := false
	for _, n := range nodes {
		switch {
		case n.Item != nil:
			if n.Item.Playlist.CanEdit && !skip[n.Item.Playlist.ID] {
				out = append(out, addEntry{id: n.Item.Playlist.ID, name: n.Item.Playlist.Name})
			}
		case depth >= addMenuDepth:
			more = more || hasAddTargets(n.Children)
		default:
			if sub := addLevel(n.Children, nil, depth+1); len(sub) > 0 {
				out = append(out, addEntry{id: n.Folder.ID, name: n.Folder.Name, sub: sub})
			}
		}
	}
	if len(out) > addMenuLevel {
		out, more = out[:addMenuLevel], true
	}
	if more {
		out = append(out, addEntry{more: true})
	}
	return out
}

// hasAddTargets reports whether nodes hold a playlist the user can edit, in
// them or in a folder below.
func hasAddTargets(nodes []*treeNode) bool {
	for _, n := range nodes {
		if n.Item != nil && n.Item.Playlist.CanEdit || n.Folder != nil && hasAddTargets(n.Children) {
			return true
		}
	}
	return false
}

// addChoices is the submenu of "Add to playlist": New playlist… first, then
// the playlists. With no playlist to list (or none read yet) it lists at
// most the one row, and the action is the plain row that opens the picker.
func (p *playlists) addChoices(data kit.MenuData) func(a *kit.ActionContext) []kit.ActionChoice {
	return func(a *kit.ActionContext) []kit.ActionChoice {
		m, ok := kit.DataOf[addMenu](a, data)
		if !ok || len(m.entries) == 0 {
			return nil
		}
		out := []kit.ActionChoice{{Key: "new", Label: i18n.T("New playlist…"), Icon: "plus", Run: p.newPlaylistWith}}
		return append(out, p.addEntryChoices(m.entries, true)...)
	}
}

// addEntryChoices wraps entries as the menu's choices: a rule after "New
// playlist…" (top) and between the recent ones and the rest.
func (p *playlists) addEntryChoices(entries []addEntry, top bool) []kit.ActionChoice {
	out := make([]kit.ActionChoice, len(entries))
	for i, e := range entries {
		rule := e.more || i == 0 && top || i > 0 && entries[i-1].recent && !e.recent
		switch {
		case e.more:
			out[i] = kit.ActionChoice{Key: "more", Label: i18n.T("More playlists…"), Rule: rule, Run: p.pickPlaylist}
		case e.folder():
			out[i] = kit.ActionChoice{Key: "folder:" + e.id, Label: e.name, Icon: "folder-closed", Rule: rule, Sub: p.addEntryChoices(e.sub, false)}
		default:
			key := "in:"
			if e.recent {
				key = "recent:"
			}
			out[i] = kit.ActionChoice{Key: key + e.id, Label: e.name, Rule: rule, Run: func(a *kit.ActionContext) {
				p.addToPlaylistNow(e.id, e.name, playlistItems(a.Items), kit.DescribeItems(a.Items))
			}}
		}
	}
	return out
}

// pickPlaylist opens the picker for the items of a (the plain row, and "More
// playlists…").
func (p *playlists) pickPlaylist(a *kit.ActionContext) {
	p.addToPlaylist(playlistItems(a.Items), kit.DescribeItems(a.Items))
}

// newPlaylistWith opens the new playlist form, which makes it with the items
// of a in it.
func (p *playlists) newPlaylistWith(a *kit.ActionContext) {
	p.dialogs.show(dialogState{kind: dialogCreate, items: playlistItems(a.Items), label: kit.DescribeItems(a.Items)})
}
