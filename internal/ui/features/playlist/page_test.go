package playlist

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func TestPageShowsHeaderMetaAndByline(t *testing.T) {
	d := libraryDetail("p.1", "Road Trip", threeSongs()...)
	d.Playlist.Description = "Songs for the drive."
	d.Curator = services.PlaylistCurator{ID: "ac.1", Kind: "apple-curator", Name: "Apple Music Indie"}
	r := newPageRig(t, "p.1", d)
	r.shows("Road Trip", "Songs for the drive.", "Playlist · 3 songs, 10 min", "Apple Music Indie", "Songs in Road Trip", "Song A", "Artist B", "Album C")
	// The byline is a link to the curator's page.
	r.click("Apple Music Indie")
	if got := r.router.Path(); got != "/curator/ac.1" {
		t.Errorf("the byline went to %q", got)
	}
}

func TestPageBylineLinksProfilesAndFallsBackToText(t *testing.T) {
	d := libraryDetail("p.1", "Road Trip", threeSongs()...)
	d.Curator = services.PlaylistCurator{ID: "sp.9", Kind: "profile", Name: "Robin"}
	r := newPageRig(t, "p.1", d)
	r.shows("Robin")
	r.click("Robin")
	if got := r.router.Path(); got != "/profile/sp.9" {
		t.Errorf("the profile byline went to %q", got)
	}

	// A page Sona does not have is plain text.
	r = newPageRig(t, "p.1", d)
	r.linked = false
	r.shows("Robin")
	r.click("Robin")
	if got := r.router.Path(); got != "/playlist/p.1" {
		t.Errorf("an unlinked byline went to %q", got)
	}

	// With no curator record the playlist's own curator name shows.
	d.Curator = services.PlaylistCurator{}
	d.Playlist.Curator = "Fallback Curator"
	r = newPageRig(t, "p.1", d)
	r.shows("Fallback Curator")
}

func TestPageMetaLineKindsAndUpdated(t *testing.T) {
	p := &playlists{}
	d := libraryDetail("p.1", "Mix", threeSongs()...)
	for _, c := range []struct {
		name string
		mod  func(*services.PlaylistDetail)
		want string
	}{
		{"plain", func(*services.PlaylistDetail) {}, "Playlist · 3 songs, 10 min"},
		{"mix", func(d *services.PlaylistDetail) { d.Kind = "personal-mix" }, "Made for you · 3 songs, 10 min"},
		{"chart", func(d *services.PlaylistDetail) { d.IsChart = true }, "Chart · 3 songs, 10 min"},
		{"favourites", func(d *services.PlaylistDetail) { d.Favorited = true }, "Your favourites · 3 songs, 10 min"},
		{"empty", func(d *services.PlaylistDetail) { d.Tracks, d.DurationMs = nil, 0 }, "Playlist"},
	} {
		dd := d
		c.mod(&dd)
		if got := p.metaLine(dd, nil); got != c.want {
			t.Errorf("%s: meta %q, want %q", c.name, got, c.want)
		}
	}

	collab := &services.PlaylistCollaboration{Collaborative: true, Viewer: services.RoleCollaborator}
	d.Playlist.LastModified = 0
	if got := p.metaLine(d, collab); got != "Collaborative playlist · You're a collaborator · 3 songs, 10 min" {
		t.Errorf("collaborative meta %q", got)
	}
}

// "Updated …" is in calendar words, with the exact date and time for a
// tooltip and a description; it is a tab stop, so the keyboard reaches the
// tooltip, and it changes at midnight on a page left open.
func TestPageUpdatedReadsInCalendarWordsWithTheExactDate(t *testing.T) {
	now := time.Date(2026, 11, 10, 23, 58, 0, 0, time.Local) // Tuesday
	clock := now
	for _, c := range []struct {
		name string
		mod  time.Time
		want string
	}{
		{"today", now.Add(-3 * time.Hour), "Updated 3 hr. ago"},
		{"yesterday", time.Date(2026, 11, 9, 21, 30, 0, 0, time.Local), "Updated yesterday"},
		{"this week", time.Date(2026, 11, 6, 9, 0, 0, 0, time.Local), "Updated on Friday"},
		{"last spring", time.Date(2026, 4, 2, 9, 0, 0, 0, time.Local), "Updated in April"},
		{"years ago", time.Date(2023, 4, 2, 9, 0, 0, 0, time.Local), "Updated in 2023"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := libraryDetail("p.1", "Road Trip", threeSongs()...)
			d.Playlist.LastModified = c.mod.UnixMilli()
			r := newPageRigWith(t, "p.1", func(*fakeAPI) {}, d)
			r.p.now = func() time.Time { return clock }
			r.tt.Frame()
			r.shows("Playlist · 3 songs, 10 min", c.want)
			exact := i18n.FormatDateTime(c.mod)
			tr := access.Read(t, r.tt.Tester)
			n, ok := tr.Find(ui.RoleText, c.want)
			if !ok || n.Description != exact {
				t.Errorf("the note %q is described %q, want %q:\n%s", c.want, n.Description, exact, tr.Dump())
			}
		})
	}

	// The keyboard stops at it, after the title and before the actions.
	d := libraryDetail("p.1", "Road Trip", threeSongs()...)
	d.Playlist.LastModified = now.Add(-time.Hour).UnixMilli()
	r := newPageRig(t, "p.1", d)
	r.p.now = func() time.Time { return clock }
	r.tt.Frame()
	r.shows("Updated 1 hr. ago")
	stops := stopLabels(access.TabWalk(t, r.tt.Tester, 60))
	if !inOrder(stops, []string{"Updated 1 hr. ago", "Play Road Trip"}) {
		t.Errorf("Tab walk %q lacks the note before Play", stops)
	}

	// Left open across midnight, the page relabels itself: a playlist changed
	// yesterday reads "on Monday" once the day turns, with no frame asked for
	// by the test (HasText does not draw), so only the page's own wake at
	// midnight can change it.
	d = libraryDetail("p.1", "Road Trip", threeSongs()...)
	d.Playlist.LastModified = time.Date(2026, 11, 9, 21, 30, 0, 0, time.Local).UnixMilli()
	r = newPageRig(t, "p.1", d)
	r.p.now = func() time.Time { return clock }
	r.tt.Frame()
	r.shows("Updated yesterday")
	clock = now.Add(5 * time.Minute)
	r.tt.Advance(5 * time.Minute)
	if !r.tt.HasText("Updated on Monday") {
		t.Errorf("after midnight the page reads %q", r.tt.Texts())
	}
}

func TestPageLoadingShowsTheSkeletonAndFailureOffersRetry(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.tt.Frame()
	if !r.tt.HasText("Loading") {
		t.Errorf("no loading skeleton: %q", r.tt.Texts())
	}
	r.shows("Road Trip")
	if r.tt.HasText("Loading") {
		t.Error("the skeleton stayed")
	}

	r = newPageRigWith(t, "p.1", func(api *fakeAPI) { api.failReads = true }, libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Couldn't load this playlist", "Try again")
	r.api.mu.Lock()
	r.api.failReads = false
	r.api.mu.Unlock()
	r.click("Try again")
	r.shows("Road Trip", "Song A")
	if r.tt.HasText("Couldn't load this playlist") {
		t.Error("the error stayed after a retry that worked")
	}
}

func TestPageNotFoundLinksBackToThePlaylists(t *testing.T) {
	r := newPageRig(t, "p.missing")
	r.shows("This playlist isn't available", "Go to Playlists")
	r.click("Go to Playlists")
	if got := r.router.Path(); got != "/library/playlists" {
		t.Errorf("the back link went to %q", got)
	}

	// Without a Playlists page there is no link to it.
	r = newPageRig(t, "p.missing")
	r.linked = false
	r.shows("This playlist isn't available")
	if r.tt.HasText("Go to Playlists") {
		t.Error("a link to a page that does not exist")
	}
}

func TestPagePlayAndShufflePlayThePlaylist(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Song A")
	r.click("Play Road Trip")
	r.tt.Until("the play", func() bool { return len(r.play.played()) == 1 })
	if got := r.play.played()[0]; got != "[a b c] start=0 from=playlist:p.1" {
		t.Errorf("play: %q", got)
	}

	r.click("Shuffle")
	r.tt.Until("the shuffle", func() bool { return len(r.play.played()) == 2 })
	got := r.play.played()[1]
	if !strings.HasPrefix(got, "[a b c] start=") || !strings.HasSuffix(got, " from=playlist:p.1") {
		t.Errorf("shuffle: %q", got)
	}
	if !r.play.shuffled() {
		t.Error("shuffle did not switch shuffle on")
	}
}

func TestPageRowActivationPlaysFromThatRow(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Song B")
	r.click("Song B")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the play", func() bool { return len(r.play.played()) == 1 })
	if got := r.play.played()[0]; got != "[a b c] start=1 from=playlist:p.1" {
		t.Errorf("row play: %q", got)
	}
}

func TestPagePlayIsDisabledWithNothingPlayable(t *testing.T) {
	songs := threeSongs()
	for i := range songs {
		songs[i].Playable = false
	}
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", songs...))
	r.shows("Song A")
	_ = r.tt.Click("Play Road Trip")
	r.tt.Frame()
	if n := len(r.play.played()); n != 0 {
		t.Errorf("a disabled Play played %d times", n)
	}
}

func TestPageHeaderOfAnEditablePlaylistHasEditRatingsAndMenu(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Edit", "Suggest less like Road Trip", "More for Road Trip")
	// Its rows carry the playlist as their container.
	r.api.mu.Lock()
	r.api.suggestions["p.1"] = nil
	r.api.mu.Unlock()
	// The rating asks the service once the state has loaded.
	r.tt.Until("the favourite", func() bool {
		_ = r.tt.Click("Favourite Road Trip")
		return slices.Contains(r.states.asked(), "Favorite p.1")
	})
}

func TestPageCatalogPlaylistHasLibraryButtonAndNoEdit(t *testing.T) {
	d := catalogDetail("pl.1", "Apple's Mix", threeSongs()...)
	r := newPageRig(t, "pl.1", d)
	r.shows("Apple's Mix", "Add", "Song A")
	if _, ok := r.tt.Find("Edit"); ok {
		t.Error("a playlist nobody can edit offers Edit")
	}
	if n := len(r.calls("Suggestions")); n != 0 {
		t.Errorf("suggestions asked for a catalogue playlist: %v", r.calls("Suggestions"))
	}
	r.tt.Until("the add", func() bool {
		_ = r.tt.Click("Add")
		return slices.Contains(r.states.asked(), "Add playlists pl.1")
	})
}

func TestPageEmptyStates(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip"))
	r.shows("This playlist is empty", "Add songs with “Add to playlist” in any song, album or playlist menu, or pick from the suggestions below.")

	r = newPageRig(t, "pl.1", catalogDetail("pl.1", "Apple's Mix"))
	r.shows("This playlist is empty", "Songs added to it in Apple Music on any device appear here after the next sync.")
}

func TestPageEditModeSwapsPlayForDoneAndReturnsFocus(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Songs in Road Trip")
	r.click("Edit")
	r.shows("Done", "Name and description", "Edit the songs in Road Trip")
	for _, gone := range []string{"Play Road Trip", "Shuffle", "Songs in Road Trip", "Suggested songs"} {
		if r.tt.HasText(gone) {
			t.Errorf("edit mode still shows %q", gone)
		}
	}
	if !r.tt.Focused("Done") {
		t.Error("the focus did not move to Done")
	}

	r.click("Name and description")
	if !r.p.dialogs.open || r.p.dialogs.s.kind != dialogDetails || r.p.dialogs.s.id != "p.1" || r.p.dialogs.s.name != "Road Trip" {
		t.Errorf("details dialog: open=%v %+v", r.p.dialogs.open, r.p.dialogs.s)
	}
	r.p.dialogs.open = false
	r.tt.Frame()

	r.click("Done")
	r.shows("Shuffle", "Songs in Road Trip")
	if r.tt.HasText("Done") {
		t.Error("Done stayed")
	}
	if !r.tt.Focused("Edit") {
		t.Error("the focus did not move back to Edit")
	}
}

func TestPageEditModeWithNoSongsSaysNothingIsLeft(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip"))
	r.shows("This playlist is empty")
	r.click("Edit")
	r.shows("Nothing left to arrange", "Add songs with “Add to playlist” in any song, album or playlist menu.")
}

func TestPageSuggestionsAddAtOnceAndSerialise(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.api.suggestions["p.1"] = []services.Song{
		pgSong("s1", "Sugg One", "Singer", "Album S"),
		pgSong("s2", "Sugg Two", "Singer", "Album S"),
	}
	r.shows("Suggested songs", "Sugg One", "Singer · 3:20", "Add Sugg One to Road Trip", "Play Sugg One")

	// Art plays that song.
	r.click("Play Sugg Two")
	r.tt.Until("the song", func() bool { return len(r.play.played()) == 1 })
	if got := r.play.played()[0]; got != "[s2] start=0 from=:p.1" {
		t.Errorf("suggestion played %q", got)
	}

	// The add shows "Added" at once, before the answer; the second waits
	// for the first.
	r.api.gate = make(chan struct{})
	r.click("Add Sugg One to Road Trip")
	if !r.tt.HasText("Added") || r.tt.HasText("Add Sugg One to Road Trip") {
		t.Errorf("no Added at once: %q", r.tt.Texts())
	}
	r.click("Add Sugg Two to Road Trip")
	r.did("Add p.1 [song:s1]")
	for range 20 {
		r.tt.Drain()
	}
	if r.api.did("Add p.1 [song:s2]") {
		t.Error("the second add ran before the first answered")
	}
	close(r.api.gate)
	r.did("Add p.1 [song:s2]")
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
	if r.tt.HasText("Add Sugg One to Road Trip") {
		t.Error("an added song offers Add again")
	}
}

func TestPageSuggestionFailureRestoresAddAndToasts(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.api.suggestions["p.1"] = []services.Song{pgSong("s1", "Sugg One", "Singer", "Album S")}
	r.shows("Add Sugg One to Road Trip")
	r.api.err = errOffline
	r.click("Add Sugg One to Road Trip")
	r.shows("Couldn't add “Sugg One” to “Road Trip”", "Add Sugg One to Road Trip")
	if r.tt.HasText("Added") {
		t.Error("Added stayed after the failure")
	}
}

func TestPageSuggestionsHiddenWhenEmptyOrEditing(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Song A")
	r.did("Suggestions p.1")
	for range 10 {
		r.tt.Drain()
	}
	if r.tt.HasText("Suggested songs") {
		t.Error("an empty suggestion list shows its shelf")
	}

	r = newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.api.suggestions["p.1"] = []services.Song{pgSong("s1", "Sugg One", "Singer", "Album S")}
	r.shows("Suggested songs")
	r.click("Edit")
	r.shows("Done")
	if r.tt.HasText("Suggested songs") {
		t.Error("the suggestions show while editing")
	}
}

func TestPageCollaborativePlaylistShowsTheStackDialogAndAddedBy(t *testing.T) {
	songs := threeSongs()
	for i := range songs {
		songs[i].CatalogID = "c" + songs[i].ID
	}
	d := catalogDetail("pl.u-9", "Shared mix", songs...)
	d.Kind = "user-shared"
	r := newPageRig(t, "pl.u-9", d)
	r.api.collab["pl.u-9"] = services.PlaylistCollaboration{
		Collaborative: true,
		OwnerID:       "sp.1",
		Collaborators: []services.SocialProfile{
			{ID: "sp.1", Name: "Ada", Handle: "ada"},
			{ID: "sp.2", Name: "Bo", Handle: "bo"},
			{ID: "sp.3", Name: "Cy", Handle: "cy"},
		},
		Former:  []services.SocialProfile{{ID: "sp.4", Name: "Dee", Handle: "dee"}},
		Viewer:  services.RoleListener,
		AddedBy: []services.TrackAddedBy{{SongID: "cb", ProfileID: "sp.2"}},
	}
	r.shows("Collaborative playlist · 3 songs, 10 min", "Ada & 2 others", "Added by", "Song B")
	if !r.tt.HasText("Bo") {
		t.Errorf("no adder in the Added by column: %q", r.tt.Texts())
	}

	r.click("Ada & 2 others")
	r.shows("Collaborators", "3 people can add songs to this playlist.", "Former collaborators", "Dee", "@ada", "Owner")
	// A person opens their profile and the dialog goes.
	r.click("Cy")
	if got := r.router.Path(); got != "/profile/sp.3" {
		t.Errorf("a person went to %q", got)
	}
	if r.p.page.collab.open {
		t.Error("the dialog stayed open after navigating")
	}
}

func TestPageOnlySharedPlaylistsAskForCollaboration(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Song A")
	for range 10 {
		r.tt.Drain()
	}
	if got := r.calls("Collaboration"); len(got) != 0 {
		t.Errorf("a library playlist asked for collaboration: %v", got)
	}
}

func TestCollaboratorsLabelAndRole(t *testing.T) {
	people := []services.SocialProfile{{ID: "sp.1", Name: "Ada", Handle: "ada"}, {ID: "sp.2", Handle: "bo"}, {ID: "sp.3", Name: "Cy"}}
	owner := services.PlaylistCollaboration{OwnerID: "sp.1", Collaborators: people, Viewer: services.RoleOwner}
	for _, c := range []struct {
		name string
		c    services.PlaylistCollaboration
		want string
	}{
		{"owner with others", owner, "You & 2 others"},
		{"owner with one other", services.PlaylistCollaboration{OwnerID: "sp.1", Collaborators: people[:2], Viewer: services.RoleOwner}, "You & 1 other"},
		{"owner alone", services.PlaylistCollaboration{OwnerID: "sp.1", Collaborators: people[:1], Viewer: services.RoleOwner}, "You"},
		{"listener", services.PlaylistCollaboration{OwnerID: "sp.1", Collaborators: people, Viewer: services.RoleListener}, "Ada & 2 others"},
		{"handle leads", services.PlaylistCollaboration{OwnerID: "sp.2", Collaborators: people, Viewer: services.RoleCollaborator}, "bo & 2 others"},
		{"no owner named", services.PlaylistCollaboration{Collaborators: people[:2], Viewer: services.RoleListener}, "Ada & 1 other"},
		{"alone", services.PlaylistCollaboration{OwnerID: "sp.1", Collaborators: people[:1], Viewer: services.RoleListener}, "Ada"},
	} {
		if got := collaboratorsLabel(c.c); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if got := roleLine(services.RoleOwner); got != "You started this playlist." {
		t.Errorf("owner line %q", got)
	}
	if got := roleLine(services.RoleCollaborator); got != "You're a collaborator." {
		t.Errorf("collaborator line %q", got)
	}
	if got := roleLine(services.RoleListener); got != "" {
		t.Errorf("listener line %q", got)
	}
}

func shelfDetail() services.PlaylistDetail {
	d := catalogDetail("pl.1", "Apple's Mix", threeSongs()...)
	d.Curator = services.PlaylistCurator{ID: "ac.1", Kind: "apple-curator", Name: "Cur"}
	d.FeaturedArtists = []services.Artist{{ID: "ar.1", Source: services.SourceCatalog, Name: "Zed"}}
	d.MoreByCurator = []services.Playlist{{ID: "pl.2", Source: services.SourceCatalog, Name: "Other mix", Curator: "Cur"}}
	return d
}

func TestPageShelvesLinkToArtistsAndPlaylists(t *testing.T) {
	r := newPageRig(t, "pl.1", shelfDetail())
	r.shows("Featured artists", "Zed", "More from Cur", "Other mix")
	r.click("Zed")
	if got := r.router.Path(); got != "/artist/ar.1" {
		t.Errorf("an artist tile went to %q", got)
	}

	r = newPageRig(t, "pl.1", shelfDetail())
	r.shows("Other mix")
	r.click("Other mix")
	if got := r.router.Path(); got != "/playlist/pl.2" {
		t.Errorf("a playlist tile went to %q", got)
	}

	// No page, no link.
	r = newPageRig(t, "pl.1", shelfDetail())
	r.linked = false
	r.shows("Zed", "Other mix")
	r.click("Zed")
	r.click("Other mix")
	if got := r.router.Path(); got != "/playlist/pl.1" {
		t.Errorf("unlinked tiles went to %q", got)
	}
}

func TestPageMoreLikeThisWithNoCurator(t *testing.T) {
	d := shelfDetail()
	d.Curator = services.PlaylistCurator{}
	d.Playlist.Curator = ""
	r := newPageRig(t, "pl.1", d)
	r.shows("More like this", "Other mix")
	if r.tt.HasText("More from") {
		t.Error("a curator-less shelf names a curator")
	}
}
