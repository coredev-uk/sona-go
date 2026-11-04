package kit

import (
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

func act(id string, target ItemKind, g ActionGroup, order int) ActionDef {
	return ActionDef{
		ID: id, Target: target, Group: g, Order: order, Icon: "plus",
		Label: func(*ActionContext) string { return id }, Run: func(*ActionContext) {},
	}
}

func bulkAct(id string, g ActionGroup, order int, kinds ...ItemKind) ActionDef {
	d := act(id, KindSelection, g, order)
	d.Kinds = kinds
	return d
}

func ids(list []*ActionDef) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.ID
	}
	return out
}

func song(name string) Item   { return SongItem(services.Song{ID: "s-" + name, Name: name}) }
func album(name string) Item  { return AlbumItem(services.Album{ID: "a-" + name, Name: name}) }
func artist(name string) Item { return ArtistItem(services.Artist{ID: "r-" + name, Name: name}) }

func TestRegistryListsByGroupThenOrder(t *testing.T) {
	r := NewActionRegistry(
		act("rate", KindSong, GroupRating, 10),
		act("later", KindSong, GroupQueue, 20),
		act("item", KindSong, GroupItem, 5),
		act("first", KindSong, GroupQueue, 10),
	)
	if got := ids(r.For(KindSong)); !slices.Equal(got, []string{"first", "later", "item", "rate"}) {
		t.Errorf("got %v", got)
	}
}

func TestRegistryRejectsDuplicateIDsAcrossSingleAndBulk(t *testing.T) {
	r := NewActionRegistry(act("x", KindSong, GroupItem, 1))
	for _, d := range []ActionDef{act("x", KindAlbum, GroupItem, 1), bulkAct("x", GroupItem, 1, KindSong)} {
		func() {
			defer func() {
				if v := recover(); v != "action x registered twice" {
					t.Errorf("recovered %v", v)
				}
			}()
			r.Register(d)
		}()
	}
}

func TestRegistryForUnregisteredKindIsEmpty(t *testing.T) {
	r := NewActionRegistry(act("x", KindSong, GroupItem, 1))
	if got := r.For(KindStation); len(got) != 0 || r.Has(KindStation) {
		t.Errorf("got %v", got)
	}
}

func TestBulkForListsOnlyActionsAcceptingEveryKind(t *testing.T) {
	r := NewActionRegistry(
		bulkAct("queue.many", GroupQueue, 10, KindSong, KindAlbum, KindPlaylist),
		bulkAct("remove.many", GroupItem, 35, KindSong, KindAlbum, KindArtist),
		bulkAct("add.many", GroupItem, 30, KindSong, KindAlbum),
	)
	if got := ids(r.BulkFor([]Item{song("a"), song("b")})); !slices.Equal(got, []string{"queue.many", "add.many", "remove.many"}) {
		t.Errorf("songs: %v", got)
	}
	if got := ids(r.BulkFor([]Item{song("a"), album("b")})); !slices.Equal(got, []string{"queue.many", "add.many", "remove.many"}) {
		t.Errorf("songs+albums: %v", got)
	}
	if got := ids(r.BulkFor([]Item{album("a"), artist("b")})); !slices.Equal(got, []string{"remove.many"}) {
		t.Errorf("albums+artists: %v", got)
	}
}

func TestSectionsDropEmptyGroupsInOrder(t *testing.T) {
	r := NewActionRegistry(act("r", KindSong, GroupRating, 1), act("q", KindSong, GroupQueue, 1))
	secs := Sections(r.For(KindSong))
	if len(secs) != 2 || secs[0].Group != GroupQueue || secs[1].Group != GroupRating {
		t.Errorf("sections %+v", secs)
	}
	if Sections(nil) != nil {
		t.Error("no actions, no sections")
	}
}

func TestDescribeItems(t *testing.T) {
	cases := []struct {
		items []Item
		want  string
	}{
		{[]Item{song("Airbag")}, "“Airbag”"},
		{[]Item{song("a")}, "“a”"},
		{[]Item{song("a"), song("b"), song("c")}, "3 songs"},
		{[]Item{album("a"), album("b")}, "2 albums"},
		{[]Item{artist("a"), artist("b")}, "2 artists"},
		{[]Item{song("a"), album("b")}, "2 items"},
		{[]Item{StationItem(services.Station{Name: "s"}), StationItem(services.Station{Name: "t"})}, "2 stations"},
		{[]Item{PlaylistItem(services.Playlist{}), PlaylistItem(services.Playlist{})}, "2 playlists"},
		{[]Item{MusicVideoItem(services.MusicVideo{}), MusicVideoItem(services.MusicVideo{})}, "2 music videos"},
	}
	for _, c := range cases {
		if got := DescribeItems(c.items); got != c.want {
			t.Errorf("DescribeItems = %q, want %q", got, c.want)
		}
	}
}

func TestActionContextDataAndWithData(t *testing.T) {
	d := MenuData{ID: "rating"}
	ctx := NewActionRegistry().Context([]Item{song("a")}, nil, map[string]any{})
	if _, ok := DataOf[int](ctx, d); ok {
		t.Error("data is there before it loaded")
	}
	ctx = ctx.WithData(map[string]any{"rating": 1})
	if v, ok := DataOf[int](ctx, d); !ok || v != 1 {
		t.Errorf("DataOf = %v, %v", v, ok)
	}
}

func TestRegisterRejectsBrokenDefinitions(t *testing.T) {
	for name, d := range map[string]ActionDef{
		"no label":     {ID: "a", Target: KindSong, Run: func(*ActionContext) {}},
		"unknown kind": act("b", "podcast", GroupItem, 1),
		"bulk no kind": {ID: "c", Target: KindSelection, Label: func(*ActionContext) string { return "" }, Run: func(*ActionContext) {}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			NewActionRegistry(d)
		}()
	}
}

func TestItemAccessors(t *testing.T) {
	i := song("Airbag")
	if i.Name() != "Airbag" || i.ID() != "s-Airbag" || i.Song().Name != "Airbag" || i.Album().Name != "" {
		t.Errorf("accessors: %q %q", i.Name(), i.ID())
	}
}

// The ranking cases of v0.2.0's features/commands/rank.test.ts.
type cmd struct{ label, keywords string }

func rank(q string, items []cmd) []string {
	var out []string
	for _, c := range RankCommands(q, items, func(c cmd) string { return c.label }, func(c cmd) string { return c.keywords }) {
		out = append(out, c.label)
	}
	return out
}

var rankItems = []cmd{
	{"Show or hide the queue", "up next"},
	{"Next track", "skip forward"},
	{"Library · Albums", "library"},
	{"Albums of the year", ""},
	{"Play or pause", "resume stop"},
	{"Café playlists", ""},
}

func TestRankLabelStartBeforeContains(t *testing.T) {
	if got := rank("next", rankItems); !slices.Equal(got, []string{"Next track", "Show or hide the queue"}) {
		t.Errorf("got %v", got)
	}
}

func TestRankWordStartsAboveSubstringsAndKeywordsLast(t *testing.T) {
	if got := rank("alb", rankItems); !slices.Equal(got, []string{"Albums of the year", "Library · Albums"}) {
		t.Errorf("alb: %v", got)
	}
	if got := rank("skip", rankItems); !slices.Equal(got, []string{"Next track"}) {
		t.Errorf("skip: %v", got)
	}
}

func TestRankEveryTypedWordAtWordStartsInAnyOrder(t *testing.T) {
	if got := rank("hide queue", rankItems); !slices.Equal(got, []string{"Show or hide the queue"}) {
		t.Errorf("hide queue: %v", got)
	}
	if got := rank("pause play", rankItems); !slices.Equal(got, []string{"Play or pause"}) {
		t.Errorf("pause play: %v", got)
	}
}

func TestRankFoldsCaseAndAccents(t *testing.T) {
	if got := rank("CAFE", rankItems); !slices.Equal(got, []string{"Café playlists"}) {
		t.Errorf("got %v", got)
	}
}

func TestRankFallsBackToLettersInOrder(t *testing.T) {
	if MatchScore("ntr", "Next track", "") <= 0 {
		t.Error("ntr should match Next track")
	}
	if MatchScore("xyz", "Next track", "") != 0 {
		t.Error("xyz should not match")
	}
}

func TestRankEmptyQueryKeepsOrder(t *testing.T) {
	var want []string
	for _, c := range rankItems {
		want = append(want, c.label)
	}
	if got := rank("", rankItems); !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestChordPartsAndText(t *testing.T) {
	if got := strings.Join(ChordParts("Ctrl+ArrowLeft"), "|"); got != "Ctrl|←" {
		t.Errorf("parts %q", got)
	}
	if got := ChordText("Ctrl+ArrowLeft"); got != "Ctrl+Left Arrow" {
		t.Errorf("text %q", got)
	}
	if got := ChordParts("Escape"); got[0] != "Esc" {
		t.Errorf("escape %v", got)
	}
}

func TestTracksOfSongIsItselfAndOtherKindsHaveNone(t *testing.T) {
	got, err := TracksOf(t.Context(), nil, song("Airbag"))
	if err != nil || len(got) != 1 || got[0].Name != "Airbag" {
		t.Errorf("song: %v %v", got, err)
	}
	for _, it := range []Item{artist("a"), StationItem(services.Station{}), MusicVideoItem(services.MusicVideo{})} {
		if got, err := TracksOf(t.Context(), nil, it); err != nil || got != nil {
			t.Errorf("%s: %v %v", it.Kind, got, err)
		}
	}
}

// On macOS the Meta modifier is the Command key: ⌘ on the cap, "Command" to
// assistive technology; elsewhere it is Super.
func TestMetaIsCommandOnMacOnly(t *testing.T) {
	defer func(was bool) { metaIsCommand = was }(metaIsCommand)
	metaIsCommand = true
	if got := strings.Join(ChordParts("Meta+K"), "|"); got != "⌘|K" {
		t.Errorf("ChordParts on macOS = %q", got)
	}
	if got := ChordText("Meta+K"); got != "Command+K" {
		t.Errorf("ChordText on macOS = %q", got)
	}
	metaIsCommand = false
	if got := strings.Join(ChordParts("Meta+K"), "|"); got != "Super|K" {
		t.Errorf("ChordParts elsewhere = %q", got)
	}
}
