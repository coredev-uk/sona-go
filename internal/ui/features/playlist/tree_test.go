package playlist

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// treePL builds a tree playlist; catalogID names Apple's personal mixes.
func treePL(id, name, folderID string, position int, canEdit bool, catalogID string) services.TreePlaylist {
	tp := treePlaylist(id, name, folderID, position, canEdit)
	tp.Playlist.CatalogID = catalogID
	return tp
}

func ownPL(id, name, folderID string, position int) services.TreePlaylist {
	return treePL(id, name, folderID, position, true, "")
}

// outline writes a tree as "Folder[child,child]" with playlist names;
// withHidden adds "+n" after a folder that left n playlists out.
func outline(nodes []*treeNode, withHidden bool) string {
	parts := make([]string, len(nodes))
	for i, n := range nodes {
		if n.Folder == nil {
			parts[i] = n.Item.Playlist.Name
			continue
		}
		hidden := ""
		if withHidden && n.Hidden > 0 {
			hidden = fmt.Sprintf("+%d", n.Hidden)
		}
		parts[i] = n.Folder.Name + hidden + "[" + outline(n.Children, withHidden) + "]"
	}
	return strings.Join(parts, ",")
}

func treeSample() services.PlaylistTree {
	return services.PlaylistTree{
		Folders: []services.PlaylistFolder{
			folder("f.old", "Old", "f.mix", 0),
			folder("f.mix", "Mixes", "", 1),
			folder("f.lost", "Orphan", "f.gone", 9),
		},
		Playlists: []services.TreePlaylist{
			ownPL("p.b", "Beta", "", 2),
			ownPL("p.a", "Alpha", "", 0),
			treePL("p.m", "Made for you", "", 3, false, ""),
			ownPL("p.in", "Inside", "f.mix", 1),
			ownPL("p.deep", "Deep", "f.old", 0),
		},
	}
}

func set(ids ...string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func TestBuildTreeOrder(t *testing.T) {
	nested := services.PlaylistTree{
		Folders: []services.PlaylistFolder{
			folder("f.top", "Top", "", 5),
			folder("f.b", "B", "f.top", 3),
			folder("f.a", "A", "f.top", 3),
			folder("f.empty", "Empty", "f.b", 2),
		},
		Playlists: []services.TreePlaylist{
			ownPL("p.1", "One", "f.top", 0),
			ownPL("p.2", "Two", "f.b", 1),
			ownPL("p.0", "Zero", "", 0),
		},
	}
	onlyFolders := services.PlaylistTree{
		Folders: []services.PlaylistFolder{
			folder("f.2", "Second", "", 2),
			folder("f.1", "First", "", 1),
		},
		Playlists: []services.TreePlaylist{ownPL("p.y", "Yankee", "f.1", 1), ownPL("p.x", "Xray", "f.1", 1)},
	}
	selfParent := services.PlaylistTree{
		Folders:   []services.PlaylistFolder{folder("f.me", "Me", "f.me", 0)},
		Playlists: []services.TreePlaylist{ownPL("p.in", "In", "f.me", 0)},
	}
	for _, c := range []struct {
		name string
		tree services.PlaylistTree
		want string
	}{
		{
			"folders before playlists, by position, unknown parents at the top", treeSample(),
			"Mixes[Old[Deep],Inside],Orphan[],Alpha,Beta,Made for you",
		},
		{"nested folders first too, empty folders kept", nested, "Top[A[],B[Empty[],Two],One],Zero"},
		{"ties by position fall back to the name", onlyFolders, "First[Xray,Yankee],Second[]"},
		{"empty tree", services.PlaylistTree{}, ""},
		{"a folder cannot be its own parent", selfParent, "Me[In]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := outline(buildTree(c.tree, nil), false); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestBuildTreePlaylistInUnknownFolderIsAtTheTop(t *testing.T) {
	tree := services.PlaylistTree{Playlists: []services.TreePlaylist{ownPL("p.x", "Stray", "f.gone", 0), ownPL("p.y", "Kept", "", 1)}}
	if got := outline(buildTree(tree, nil), false); got != "Stray,Kept" {
		t.Errorf("got %q", got)
	}
}

func TestBuildTreeHidden(t *testing.T) {
	running := services.PlaylistTree{
		Folders:   []services.PlaylistFolder{folder("f.run", "Running", "", 0)},
		Playlists: []services.TreePlaylist{ownPL("p.t", "Tempo", "f.run", 0), ownPL("p.l", "Long", "f.run", 1)},
	}
	lost := services.PlaylistTree{Playlists: []services.TreePlaylist{ownPL("p.x", "Stray", "f.gone", 0), ownPL("p.y", "Kept", "", 1)}}
	for _, c := range []struct {
		name   string
		tree   services.PlaylistTree
		hidden map[string]bool
		want   string
	}{
		{"every level, folders kept", treeSample(), set("p.a", "p.in", "p.deep"), "Mixes+1[Old+1[]],Orphan[],Beta,Made for you"},
		{"counts every hidden playlist of a folder", running, set("p.t", "p.l"), "Running+2[]"},
		{"counts only the hidden one", running, set("p.l"), "Running+1[Tempo]"},
		{"unpinned again is back in place", treeSample(), set(), "Mixes[Old[Deep],Inside],Orphan[],Alpha,Beta,Made for you"},
		{"IDs that are no playlist change nothing", treeSample(), set("l.album", "r.artist", "p.gone"), "Mixes[Old[Deep],Inside],Orphan[],Alpha,Beta,Made for you"},
		{"hidden playlist in an unknown folder is dropped", lost, set("p.x"), "Kept"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := outline(buildTree(c.tree, c.hidden), true); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestFolderPaths(t *testing.T) {
	var got []string
	for _, f := range folderPaths(buildTree(treeSample(), nil), "") {
		got = append(got, f.Path)
	}
	if want := []string{"Mixes", "Mixes / Old", "Orphan"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := folderPaths(buildTree(treeSample(), nil), "Root"); got[0].Path != "Root / Mixes" || got[0].Folder.ID != "f.mix" {
		t.Errorf("prefix not applied: %+v", got[0])
	}
}

func TestPlaylistSection(t *testing.T) {
	for _, c := range []struct {
		name      string
		catalogID string
		canEdit   bool
		want      section
	}{
		{"personal mix, not editable", "pl.pm-4c1f", false, sectionMadeForYou},
		{"personal mix, editable", "pl.pm-4c1f", true, sectionMadeForYou},
		{"editorial", "pl.f4d106fe", false, sectionApple},
		{"someone's shared playlist", "pl.u-8aZ", false, sectionApple},
		{"Replay is not a mix", "pl.rp-1", false, sectionApple},
		{"no catalogue ID", "", false, sectionApple},
		{"own", "", true, sectionOwn},
		{"collaborative own", "pl.u-8aZ", true, sectionOwn},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := playlistSection(services.Playlist{CatalogID: c.catalogID, CanEdit: c.canEdit}); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestSidebarPlaylists(t *testing.T) {
	library := services.PlaylistTree{
		Folders: []services.PlaylistFolder{folder("f.run", "Running", "", 0)},
		Playlists: []services.TreePlaylist{
			ownPL("p.own", "Mine", "", 5),
			treePL("p.fav", "Favourites Mix", "", 2, false, "pl.pm-1"),
			treePL("p.new", "New Music Mix", "", 1, false, "pl.pm-2"),
			treePL("p.zed", "Zen", "", 7, false, "pl.f4d"),
			treePL("p.abe", "Alt Rock", "f.run", 0, false, "pl.f4e"),
			ownPL("p.tempo", "Tempo", "f.run", 1),
		},
	}
	names := func(items []services.TreePlaylist) string {
		var s []string
		for _, it := range items {
			s = append(s, it.Playlist.Name)
		}
		return strings.Join(s, ",")
	}
	t.Run("three sections", func(t *testing.T) {
		s := sidebarPlaylists(library, nil)
		if got := names(s.MadeForYou); got != "New Music Mix,Favourites Mix" {
			t.Errorf("made for you %q (Apple's order)", got)
		}
		if got := names(s.Apple); got != "Alt Rock,Zen" {
			t.Errorf("apple %q (by name)", got)
		}
		if got := outline(s.Own, true); got != "Running+1[Tempo],Mine" {
			t.Errorf("own %q", got)
		}
	})
	t.Run("pinned is in no section", func(t *testing.T) {
		s := sidebarPlaylists(library, set("p.fav", "p.zed", "p.own", "p.tempo"))
		if got := names(s.MadeForYou); got != "New Music Mix" {
			t.Errorf("made for you %q", got)
		}
		if got := names(s.Apple); got != "Alt Rock" {
			t.Errorf("apple %q", got)
		}
		if got := outline(s.Own, true); got != "Running+2[]" {
			t.Errorf("own %q", got)
		}
	})
	t.Run("unpinned is back", func(t *testing.T) {
		if got := names(sidebarPlaylists(library, set("p.fav")).MadeForYou); got != "New Music Mix" {
			t.Errorf("pinned: %q", got)
		}
		if got := names(sidebarPlaylists(library, set()).MadeForYou); got != "New Music Mix,Favourites Mix" {
			t.Errorf("unpinned: %q", got)
		}
	})
	t.Run("empty sections", func(t *testing.T) {
		s := sidebarPlaylists(services.PlaylistTree{Playlists: []services.TreePlaylist{ownPL("p.own", "Mine", "", 0)}}, nil)
		if len(s.MadeForYou) != 0 || len(s.Apple) != 0 || outline(s.Own, true) != "Mine" {
			t.Errorf("got %+v", s)
		}
	})
}

func TestAddTargets(t *testing.T) {
	nodes := buildTree(treeSample(), nil)
	flat := func(groups []targetGroup) []string {
		var out []string
		for _, g := range groups {
			for _, it := range g.Items {
				out = append(out, g.Path+"|"+it.Playlist.Name)
			}
		}
		return out
	}
	ids := func(items []services.TreePlaylist) []string {
		var out []string
		for _, it := range items {
			out = append(out, it.Playlist.ID)
		}
		return out
	}
	t.Run("editable only, grouped by folder, recents first", func(t *testing.T) {
		recent, groups := addTargets(nodes, []string{"p.deep", "p.m", "p.unknown"}, "")
		if got := ids(recent); !slices.Equal(got, []string{"p.deep"}) {
			t.Errorf("recent %v (the non-editable and unknown ones are left out)", got)
		}
		want := []string{"|Alpha", "|Beta", "Mixes|Inside", "Mixes / Old|Deep"}
		if got := flat(groups); !slices.Equal(got, want) {
			t.Errorf("groups %v, want %v", got, want)
		}
	})
	t.Run("filter is case-insensitive, any part, trimmed, hides recents", func(t *testing.T) {
		recent, groups := addTargets(nodes, []string{"p.deep"}, "  E ")
		if len(recent) != 0 {
			t.Errorf("recent %v while filtering", ids(recent))
		}
		want := []string{"|Beta", "Mixes|Inside", "Mixes / Old|Deep"}
		if got := flat(groups); !slices.Equal(got, want) {
			t.Errorf("groups %v, want %v", got, want)
		}
	})
	t.Run("recents are at most five, in Apple's order", func(t *testing.T) {
		var pls []services.TreePlaylist
		var order []string
		for i := range 8 {
			id := fmt.Sprintf("p.%d", i)
			pls = append(pls, ownPL(id, fmt.Sprintf("List %d", i), "", i))
			order = append([]string{id}, order...)
		}
		recent, _ := addTargets(buildTree(services.PlaylistTree{Playlists: pls}, nil), order, "")
		if got, want := ids(recent), order[:5]; !slices.Equal(got, want) {
			t.Errorf("recent %v, want %v", got, want)
		}
	})
	t.Run("nothing matches", func(t *testing.T) {
		recent, groups := addTargets(nodes, nil, "zzz")
		if len(recent) != 0 || len(groups) != 0 {
			t.Errorf("got %v %v", recent, groups)
		}
	})
}
