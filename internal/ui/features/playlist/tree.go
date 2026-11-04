package playlist

import (
	"slices"
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// treeNode is a folder (Folder set, with its Children) or a playlist (Item
// set) of the sidebar's tree (tree.ts's TreeNode).
type treeNode struct {
	Folder   *services.PlaylistFolder
	Item     *services.TreePlaylist
	Children []*treeNode
	// Hidden is how many of a folder's playlists are left out because the
	// sidebar lists them elsewhere.
	Hidden int
}

func (n *treeNode) position() int {
	if n.Folder != nil {
		return n.Folder.Position
	}
	return n.Item.Position
}

func (n *treeNode) name() string {
	if n.Folder != nil {
		return n.Folder.Name
	}
	return n.Item.Playlist.Name
}

// section is the sidebar section a library playlist is listed in (unless
// it is pinned).
type section int

const (
	sectionMadeForYou section = iota
	sectionApple
	sectionOwn
)

// personalMix is the catalogue ID prefix of Apple's personal mixes
// (Favourites Mix, New Music Mix, Chill Mix…).
const personalMix = "pl.pm-"

// playlistSection is where a library playlist belongs. Apple's personal
// mixes are catalogue playlists `pl.pm-…`, so the library copy names one by
// its catalogue ID. Any other playlist the user cannot edit was added from
// Apple Music (or another curator); the rest are the user's own.
func playlistSection(p services.Playlist) section {
	if strings.HasPrefix(p.CatalogID, personalMix) {
		return sectionMadeForYou
	}
	if p.CanEdit {
		return sectionOwn
	}
	return sectionApple
}

func compareNames(a, b string) int { return i18n.Collator().CompareString(a, b) }

// buildTree is the folder tree: at every level folders come first, then
// playlists, each group in Apple's position order (then by name). Folders
// whose parent is unknown, and playlists in an unknown folder, sit at the
// top level. Playlists in hidden (library IDs) are left out, as the sidebar
// lists them in another section; their folders stay, counting them.
func buildTree(tree services.PlaylistTree, hidden map[string]bool) []*treeNode {
	folders := make(map[string]*treeNode, len(tree.Folders))
	for i := range tree.Folders {
		f := &tree.Folders[i]
		folders[f.ID] = &treeNode{Folder: f}
	}
	var root []*treeNode
	parentList := func(id string) *[]*treeNode {
		if f := folders[id]; id != "" && f != nil {
			return &f.Children
		}
		return &root
	}
	// Folder order of the input, so the result is deterministic.
	for i := range tree.Folders {
		f := &tree.Folders[i]
		node := folders[f.ID]
		// A folder cannot hold itself (guards bad data from a cycle).
		if f.ParentID == f.ID {
			root = append(root, node)
		} else {
			l := parentList(f.ParentID)
			*l = append(*l, node)
		}
	}
	for i := range tree.Playlists {
		item := &tree.Playlists[i]
		if !hidden[item.Playlist.ID] {
			l := parentList(item.FolderID)
			*l = append(*l, &treeNode{Item: item})
		} else if f := folders[item.FolderID]; f != nil {
			f.Hidden++
		}
	}
	var sortList func([]*treeNode)
	sortList = func(list []*treeNode) {
		slices.SortStableFunc(list, func(a, b *treeNode) int {
			if (a.Folder != nil) != (b.Folder != nil) {
				if a.Folder != nil {
					return -1
				}
				return 1
			}
			if d := a.position() - b.position(); d != 0 {
				return d
			}
			return compareNames(a.name(), b.name())
		})
		for _, n := range list {
			if n.Folder != nil {
				sortList(n.Children)
			}
		}
	}
	sortList(root)
	return root
}

// sidebarLists are the sidebar's playlist sections.
type sidebarLists struct {
	// MadeForYou are Apple's personal mixes, in Apple's order.
	MadeForYou []services.TreePlaylist
	// Apple are the playlists the user added but cannot edit, by name.
	Apple []services.TreePlaylist
	// Own are the user's own playlists and folders.
	Own []*treeNode
}

// sidebarPlaylists splits the tree into the sidebar's sections. A pinned
// playlist (library IDs in pinned) is in none of them: it shows once, under
// Pinned, whatever its section. Folders keep to the user's own playlists; a
// folder's non-editable playlists are listed in their section and counted
// as left out of the folder.
func sidebarPlaylists(tree services.PlaylistTree, pinned map[string]bool) sidebarLists {
	var out sidebarLists
	hidden := make(map[string]bool, len(pinned))
	for id := range pinned {
		hidden[id] = true
	}
	for _, item := range tree.Playlists {
		sec := playlistSection(item.Playlist)
		if sec == sectionOwn {
			continue
		}
		hidden[item.Playlist.ID] = true
		if pinned[item.Playlist.ID] {
			continue
		}
		if sec == sectionMadeForYou {
			out.MadeForYou = append(out.MadeForYou, item)
		} else {
			out.Apple = append(out.Apple, item)
		}
	}
	slices.SortStableFunc(out.MadeForYou, func(a, b services.TreePlaylist) int {
		if d := a.Position - b.Position; d != 0 {
			return d
		}
		return compareNames(a.Playlist.Name, b.Playlist.Name)
	})
	slices.SortStableFunc(out.Apple, func(a, b services.TreePlaylist) int {
		return compareNames(a.Playlist.Name, b.Playlist.Name)
	})
	out.Own = buildTree(tree, hidden)
	return out
}

// folderPath is a folder with its path ("Mixes / Old").
type folderPath struct {
	Folder services.PlaylistFolder
	Path   string
}

// folderPaths lists every folder with its path, depth-first in tree order.
func folderPaths(nodes []*treeNode, prefix string) []folderPath {
	var out []folderPath
	for _, n := range nodes {
		if n.Folder == nil {
			continue
		}
		path := n.Folder.Name
		if prefix != "" {
			path = prefix + " / " + n.Folder.Name
		}
		out = append(out, folderPath{*n.Folder, path})
		out = append(out, folderPaths(n.Children, path)...)
	}
	return out
}

// targetGroup is the playlists of one folder, as the add dialog lists them.
type targetGroup struct {
	// Path is "" for the top level, else the folder path.
	Path  string
	Items []services.TreePlaylist
}

// addTargets are the playlists songs can be added to (editable ones),
// grouped by folder in tree order and filtered by query (case-insensitive,
// any part of the name), plus Apple's recently used ones (at most 5, in
// Apple's order) first when not filtering.
func addTargets(nodes []*treeNode, recentIDs []string, query string) (recent []services.TreePlaylist, groups []targetGroup) {
	q := strings.ToLower(strings.TrimSpace(query))
	match := func(p *services.TreePlaylist) bool {
		return p.Playlist.CanEdit && (q == "" || strings.Contains(strings.ToLower(p.Playlist.Name), q))
	}
	var walk func(list []*treeNode, path string)
	walk = func(list []*treeNode, path string) {
		var items []services.TreePlaylist
		for _, n := range list {
			if n.Item != nil && match(n.Item) {
				items = append(items, *n.Item)
			}
		}
		if len(items) > 0 {
			groups = append(groups, targetGroup{path, items})
		}
		for _, n := range list {
			if n.Folder != nil {
				p := n.Folder.Name
				if path != "" {
					p = path + " / " + n.Folder.Name
				}
				walk(n.Children, p)
			}
		}
	}
	walk(nodes, "")
	if q == "" {
		byID := map[string]services.TreePlaylist{}
		for _, g := range groups {
			for _, it := range g.Items {
				byID[it.Playlist.ID] = it
			}
		}
		for _, id := range recentIDs {
			if it, ok := byID[id]; ok {
				recent = append(recent, it)
			}
		}
		recent = recent[:min(len(recent), 5)]
	}
	return recent, groups
}
