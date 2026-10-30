package services

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
)

func detailsService(t *testing.T) *CatalogService {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return &CatalogService{e: &Env{members: newMembership(d)}}
}

func decode[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAlbumPageMapping(t *testing.T) {
	a := decode[applemusic.Album](t, `{"id":"7","type":"albums","attributes":{"name":"In Rainbows","artistName":"Radiohead","url":"https://music.apple.com/gb/album/x/7",
		"editorialNotes":{"short":"A <i>classic</i>.","standard":"First &amp; best.<br /><br />Second para."},"recordLabel":"XL","copyright":"℗ 2007"},
		"relationships":{"artists":{"data":[{"id":"55","type":"artists","attributes":{"name":"Radiohead"}},{"id":"56","type":"artists","attributes":{"name":"Guest"}}]},
		"tracks":{"data":[
			{"id":"1","type":"songs","meta":{"popularity":0.67},"attributes":{"name":"15 Step","durationInMillis":1000,"playParams":{"id":"1"}}},
			{"id":"2","type":"songs","meta":{"popularity":1},"attributes":{"name":"Weird Fishes","durationInMillis":2000}},
			{"id":"3","type":"songs","meta":{"popularity":0.98},"attributes":{"name":"All I Need"}},
			{"id":"4","type":"songs","meta":{"popularity":0.85},"attributes":{"name":"Jigsaw"}},
			{"id":"5","type":"songs","meta":{"popularity":0.81},"attributes":{"name":"Reckoner"}}]}},
		"views":{"more-by-artist":{"data":[{"id":"8","type":"albums","attributes":{"name":"Kid A"}}]},
		"appears-on":{"data":[{"id":"pl.1","type":"playlists","attributes":{"name":"Essentials"}}]},
		"related-videos":{"data":[{"id":"9","type":"music-videos","attributes":{"name":"Clip","hasHDR":true}}]}}}`)
	p := detailsService(t).albumPage(t.Context(), a)
	if p.Album.ArtistID != "55" || len(p.Artists) != 2 || p.Tracks[0].AlbumID != "7" || p.Tracks[0].ArtistID != "55" || p.DurationMs != 3000 {
		t.Fatalf("album = %+v", p.AlbumDetail)
	}
	if p.Notes != "First & best.\n\nSecond para." || p.NotesShort != "A classic." || p.Label != "XL" || p.URL == "" {
		t.Fatalf("notes = %q / %q", p.Notes, p.NotesShort)
	}
	// The three most played tracks over the floor, most played first.
	if !slices.Equal(p.PopularIDs, []string{"2", "3", "4"}) {
		t.Fatalf("popular = %v", p.PopularIDs)
	}
	if len(p.MoreByArtist) != 1 || len(p.AppearsOn) != 1 || !p.Videos[0].HasHDR || p.Similar == nil || p.OtherVersions == nil {
		t.Fatalf("sections = %+v", p)
	}
	// Empty sections are [] in JSON, never null.
	b, _ := json.Marshal(p)
	raw := decode[map[string]any](t, string(b))
	for _, k := range []string{"similar", "otherVersions", "popularIds", "tracks"} {
		if _, ok := raw[k].([]any); !ok {
			t.Errorf("%s = %v", k, raw[k])
		}
	}
}

func TestPopularTracksSkipsShortReleases(t *testing.T) {
	single := []applemusic.Song{{}, {}}
	single[0].ID, single[0].Meta.Popularity = "1", 1
	if got := popularTracks(single); len(got) != 0 {
		t.Fatalf("single = %v", got)
	}
}

func TestArtistPageViewMapping(t *testing.T) {
	a := decode[applemusic.Artist](t, `{"id":"657515","type":"artists","attributes":{"name":"Radiohead","genreNames":["Alternative"],"origin":"Abingdon","bornOrFormed":"1985","artistBio":"<p>More than a band.</p><p>Art.</p>"},
		"views":{
		"latest-release":{"data":[{"id":"1","type":"albums","attributes":{"name":"Hail"}}]},
		"top-songs":{"data":[{"id":"2","type":"songs","attributes":{"name":"Creep"}}],"next":"/v1/catalog/gb/artists/657515/view/top-songs?offset=10"},
		"full-albums":{"data":[{"id":"3","type":"albums","attributes":{"name":"OK Computer"}}],"next":"/v1/catalog/gb/artists/657515/view/full-albums?offset=10"},
		"singles":{"data":[{"id":"4","type":"albums","attributes":{"name":"Single","isSingle":true}}]},
		"top-music-videos":{"data":[]},
		"music-videos":{"data":[{"id":"6","type":"music-videos","attributes":{"name":"Clip"}}]},
		"playlists":{"data":[{"id":"pl.7","type":"playlists","attributes":{"name":"Essentials"}}]},
		"similar-artists":{"data":[{"id":"8","type":"artists","attributes":{"name":"Muse"}}]}}}`)
	p := detailsService(t).artistPage(t.Context(), a)
	if p.Bio != "More than a band.\n\nArt." || p.Origin != "Abingdon" || p.BornOrFormed != "1985" || p.Genres[0] != "Alternative" {
		t.Fatalf("about = %q %q %q %v", p.Bio, p.Origin, p.BornOrFormed, p.Genres)
	}
	if p.LatestRelease == nil || p.LatestRelease.ID != "1" || len(p.TopSongs) != 1 || !p.TopSongsMore {
		t.Fatalf("header sections = %+v", p)
	}
	var got []string
	for _, sh := range p.Shelves {
		got = append(got, sh.View+":"+string(sh.Kind))
	}
	// Display order; empty views are left out; empty top videos fall back to all videos.
	want := []string{"full-albums:albums", "singles:albums", "music-videos:videos", "playlists:playlists", "similar-artists:artists"}
	if !slices.Equal(got, want) {
		t.Fatalf("shelves = %v\nwant %v", got, want)
	}
	if !p.Shelves[0].More || p.Shelves[1].More || len(p.Shelves[2].Videos) != 1 || p.Shelves[4].Artists[0].Name != "Muse" || p.Shelves[3].Albums == nil {
		t.Fatalf("shelves = %+v", p.Shelves)
	}
	// Every shelf view can be paged by ArtistView.
	for _, v := range append(slices.Clone(artistShelves), ViewTopSongs, ViewMusicVideos) {
		if _, ok := viewKinds[v]; !ok {
			t.Errorf("view %s has no kind", v)
		}
	}
	if _, err := detailsService(t).ArtistView(t.Context(), "1", "canvas", 0, 10); err == nil {
		t.Error("unknown view accepted")
	}
}

func TestCreditGroups(t *testing.T) {
	s := decode[applemusic.Song](t, `{"id":"1","type":"songs","relationships":{"credits":{"data":[
		{"id":"1-4","type":"role-categories","attributes":{"kind":"performer","title":"PERFORMING ARTISTS"},"relationships":{"credit-artists":{"data":[{"id":"a","type":"credit-artists","attributes":{"name":"Thom Yorke","roleNames":["Guitar","Lead Vocals"]}}]}}},
		{"id":"1-5","type":"role-categories","attributes":{"kind":"production-and-engineering","title":"PRODUCTION & ENGINEERING"},"relationships":{"credit-artists":{"data":[]}}}]}}}`)
	g := creditGroups(s.Relationships.Credits.Data)
	if len(g) != 1 || g[0].Kind != "performer" || g[0].Credits[0].Name != "Thom Yorke" || len(g[0].Credits[0].Roles) != 2 {
		t.Fatalf("credits = %+v", g)
	}
}

func TestLyricsDTO(t *testing.T) {
	ms := time.Millisecond
	l := lyricsDTO(applemusic.Lyrics{Timing: "Word", Lines: []applemusic.LyricLine{{
		Begin: 1500 * ms, End: 3 * time.Second, Key: "L1", Text: "hi there",
		Words: []applemusic.LyricWord{{Begin: 1500 * ms, End: 2 * time.Second, Text: "hi "}},
	}, {Key: "L2", Text: "untranslated"}}, Localizations: []applemusic.LyricLocalization{
		{Kind: "translation", Language: "fr-FR", Lines: map[string]applemusic.LyricLine{"L1": {Text: "salut"}}},
		{Kind: "translation", Language: "en-US", Lines: map[string]applemusic.LyricLine{"L1": {Text: "hello", BackgroundText: "(hey)"}}},
		{Kind: "transliteration", Language: "ko-Latn", Lines: map[string]applemusic.LyricLine{"L1": {Text: "an nyeong", Words: []applemusic.LyricWord{{Begin: 1500 * ms, End: 1800 * ms, Text: "an "}}}}},
	}}, "en-GB")
	if l.Songwriters == nil || len(l.Lines) != 2 || l.Lines[0].BeginMs != 1500 || l.Lines[0].EndMs != 3000 || l.Lines[0].Words[0].EndMs != 2000 {
		t.Fatalf("lyrics = %+v", l)
	}
	if l.TranslationLanguage != "en-US" || l.TransliterationLanguage != "ko-Latn" {
		t.Fatalf("languages = %q %q", l.TranslationLanguage, l.TransliterationLanguage)
	}
	tr, tl := l.Lines[0].Translation, l.Lines[0].Transliteration
	if tr == nil || tr.Text != "hello" || tr.Background != "(hey)" || tl == nil || tl.Words[0].EndMs != 1800 {
		t.Fatalf("line 1 localizations = %+v %+v", tr, tl)
	}
	if l.Lines[1].Translation != nil || l.Lines[1].Transliteration != nil || l.Lines[1].Words == nil {
		t.Fatalf("line 2 = %+v", l.Lines[1])
	}
	plain := lyricsDTO(applemusic.Lyrics{Lines: []applemusic.LyricLine{{Key: "L1", Text: "x"}}}, "en-GB")
	if plain.TranslationLanguage != "" || plain.Lines[0].Translation != nil {
		t.Fatalf("plain = %+v", plain)
	}
}

func TestPlainText(t *testing.T) {
	for in, want := range map[string]string{
		"":                                 "",
		"A <i>long</i> note.":              "A long note.",
		"One<br/>two<BR>three":             "One\ntwo\nthree",
		"<p>A</p>\n<p>B &amp; C</p>":       "A\n\nB & C",
		"Para one.<br /><br /><br />Next.": "Para one.\n\nNext.",
	} {
		if got := plainText(in); got != want {
			t.Errorf("plainText(%q) = %q, want %q", in, got, want)
		}
	}
}
